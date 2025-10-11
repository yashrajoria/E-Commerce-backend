package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"product-service/models"
	"product-service/repository"

	"github.com/cloudinary/cloudinary-go"
	"github.com/cloudinary/cloudinary-go/api/uploader"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ListProductsParams defines the parameters for listing products.
type ListProductsParams struct {
	Page       int
	PerPage    int
	IsFeatured *bool // Use a pointer to distinguish between false and not set
	CategoryID uuid.UUID
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
	productRepo  *repository.ProductRepository
	categoryRepo *repository.CategoryRepository
	cld          *cloudinary.Cloudinary
}

func NewProductService(pr *repository.ProductRepository, cr *repository.CategoryRepository, cld *cloudinary.Cloudinary) *ProductService {
	return &ProductService{
		productRepo:  pr,
		categoryRepo: cr,
		cld:          cld,
	}
}

func (s *ProductService) GetProduct(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	return s.productRepo.FindByID(ctx, id)
}

func (s *ProductService) ListProducts(ctx context.Context, params ListProductsParams) ([]*models.Product, int64, error) {
	filter := bson.M{"deleted_at": bson.M{"$exists": false}}
	if params.IsFeatured != nil {
		filter["is_featured"] = *params.IsFeatured
	}
	if params.CategoryID != uuid.Nil {
		filter["category_ids"] = params.CategoryID
	}

	findOptions := options.Find().
		SetLimit(int64(params.PerPage)).
		SetSkip(int64((params.Page - 1) * params.PerPage))

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

	// Step 2: Upload images to Cloudinary
	var imageURLs []string
	for i, fileHeader := range images {
		file, err := fileHeader.Open()
		if err != nil {
			continue // Or return error
		}
		defer file.Close()
		uploadParams := uploader.UploadParams{
			PublicID: fmt.Sprintf("product_img_%s_%d", req.SKU, i),
			Folder:   "ecommerce/products",
		}
		uploadResp, err := s.cld.Upload.Upload(ctx, file, uploadParams)
		if err != nil {
			continue // Or return error
		}
		imageURLs = append(imageURLs, uploadResp.SecureURL)
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

func (s *ProductService) CreateBulkProducts(ctx context.Context, file multipart.File) (int, []map[string]interface{}, error) {
	r := csv.NewReader(file)
	headers, err := r.Read()
	if err != nil {
		return 0, nil, fmt.Errorf("CSV must include a header row")
	}

	// Create a map for header indexes for flexible column order.
	index := make(map[string]int)
	for i, h := range headers {
		index[strings.ToLower(strings.TrimSpace(h))] = i
	}

	// --- 1. Single Pass: Read file, collect data and all unique category names ---
	type pendingProduct struct {
		Row           []string
		RowNum        int
		CategoryNames []string
	}
	var pendingProducts []pendingProduct
	categoryNamesSet := make(map[string]bool)
	var errorsList []map[string]interface{}
	rowNum := 2 // Start after header
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

		rawCategories := strings.Split(row[index["categories"]], ",")
		var currentCatNames []string
		for _, cName := range rawCategories {
			trimmed := strings.TrimSpace(cName)
			if trimmed != "" {
				categoryNamesSet[trimmed] = true
				currentCatNames = append(currentCatNames, trimmed)
			}
		}
		pendingProducts = append(pendingProducts, pendingProduct{Row: row, RowNum: rowNum, CategoryNames: currentCatNames})
		rowNum++
	}

	// --- 2. Prefetch all required categories in a single DB query ---
	var catNames []string
	for name := range categoryNamesSet {
		catNames = append(catNames, name)
	}
	categories, err := s.categoryRepo.FindByNames(ctx, catNames)
	if err != nil {
		return 0, errorsList, err
	}
	categoryCache := make(map[string]models.Category)
	for _, cat := range categories {
		categoryCache[cat.Name] = cat
	}

	// --- 3. Process in-memory data to build final product list for insertion ---
	var productsToInsert []interface{}
	for _, pp := range pendingProducts {
		// Parse and validate data from pp.Row
		name := strings.TrimSpace(pp.Row[index["name"]])
		price, err1 := strconv.ParseFloat(strings.TrimSpace(pp.Row[index["price"]]), 64)
		quantity, err2 := strconv.Atoi(strings.TrimSpace(pp.Row[index["quantity"]]))
		isFeatured, err3 := strconv.ParseBool(strings.TrimSpace(pp.Row[index["is_featured"]]))

		if name == "" || err1 != nil || err2 != nil || err3 != nil {
			errorsList = append(errorsList, map[string]interface{}{"row": pp.RowNum, "error": "Invalid data format (name, price, quantity, or is_featured)"})
			continue
		}

		// Map categories using the pre-fetched cache
		var categoryIDs []uuid.UUID
		categorySet := make(map[uuid.UUID]bool)
		allCatsFound := true
		for _, name := range pp.CategoryNames {
			cat, ok := categoryCache[name]
			if !ok {
				errorsList = append(errorsList, map[string]interface{}{"row": pp.RowNum, "error": fmt.Sprintf("Category '%s' not found", name)})
				allCatsFound = false
				break // Stop processing categories for this row
			}
			if !categorySet[cat.ID] { // Handle ancestors and duplicates
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
			continue // Skip this product and move to the next
		}

		now := time.Now().UTC()
		product := models.Product{
			ID:          uuid.New(),
			Name:        name,
			Price:       price,
			Quantity:    quantity,
			Description: strings.TrimSpace(pp.Row[index["description"]]),
			Images:      []string{strings.TrimSpace(pp.Row[index["imageurl"]])},
			Brand:       strings.TrimSpace(pp.Row[index["brand"]]),
			SKU:         strings.TrimSpace(pp.Row[index["sku"]]),
			IsFeatured:  isFeatured,
			CategoryIDs: categoryIDs,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		productsToInsert = append(productsToInsert, product)
	}

	// --- 4. Perform a single bulk insert operation ---
	if len(productsToInsert) > 0 {
		_, err := s.productRepo.CreateMany(ctx, productsToInsert)
		if err != nil {
			// You could try to parse the bulk write error to find which documents failed
			return 0, errorsList, err
		}
	}

	return len(productsToInsert), errorsList, nil
}
