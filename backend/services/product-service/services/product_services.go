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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ListProductsParams defines the parameters for listing products.
type ListProductsParams struct {
	Page       int
	PerPage    int
	IsFeatured *bool // Use a pointer to distinguish between false and not set
	CategoryID []uuid.UUID
	MinPrice   *float64
	MaxPrice   *float64
	Sort       string
}

type ProductCreateRequest struct {
	Name        string
	Description string
	Brand       string
	SKU         string
	Price       float64
	Quantity    int
	IsFeatured  bool
	Categories  []string // Expecting an array of category names
}
type ProductInternalDTO struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Price float64   `json:"price"`
	Stock int       `json:"stock"`
}

type ProductService struct {
	productRepo   *repository.ProductRepository
	categoryRepo  *repository.CategoryRepository
	s3Client      *s3.Client
	presignClient *s3.PresignClient
	bucket        string
	prefix        string
	endpoint      string
	cdnDomain     string
}

func NewProductService(pr *repository.ProductRepository, cr *repository.CategoryRepository, s3Client *s3.Client, presignClient *s3.PresignClient, bucket, prefix, endpoint, cdnDomain string) *ProductService {
	return &ProductService{
		productRepo:   pr,
		categoryRepo:  cr,
		s3Client:      s3Client,
		presignClient: presignClient,
		bucket:        bucket,
		prefix:        prefix,
		endpoint:      endpoint,
		cdnDomain:     cdnDomain,
	}
}

// GeneratePresignedUpload returns a presigned PUT URL, the object key, and the public URL for the object
func (s *ProductService) GeneratePresignedUpload(ctx context.Context, sku, filename, contentType string, expiresSeconds int64) (string, string, string, error) {
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

func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return s.productRepo.FindByID(ctx, id)
}

func (s *ProductService) ListProducts(ctx context.Context, params ListProductsParams) ([]*models.Product, int64, error) {
	filter := bson.M{"deleted_at": bson.M{"$exists": false}}
	if params.IsFeatured != nil {
		filter["is_featured"] = *params.IsFeatured
	}
	if len(params.CategoryID) > 0 {
		filter["category_ids"] = bson.M{"$in": params.CategoryID}
	}
	if params.MinPrice != nil || params.MaxPrice != nil {
		priceFilter := bson.M{}
		if params.MinPrice != nil {
			priceFilter["$gte"] = *params.MinPrice
		}
		if params.MaxPrice != nil {
			priceFilter["$lte"] = *params.MaxPrice
		}
		filter["price"] = priceFilter
	}

	findOptions := options.Find().
		SetLimit(int64(params.PerPage)).
		SetSkip(int64((params.Page - 1) * params.PerPage))
	if sortField, sortDirection := resolveSort(params.Sort); sortField != "" {
		findOptions.SetSort(bson.D{{Key: sortField, Value: sortDirection}})
	}

	products, err := s.productRepo.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.productRepo.Count(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func resolveSort(sortParam string) (string, int) {
	switch sortParam {
	case "price_asc":
		return "price", 1
	case "price_desc":
		return "price", -1
	case "created_at_asc":
		return "created_at", 1
	case "created_at_desc":
		return "created_at", -1
	case "name_asc":
		return "name", 1
	case "name_desc":
		return "name", -1
	default:
		return "", 0
	}
}

func (s *ProductService) CreateProduct(ctx context.Context, req ProductCreateRequest, images []*multipart.FileHeader) (*models.Product, error) {
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
			continue // Or return error
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

	// Step 4: Save to database
	_, err = s.productRepo.Create(ctx, product)
	if err != nil {
		return nil, err
	}

	return product, nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, id uuid.UUID, updates bson.M) (int64, error) {
	// Business logic for updates can go here (e.g., validating fields)
	if len(updates) == 0 {
		return 0, fmt.Errorf("no update fields provided")
	}
	delete(updates, "_id") // Prevent changing the ID

	result, err := s.productRepo.Update(ctx, id, updates)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, id uuid.UUID) (int64, error) {
	result, err := s.productRepo.Delete(ctx, id)
	if err != nil {
		return 0, err
	}
	return result.ModifiedCount, nil
}

func (s *ProductService) GetProductInternal(ctx context.Context, id uuid.UUID) (*ProductInternalDTO, error) {
	product, err := s.productRepo.FindByIDInternal(ctx, id)
	if err != nil {
		return nil, err
	}

	// Transform the full model to the simplified DTO
	dto := &ProductInternalDTO{
		ID:    product.ID,
		Name:  product.Name,
		Price: product.Price,
		Stock: product.Quantity,
	}

	return dto, nil
}

func (s *ProductService) ValidateBulkImport(ctx context.Context, file multipart.File) (*models.BulkImportValidation, error) {
	r := csv.NewReader(file)
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV must include a header row")
	}

	index := make(map[string]int)
	for i, h := range headers {
		index[strings.ToLower(strings.TrimSpace(h))] = i
	}

	type pendingProduct struct {
		Row           []string
		RowNum        int
		CategoryNames []string
		SKU           string
	}

	var pendingProducts []pendingProduct
	categoryNamesSet := make(map[string]bool)
	skuSet := make(map[string]int) // Track SKUs and their row numbers
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

		// Validate required fields
		name := strings.TrimSpace(row[index["name"]])
		sku := strings.TrimSpace(row[index["sku"]])
		priceStr := strings.TrimSpace(row[index["price"]])
		quantityStr := strings.TrimSpace(row[index["quantity"]])
		isFeaturedStr := strings.TrimSpace(row[index["is_featured"]])

		hasError := false

		if name == "" {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Product name is required",
			})
			hasError = true
		}

		if sku == "" {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "SKU is required",
			})
			hasError = true
		} else {
			// Check for duplicate SKUs in CSV
			if existingRow, exists := skuSet[sku]; exists {
				errorsList = append(errorsList, map[string]interface{}{
					"row":   rowNum,
					"error": fmt.Sprintf("Duplicate SKU '%s' found (also in row %d)", sku, existingRow),
				})
				hasError = true
			} else {
				skuSet[sku] = rowNum
			}
		}

		if _, err := strconv.ParseFloat(priceStr, 64); err != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Invalid price format",
			})
			hasError = true
		}

		if _, err := strconv.Atoi(quantityStr); err != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Invalid quantity format",
			})
			hasError = true
		}

		if _, err := strconv.ParseBool(isFeaturedStr); err != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Invalid is_featured format (must be TRUE or FALSE)",
			})
			hasError = true
		}

		// Validate image URL
		imageURL := strings.TrimSpace(row[index["imageurl"]])
		if imageURL != "" {
			if u, err := url.Parse(imageURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
				warningsList = append(warningsList, map[string]interface{}{
					"row":     rowNum,
					"warning": "Invalid image URL - product will be created without image",
				})
			}
		}

		// Parse categories
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
			pendingProducts = append(pendingProducts, pendingProduct{
				Row:           row,
				RowNum:        rowNum,
				CategoryNames: currentCatNames,
				SKU:           sku,
			})
		}

		rowNum++
	}

	// Check which categories exist in database
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

	// Check for duplicate SKUs in database
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
		duplicateSKUs = append(duplicateSKUs, existingProduct.SKU)
		warningsList = append(warningsList, map[string]interface{}{
			"row":     skuSet[existingProduct.SKU],
			"warning": fmt.Sprintf("SKU '%s' already exists in database - will be skipped", existingProduct.SKU),
		})
	}

	return &models.BulkImportValidation{
		TotalProducts:     len(pendingProducts) + len(errorsList),
		ValidProducts:     len(pendingProducts),
		InvalidProducts:   len(errorsList),
		MissingCategories: missingCategories,
		DuplicateSKUs:     duplicateSKUs,
		Errors:            errorsList,
		Warnings:          warningsList,
	}, nil
}

func (s *ProductService) CreateBulkProducts(ctx context.Context, file multipart.File, autoCreateCategories bool) (*models.BulkImportResult, error) {
	r := csv.NewReader(file)
	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV must include a header row")
	}

	index := make(map[string]int)
	for i, h := range headers {
		index[strings.ToLower(strings.TrimSpace(h))] = i
	}

	type pendingProduct struct {
		Row           []string
		RowNum        int
		CategoryNames []string
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
			errorsList = append(errorsList, map[string]interface{}{
				"row":   rowNum,
				"error": "Failed to parse CSV row",
			})
			rowNum++
			continue
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
		pendingProducts = append(pendingProducts, pendingProduct{
			Row:           row,
			RowNum:        rowNum,
			CategoryNames: currentCatNames,
		})
		rowNum++
	}

	// Fetch existing categories
	var catNames []string
	for name := range categoryNamesSet {
		catNames = append(catNames, name)
	}

	categories, err := s.categoryRepo.FindByNames(ctx, catNames)
	if err != nil {
		return nil, err
	}

	categoryCache := make(map[string]models.Category)
	for _, cat := range categories {
		categoryCache[cat.Name] = cat
	}

	// Auto-create missing categories if enabled
	if autoCreateCategories {
		for _, name := range catNames {
			if _, exists := categoryCache[name]; !exists {
				newCategory := &models.Category{
					ID:        uuid.New(),
					Name:      name,
					Slug:      strings.ToLower(strings.ReplaceAll(name, " ", "-")),
					Ancestors: []uuid.UUID{},
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				}

				_, err := s.categoryRepo.Create(ctx, newCategory)
				if err != nil {
					return nil, fmt.Errorf("failed to create category '%s': %w", name, err)
				}
				categoryCache[name] = *newCategory
			}
		}
	}

	// Get existing SKUs to avoid duplicates
	var skusToCheck []string
	for _, pp := range pendingProducts {
		sku := strings.TrimSpace(pp.Row[index["sku"]])
		if sku != "" {
			skusToCheck = append(skusToCheck, sku)
		}
	}

	existingSKUs := make(map[string]bool)
	if len(skusToCheck) > 0 {
		existingProducts, err := s.productRepo.FindBySKUs(ctx, skusToCheck)
		if err == nil {
			for _, product := range existingProducts {
				existingSKUs[product.SKU] = true
			}
		}
	}

	// Process products
	var productsToInsert []interface{}
	for _, pp := range pendingProducts {
		name := strings.TrimSpace(pp.Row[index["name"]])
		sku := strings.TrimSpace(pp.Row[index["sku"]])
		price, err1 := strconv.ParseFloat(strings.TrimSpace(pp.Row[index["price"]]), 64)
		quantity, err2 := strconv.Atoi(strings.TrimSpace(pp.Row[index["quantity"]]))
		isFeatured, err3 := strconv.ParseBool(strings.TrimSpace(pp.Row[index["is_featured"]]))

		if name == "" || sku == "" || err1 != nil || err2 != nil || err3 != nil {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   pp.RowNum,
				"error": "Invalid data format (name, sku, price, quantity, or is_featured)",
			})
			continue
		}

		// Validate quantity is non-negative
		if quantity < 0 {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   pp.RowNum,
				"error": "Quantity cannot be negative",
			})
			continue
		}

		// Skip if SKU already exists
		if existingSKUs[sku] {
			errorsList = append(errorsList, map[string]interface{}{
				"row":   pp.RowNum,
				"error": fmt.Sprintf("SKU '%s' already exists in database", sku),
			})
			continue
		}

		// Map categories
		var categoryIDs []uuid.UUID
		categorySet := make(map[uuid.UUID]bool)
		allCatsFound := true

		for _, catName := range pp.CategoryNames {
			cat, ok := categoryCache[catName]
			if !ok {
				if !autoCreateCategories {
					errorsList = append(errorsList, map[string]interface{}{
						"row":   pp.RowNum,
						"error": fmt.Sprintf("Category '%s' not found", catName),
					})
					allCatsFound = false
					break
				}
			}

			if !categorySet[cat.ID] {
				categoryIDs = append(categoryIDs, cat.ID)
				categorySet[cat.ID] = true
				for _, ancestor := range cat.Ancestors {
					if !categorySet[ancestor] {
						categoryIDs = append(categoryIDs, ancestor)
						categorySet[ancestor] = true
					}
				}
			}
		}

		if !allCatsFound {
			continue
		}

		// Upload image to S3
		imageURL := strings.TrimSpace(pp.Row[index["imageurl"]])
		var imageURLs []string

		if imageURL != "" {
			uploadedURL, err := s.uploadImageFromURL(ctx, imageURL, sku, 0)
			if err != nil {
				errorsList = append(errorsList, map[string]interface{}{
					"row":     pp.RowNum,
					"warning": fmt.Sprintf("Failed to upload image, product created without image: %v", err),
				})
			} else {
				imageURLs = append(imageURLs, uploadedURL)
			}
		}

		now := time.Now().UTC()
		product := models.Product{
			ID:          uuid.New(),
			Name:        name,
			Price:       price,
			Quantity:    quantity,
			Description: strings.TrimSpace(pp.Row[index["description"]]),
			Images:      imageURLs,
			Brand:       strings.TrimSpace(pp.Row[index["brand"]]),
			SKU:         sku,
			IsFeatured:  isFeatured,
			CategoryIDs: categoryIDs,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		productsToInsert = append(productsToInsert, product)
	}

	if len(productsToInsert) > 0 {
		_, err := s.productRepo.CreateMany(ctx, productsToInsert)
		if err != nil {
			return nil, err
		}
	}

	return &models.BulkImportResult{
		InsertedCount: len(productsToInsert),
		ErrorsCount:   len(errorsList),
		Errors:        errorsList,
		Message:       "Bulk import process completed",
	}, nil
}

func (s *ProductService) uploadImageFromURL(ctx context.Context, imageURL, sku string, index int) (string, error) {
	resp, err := http.Get(imageURL)
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
