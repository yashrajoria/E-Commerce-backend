package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/yashrajoria/product-service/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProductRepository struct {
	collection *mongo.Collection
}

func NewProductRepository(db *mongo.Database) *ProductRepository {
	return &ProductRepository{
		collection: db.Collection("products"),
	}
}

func (r *ProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	filter := bson.M{"_id": id, "deleted_at": bson.M{"$exists": false}}
	var product models.Product
	err := r.collection.FindOne(ctx, filter).Decode(&product)
	return &product, err
}

func (r *ProductRepository) Find(ctx context.Context, filter bson.M, findOptions *options.FindOptions) ([]*models.Product, error) {
	cursor, err := r.collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var products []*models.Product
	if err = cursor.All(ctx, &products); err != nil {
		return nil, err
	}
	return products, nil
}

func (r *ProductRepository) Count(ctx context.Context, filter bson.M) (int64, error) {
	return r.collection.CountDocuments(ctx, filter)
}

func (r *ProductRepository) Create(ctx context.Context, product *models.Product) (*mongo.InsertOneResult, error) {
	return r.collection.InsertOne(ctx, product)
}

func (r *ProductRepository) CreateMany(ctx context.Context, products []interface{}) (*mongo.InsertManyResult, error) {
	return r.collection.InsertMany(ctx, products)
}

func (r *ProductRepository) Update(ctx context.Context, id uuid.UUID, updates bson.M) (*mongo.UpdateResult, error) {
	filter := bson.M{"_id": id, "deleted_at": bson.M{"$exists": false}}
	updates["updated_at"] = time.Now().UTC()
	updateQuery := bson.M{"$set": updates}
	return r.collection.UpdateOne(ctx, filter, updateQuery)
}

// Delete performs a soft delete.
func (r *ProductRepository) Delete(ctx context.Context, id uuid.UUID) (*mongo.UpdateResult, error) {
	filter := bson.M{"_id": id, "deleted_at": bson.M{"$exists": false}}
	update := bson.M{"$set": bson.M{"deleted_at": time.Now().UTC()}}
	return r.collection.UpdateOne(ctx, filter, update)
}

func (r *ProductRepository) FindByIDInternal(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	// This filter intentionally omits the "deleted_at" check
	filter := bson.M{"_id": id}
	var product models.Product
	err := r.collection.FindOne(ctx, filter).Decode(&product)
	return &product, err
}
