package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"product-service/models"
	"product-service/repository"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ProductServiceDDB is a DynamoDB-backed product service
type ProductServiceDDB struct {
	productRepo     repository.ProductRepo
	categoryRepo    repository.CategoryRepo
	s3Client        *s3.Client
	presignClient   *s3.PresignClient
	bucket          string
	prefix          string
	endpoint        string
	cdnDomain       string
	inventoryClient *InventoryClient
}

func NewProductServiceDDB(
	pr repository.ProductRepo,
	cr repository.CategoryRepo,
	s3Client *s3.Client,
	presignClient *s3.PresignClient,
	bucket, prefix, endpoint, cdnDomain string,
	inventoryClient *InventoryClient,
) *ProductServiceDDB {
	return &ProductServiceDDB{
		productRepo:     pr,
		categoryRepo:    cr,
		s3Client:        s3Client,
		presignClient:   presignClient,
		bucket:          bucket,
		prefix:          prefix,
		endpoint:        endpoint,
		cdnDomain:       cdnDomain,
		inventoryClient: inventoryClient,
	}
}

// GeneratePresignedUpload returns a presigned PUT URL, the object key, and the public URL
func (s *ProductServiceDDB) GeneratePresignedUpload(ctx context.Context, sku, filename, contentType string, expiresSeconds int64) (string, string, string, error) {
	ext := filepath.Ext(filename)
	key := fmt.Sprintf("%sproduct_img_%s_%s%s", s.prefix, sku, uuid.New().String(), ext)

	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}

	presignedReq, err := s.presignClient.PresignPutObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = time.Duration(expiresSeconds) * time.Second
	})
	if err != nil {
		return "", "", "", fmt.Errorf("failed to presign put object: %w", err)
	}

	var publicURL string
	if s.cdnDomain != "" {
		publicURL = fmt.Sprintf("https://%s/%s", strings.TrimRight(s.cdnDomain, "/"), key)
	} else if s.endpoint != "" {
		publicURL = fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.endpoint, "/"), s.bucket, key)
	} else {
		publicURL = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, key)
	}

	return presignedReq.URL, key, publicURL, nil
}

func (s *ProductServiceDDB) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return s.productRepo.FindByID(ctx, id)
}

func (s *ProductServiceDDB) ListProducts(ctx context.Context, params ListProductsParams) ([]*models.Product, int64, error) {
	// Build filter map (use plain types for repository layer)
	filter := make(map[string]interface{})

	if params.IsFeatured != nil {
		filter["is_featured"] = *params.IsFeatured
	}
	if len(params.CategoryID) > 0 {
		// convert to []string for easier handling in the repo layer
		ids := make([]string, 0, len(params.CategoryID))
		for _, u := range params.CategoryID {
			ids = append(ids, u.String())
		}
		filter["category_ids"] = ids
	}
	if params.MinPrice != nil {
		filter["min_price"] = *params.MinPrice
	}
	if params.MaxPrice != nil {
		filter["max_price"] = *params.MaxPrice
	}
	if params.Brand != nil {
		filter["brand"] = *params.Brand
	}
	if params.InStock != nil {
		filter["in_stock"] = *params.InStock
	}

	limit := params.PerPage
	skip := (params.Page - 1) * params.PerPage

	products, err := s.productRepo.Find(ctx, filter, limit, skip)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.productRepo.Count(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func (s *ProductServiceDDB) CreateProduct(ctx context.Context, req ProductCreateRequest, images []*multipart.FileHeader) (*models.Product, error) {
	// Step 1: Look up categories
	categories, err := s.categoryRepo.FindByNames(ctx, req.Categories)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch categories: %w", err)
	}
	if len(categories) != len(req.Categories) {
		return nil, fmt.Errorf("one or more categories not found")
	}

	var categoryIDs []uuid.UUID
	categorySet := make(map[uuid.UUID]bool)
	for _, cat := range categories {
		if !categorySet[cat.ID] {
			categoryIDs = append(categoryIDs, cat.ID)
			categorySet[cat.ID] = true
		}
		for _, ancestor := range cat.Ancestors {
			if !categorySet[ancestor] {
				categoryIDs = append(categoryIDs, ancestor)
				categorySet[ancestor] = true
			}
		}
	}

	// Step 2: Upload images to S3
	var imageURLs []string
	for i, fileHeader := range images {
		file, err := fileHeader.Open()
		if err != nil {
			continue
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("%sproduct_img_%s_%d", s.prefix, req.SKU, i)
		_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(s.bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(fileHeader.Header.Get("Content-Type")),
		})
		if err != nil {
			continue
		}
		var urlStr string
		if s.endpoint != "" {
			urlStr = fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.endpoint, "/"), s.bucket, key)
		} else {
			urlStr = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, key)
		}
		imageURLs = append(imageURLs, urlStr)
	}

	// Step 3: Create the product model
	now := time.Now().UTC()
	product := &models.Product{
		ID:          uuid.New(),
		Name:        req.Name,
		Price:       req.Price,
		Quantity:    req.Quantity,
		Description: req.Description,
		Images:      imageURLs,
		Brand:       req.Brand,
		SKU:         req.SKU,
		CategoryIDs: categoryIDs,
		IsFeatured:  req.IsFeatured,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Step 4: Save to DynamoDB
	err = s.productRepo.Create(ctx, product)
	if err != nil {
		return nil, err
	}

	// Step 5: Sync inventory (fire-and-forget; log errors but don't fail the product creation)
	if s.inventoryClient != nil && product.Quantity > 0 {
		if invErr := s.inventoryClient.SetStock(ctx, product.ID.String(), product.Quantity); invErr != nil {
			zap.L().Warn("Failed to sync inventory for new product",
				zap.String("product_id", product.ID.String()),
				zap.Int("quantity", product.Quantity),
				zap.Error(invErr),
			)
		}
	}

	return product, nil
}

func (s *ProductServiceDDB) UpdateProduct(ctx context.Context, id uuid.UUID, updates map[string]interface{}) (int64, error) {
	if len(updates) == 0 {
		return 0, fmt.Errorf("no update fields provided")
	}
	delete(updates, "_id")
	delete(updates, "product_id")

	updates["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	err := s.productRepo.Update(ctx, id, updates)
	if err != nil {
		return 0, err
	}

	return 1, nil
}

func (s *ProductServiceDDB) DeleteProduct(ctx context.Context, id uuid.UUID) (int64, error) {
	err := s.productRepo.Delete(ctx, id)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

// BulkDeleteProducts deletes products by IDs, by category membership, or all products when requested.
func (s *ProductServiceDDB) BulkDeleteProducts(ctx context.Context, req BulkDeleteRequest) (int64, error) {
	// If DeleteAll, fetch all product IDs
	var idsToDelete []uuid.UUID
	if req.DeleteAll {
		products, err := s.productRepo.Find(ctx, nil, 0, 0)
		if err != nil {
			return 0, fmt.Errorf("failed to fetch products for delete-all: %w", err)
		}
		for _, p := range products {
			idsToDelete = append(idsToDelete, p.ID)
		}
	} else {
		// Add explicit IDs
		idsSet := make(map[string]uuid.UUID)
		for _, id := range req.IDs {
			idsSet[id.String()] = id
		}

		// If categories provided, scan products and add matches
		if len(req.CategoryIDs) > 0 {
			// Fetch all products (Find currently does table scan)
			products, err := s.productRepo.Find(ctx, nil, 0, 0)
			if err != nil {
				return 0, fmt.Errorf("failed to fetch products for category delete: %w", err)
			}
			for _, p := range products {
				for _, cat := range p.CategoryIDs {
					for _, target := range req.CategoryIDs {
						if cat == target {
							idsSet[p.ID.String()] = p.ID
							break
						}
					}
				}
			}
		}

		for _, v := range idsSet {
			idsToDelete = append(idsToDelete, v)
		}
	}

	if len(idsToDelete) == 0 {
		return 0, nil
	}

	// Perform batch delete via repo
	if err := s.productRepo.DeleteMany(ctx, idsToDelete); err != nil {
		return 0, fmt.Errorf("failed to delete products: %w", err)
	}

	return int64(len(idsToDelete)), nil
}

func (s *ProductServiceDDB) GetProductInternal(ctx context.Context, id uuid.UUID) (*ProductInternalDTO, error) {
	product, err := s.productRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	dto := &ProductInternalDTO{
		ID:    product.ID,
		Name:  product.Name,
		Price: product.Price,
		Stock: product.Quantity,
	}

	return dto, nil
}

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

func (s *ProductServiceDDB) ProcessBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportResult, error) {
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
	key := fmt.Sprintf("%sproduct_img_%s_%d", s.prefix, sku, index)
	_, err = s.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(http.DetectContentType(data)),
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to s3: %w", err)
	}

	if s.endpoint != "" {
		return fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.endpoint, "/"), s.bucket, key), nil
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.bucket, key), nil
}
