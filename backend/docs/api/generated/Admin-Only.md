# Admin Only API

Endpoints:

- **POST** /products — 🔐 Create a new product
- **PUT** /products/{id} — 🔐 Update a product
- **DELETE** /products/{id} — 🔐 Delete a product
- **GET** /products/presign — 🔐 Get a presigned S3 upload URL for a new product (by SKU)
- **POST** /products/{id}/images/presign — 🔐 Get a presigned S3 upload URL for an existing product image
- **POST** /products/bulk/validate — 🔐 Validate a product bulk import CSV (dry run)
- **POST** /products/bulk — 🔐 Bulk import products from CSV
- **GET** /products/bulk/jobs/{id} — 🔐 Get bulk import job status and result
- **POST** /products/bulk/delete — 🔐 Bulk delete products
- **POST** /categories — 🔐 Create a category
- **PUT** /categories/{id} — 🔐 Update a category
- **DELETE** /categories/{id} — 🔐 Delete a category
- **GET** /orders/admin — 🔐 Admin — list all orders across all users
- **GET** /inventory/{productId} — 🔐 Get stock level for a product
- **PUT** /inventory/{productId} — 🔐 Update stock quantity for a product
- **POST** /inventory — 🔐 Create or upsert stock record for a product
- **POST** /coupons — 🔐 Create a coupon
- **GET** /coupons — 🔐 List all coupons
- **DELETE** /coupons/{code} — 🔐 Deactivate a coupon