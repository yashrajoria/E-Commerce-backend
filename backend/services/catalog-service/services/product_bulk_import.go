package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"catalog-service/models"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ValidateBulkImport executes business logic for validate bulk import.
func (s *ProductServiceDDB) ValidateBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportValidation, error) {
	r := csv.NewReader(file)
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV must include a header row")
	}

	index := make(map[string]int)
	for i, h := range headers {
		index[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Ensure required headers exist to avoid panics on indexing
	requiredHeaders := []string{"name", "sku", "price", "quantity", "is_featured", "categories", "imageurl"}
	for _, h := range requiredHeaders {
		if _, ok := index[h]; !ok {
			return nil, fmt.Errorf("missing required CSV header: %s", h)
		}
	}

	type pendingProduct struct {
		Row           []string
		RowNum        int
		CategoryNames []string
		SKU           string
	}

	var pendingProducts []pendingProduct
	categoryNamesSet := make(map[string]bool)
	skuSet := make(map[string]int)
	var errorsList []map[string]interface{}
	var warningsList []map[string]interface{}
	rowNum := 2

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Failed to parse CSV row",
			})
			rowNum++
			continue
		}

		// Safe access helper in case the row is malformed or short
		get := func(key string) string {
			if idx, ok := index[key]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		name := get("name")
		sku := get("sku")
		priceStr := get("price")
		quantityStr := get("quantity")
		isFeaturedStr := get("is_featured")

		hasError := false

		if name == "" {
			errorsList = append(errorsList, map[string]interface{}{"row": rowNum, "error": "Product name is required"})
			hasError = true
		}

		if sku == "" {
			errorsList = append(errorsList, map[string]interface{}{"row": rowNum, "error": "SKU is required"})
			hasError = true
		} else if existingRow, exists := skuSet[sku]; exists {
			errorsList = append(errorsList, map[string]interface{}{"row": rowNum, "error": fmt.Sprintf("Duplicate SKU '%s' found (also in row %d)", sku, existingRow)})
			hasError = true
		} else {
			skuSet[sku] = rowNum
		}

		if _, err := strconv.ParseFloat(priceStr, 64); err != nil {
			errorsList = append(errorsList, map[string]interface{}{"row": rowNum, "error": "Invalid price format"})
			hasError = true
		}

		if _, err := strconv.Atoi(quantityStr); err != nil {
			errorsList = append(errorsList, map[string]interface{}{"row": rowNum, "error": "Invalid quantity format"})
			hasError = true
		}

		if _, err := strconv.ParseBool(isFeaturedStr); err != nil {
			errorsList = append(errorsList, map[string]interface{}{"row": rowNum, "error": "Invalid is_featured format (must be TRUE or FALSE)"})
			hasError = true
		}

		imageURL := get("imageurl")
		if imageURL != "" {
			if u, err := url.Parse(imageURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
				warningsList = append(warningsList, map[string]interface{}{"row": rowNum, "warning": "Invalid image URL - product will be created without image"})
			}
		}

		rawCategories := strings.Split(row[index["categories"]], ",")
		var currentCatNames []string
		for _, cName := range rawCategories {
			trimmed := strings.TrimSpace(cName)
			if trimmed != "" {
				categoryNamesSet[trimmed] = true
				currentCatNames = append(currentCatNames, trimmed)
			}
		}

		if !hasError {
			pendingProducts = append(pendingProducts, pendingProduct{Row: row, RowNum: rowNum, CategoryNames: currentCatNames, SKU: sku})
		}
		rowNum++
	}

	var catNames []string
	for name := range categoryNamesSet {
		catNames = append(catNames, name)
	}

	existingCategories, err := s.categoryRepo.FindByNames(ctx, catNames)
	if err != nil {
		return nil, err
	}

	existingCatMap := make(map[string]bool)
	for _, cat := range existingCategories {
		existingCatMap[cat.Name] = true
	}

	var missingCategories []string
	for _, name := range catNames {
		if !existingCatMap[name] {
			missingCategories = append(missingCategories, name)
		}
	}

	var skusToCheck []string
	for sku := range skuSet {
		skusToCheck = append(skusToCheck, sku)
	}

	existingSKUs, err := s.productRepo.FindBySKUs(ctx, skusToCheck)
	if err != nil {
		return nil, err
	}

	var duplicateSKUs []string
	for _, existingProduct := range existingSKUs {
		if _, inCSV := skuSet[existingProduct.SKU]; inCSV {
			duplicateSKUs = append(duplicateSKUs, existingProduct.SKU)
		}
	}

	return &models.BulkImportValidation{
		TotalProducts:     rowNum - 2,
		ValidProducts:     len(pendingProducts),
		InvalidProducts:   len(errorsList),
		Errors:            errorsList,
		Warnings:          warningsList,
		MissingCategories: missingCategories,
		DuplicateSKUs:     duplicateSKUs,
	}, nil
}

// ProcessBulkImport executes business logic for process bulk import.
func (s *ProductServiceDDB) ProcessBulkImport(ctx context.Context, file multipart.File, autoCreateCategories bool) (*models.BulkImportResult, error) {
	r := csv.NewReader(file)
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV must include a header row")
	}

	index := make(map[string]int)
	for i, h := range headers {
		index[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// Ensure required headers exist
	requiredHeaders := []string{"name", "sku", "price", "quantity", "is_featured", "categories", "imageurl"}
	for _, h := range requiredHeaders {
		if _, ok := index[h]; !ok {
			return nil, fmt.Errorf("missing required CSV header: %s", h)
		}
	}

	type pendingProduct struct {
		Row           []string
		RowNum        int
		CategoryNames []string
		SKU           string
	}

	var pendingProducts []pendingProduct
	categoryNamesSet := make(map[string]bool)
	var errorsList []map[string]interface{}
	rowNum := 2

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errorsList = append(errorsList, map[string]interface{}{"row": rowNum, "error": "Failed to parse CSV row"})
			rowNum++
			continue
		}
		// Safe access helper
		get := func(key string) string {
			if idx, ok := index[key]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		sku := get("sku")
		rawCategories := strings.Split(get("categories"), ",")
		var catNames []string
		for _, cName := range rawCategories {
			trimmed := strings.TrimSpace(cName)
			if trimmed != "" {
				categoryNamesSet[trimmed] = true
				catNames = append(catNames, trimmed)
			}
		}

		pendingProducts = append(pendingProducts, pendingProduct{Row: row, RowNum: rowNum, CategoryNames: catNames, SKU: sku})
		rowNum++
	}

	var allCatNames []string
	for name := range categoryNamesSet {
		allCatNames = append(allCatNames, name)
	}

	existingCategories, err := s.categoryRepo.FindByNames(ctx, allCatNames)
	if err != nil {
		return nil, err
	}

	catNameToIDs := make(map[string][]uuid.UUID)
	for _, cat := range existingCategories {
		ids := []uuid.UUID{cat.ID}
		ids = append(ids, cat.Ancestors...)
		catNameToIDs[cat.Name] = ids
	}

	if autoCreateCategories {
		now := time.Now().UTC()
		for _, catName := range allCatNames {
			if _, exists := catNameToIDs[catName]; exists {
				continue
			}

			newCategory := &models.Category{
				ID:        uuid.New(),
				Name:      catName,
				ParentIDs: []uuid.UUID{},
				Ancestors: []uuid.UUID{},
				Slug:      strings.ToLower(strings.ReplaceAll(catName, " ", "-")),
				IsActive:  true,
				CreatedAt: now,
				UpdatedAt: now,
			}

			if err := s.categoryRepo.Create(ctx, newCategory); err != nil {
				return nil, fmt.Errorf("failed to auto-create category '%s': %w", catName, err)
			}

			catNameToIDs[catName] = []uuid.UUID{newCategory.ID}
		}
	}

	var productsToInsert []models.Product
	for _, pp := range pendingProducts {
		// Safe getters for fields
		get := func(key string) string {
			if idx, ok := index[key]; ok && idx < len(pp.Row) {
				return strings.TrimSpace(pp.Row[idx])
			}
			return ""
		}

		name := get("name")
		sku := get("sku")
		price, _ := strconv.ParseFloat(get("price"), 64)
		quantity, _ := strconv.Atoi(get("quantity"))
		isFeatured, _ := strconv.ParseBool(get("is_featured"))

		categorySet := make(map[uuid.UUID]bool)
		for _, catName := range pp.CategoryNames {
			if ids, ok := catNameToIDs[catName]; ok {
				for _, id := range ids {
					categorySet[id] = true
				}
			}
		}
		var categoryIDs []uuid.UUID
		for id := range categorySet {
			categoryIDs = append(categoryIDs, id)
		}

		imageURL := get("imageurl")
		var imageURLs []string
		if imageURL != "" {
			uploadedURL, err := s.uploadImageFromURL(ctx, imageURL, sku, 0)
			if err == nil {
				imageURLs = append(imageURLs, uploadedURL)
			}
		}

		now := time.Now().UTC()
		product := models.Product{
			ID:          uuid.New(),
			Name:        name,
			Price:       price,
			Quantity:    quantity,
			Description: get("description"),
			Images:      imageURLs,
			Brand:       get("brand"),
			SKU:         sku,
			IsFeatured:  isFeatured,
			CategoryIDs: categoryIDs,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		productsToInsert = append(productsToInsert, product)
	}

	// Insert in chunks and skip duplicates by SKU
	insertedCount := 0
	var rowResults []map[string]interface{}
	const chunkSize = 25
	for i := 0; i < len(productsToInsert); i += chunkSize {
		end := i + chunkSize
		if end > len(productsToInsert) {
			end = len(productsToInsert)
		}
		chunk := productsToInsert[i:end]

		// Build SKU list for this chunk and check existing SKUs
		var skus []string
		for _, p := range chunk {
			skus = append(skus, p.SKU)
		}
		existing, err := s.productRepo.FindBySKUs(ctx, skus)
		if err != nil {
			return nil, err
		}
		existingSet := make(map[string]bool)
		for _, ep := range existing {
			existingSet[ep.SKU] = true
		}

		var toInsert []models.Product
		for _, p := range chunk {
			// find corresponding pendingProducts rowNum for SKU
			rowNumForSKU := -1
			for _, pp := range pendingProducts {
				if pp.SKU == p.SKU {
					rowNumForSKU = pp.RowNum
					break
				}
			}
			if p.SKU != "" && existingSet[p.SKU] {
				// record duplicate SKU as an error and row result
				errorsList = append(errorsList, map[string]interface{}{"row": rowNumForSKU, "sku": p.SKU, "error": "duplicate SKU - already exists"})
				rowResults = append(rowResults, map[string]interface{}{"row": rowNumForSKU, "sku": p.SKU, "status": "skipped", "reason": "duplicate SKU"})
				continue
			}
			toInsert = append(toInsert, p)
		}

		if len(toInsert) > 0 {
			if err := s.productRepo.CreateMany(ctx, toInsert); err != nil {
				// mark all rows in this toInsert as failed
				for _, p := range toInsert {
					rowNumForSKU := -1
					for _, pp := range pendingProducts {
						if pp.SKU == p.SKU {
							rowNumForSKU = pp.RowNum
							break
						}
					}
					errorsList = append(errorsList, map[string]interface{}{"row": rowNumForSKU, "sku": p.SKU, "error": err.Error()})
					rowResults = append(rowResults, map[string]interface{}{"row": rowNumForSKU, "sku": p.SKU, "status": "failed", "reason": err.Error()})
				}
				return nil, err
			}
			insertedCount += len(toInsert)
			for _, p := range toInsert {
				rowNumForSKU := -1
				for _, pp := range pendingProducts {
					if pp.SKU == p.SKU {
						rowNumForSKU = pp.RowNum
						break
					}
				}
				rowResults = append(rowResults, map[string]interface{}{"row": rowNumForSKU, "sku": p.SKU, "status": "inserted"})

				// Sync inventory for each inserted product
				if s.inventoryClient != nil && p.Quantity > 0 {
					if invErr := s.inventoryClient.SetStock(ctx, p.ID.String(), p.Quantity); invErr != nil {
						zap.L().Warn("Failed to sync inventory for bulk product",
							zap.String("product_id", p.ID.String()),
							zap.Int("quantity", p.Quantity),
							zap.Error(invErr),
						)
					}
				}
			}
		}
	}

	return &models.BulkImportResult{
		InsertedCount: insertedCount,
		ErrorsCount:   len(errorsList),
		Errors:        errorsList,
		Message:       "Bulk import process completed",
		RowResults:    rowResults,
	}, nil
}

func (s *ProductServiceDDB) uploadImageFromURL(ctx context.Context, imageURL, sku string, index int) (string, error) {
	// Use a context-aware HTTP client with a timeout to avoid hanging
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for image download: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read downloaded image: %w", err)
	}
	detected := http.DetectContentType(data[:min(len(data), 512)])
	switch detected {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		// allowed
	default:
		return "", fmt.Errorf("downloaded file is not an allowed image (detected %s)", detected)
	}
	key := fmt.Sprintf("%sproduct_img_%s_%d", s.prefix, sku, index)
	if s.storage == nil {
		return "", fmt.Errorf("storage client not configured")
	}
	return s.storage.Upload(ctx, key, data, detected)
}

func (s *ProductServiceDDB) publicObjectURL(key string) string {
	if s.storage != nil {
		return s.storage.PublicURL(key)
	}
	return key
}

func normalizePublicBaseURL(raw string) string {
	base := strings.TrimSpace(raw)
	base = strings.TrimRight(base, "/")
	if base == "" {
		return ""
	}
	if strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		return base
	}
	return "https://" + base
}
