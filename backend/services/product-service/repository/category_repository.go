package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// This can be in the same file as ProductRepository or separate. For simplicity, showing it here.

type CategoryRepository struct {
	collection        *mongo.Collection
	productCollection *mongo.Collection
}

func NewCategoryRepository(db *mongo.Database) *CategoryRepository {
	return &CategoryRepository{
		collection:        db.Collection("categories"),
		productCollection: db.Collection("products"),
	}
}

func (r *CategoryRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	filter := bson.M{"_id": id, "deleted_at": bson.M{"$exists": false}}
	var category models.Category
	err := r.collection.FindOne(ctx, filter).Decode(&category)
	return &category, err
}

func (r *CategoryRepository) FindByName(ctx context.Context, name string) (*models.Category, error) {
	filter := bson.M{"name": name, "deleted_at": bson.M{"$exists": false}}
	var category models.Category
	err := r.collection.FindOne(ctx, filter).Decode(&category)
	return &category, err
}

func (r *CategoryRepository) FindByNames(ctx context.Context, names []string) ([]models.Category, error) {
	if len(names) == 0 {
		return []models.Category{}, nil
	}
	filter := bson.M{"name": bson.M{"$in": names}, "deleted_at": bson.M{"$exists": false}}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var categories []models.Category
	if err = cursor.All(ctx, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *CategoryRepository) FindAll(ctx context.Context) ([]models.Category, error) {
	filter := bson.M{"deleted_at": bson.M{"$exists": false}}
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var categories []models.Category
	if err = cursor.All(ctx, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *CategoryRepository) Create(ctx context.Context, category *models.Category) (*mongo.InsertOneResult, error) {
	return r.collection.InsertOne(ctx, category)
}

func (r *CategoryRepository) Update(ctx context.Context, id uuid.UUID, updates bson.M) (*mongo.UpdateResult, error) {
	filter := bson.M{"_id": id, "deleted_at": bson.M{"$exists": false}}
	updates["updated_at"] = time.Now().UTC()
	updateQuery := bson.M{"$set": updates}
	return r.collection.UpdateOne(ctx, filter, updateQuery)
}

func (r *CategoryRepository) Delete(ctx context.Context, id uuid.UUID) (*mongo.UpdateResult, error) {
	filter := bson.M{"_id": id, "deleted_at": bson.M{"$exists": false}}
	update := bson.M{"$set": bson.M{"deleted_at": time.Now().UTC()}}
	return r.collection.UpdateOne(ctx, filter, update)
}

// Check if any products are associated with a category
func (r *CategoryRepository) HasProducts(ctx context.Context, id uuid.UUID) (bool, error) {
	filter := bson.M{
		"category_ids": id,
		"deleted_at":   bson.M{"$exists": false},
	}
	count, err := r.productCollection.CountDocuments(ctx, filter)
	return count > 0, err
}
