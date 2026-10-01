# Product Service API

Endpoints:

- **GET** /products — List products
- **POST** /products — 🔐 Create a new product
- **GET** /products/{id} — Get product by ID
- **PUT** /products/{id} — 🔐 Update a product
- **DELETE** /products/{id} — 🔐 Delete a product
- **GET** /products/presign — 🔐 Get a presigned S3 upload URL for a new product (by SKU)
- **POST** /products/{id}/images/presign — 🔐 Get a presigned S3 upload URL for an existing product image
- **POST** /products/bulk/validate — 🔐 Validate a product bulk import CSV (dry run)
- **POST** /products/bulk — 🔐 Bulk import products from CSV
- **GET** /products/bulk/jobs/{id} — 🔐 Get bulk import job status and result
- **POST** /products/bulk/delete — 🔐 Bulk delete products
- **GET** /products/internal/{id} — Internal — fetch product DTO by ID (service-to-service only)
- **GET** /categories — Get full category tree
- **POST** /categories — 🔐 Create a category
- **PUT** /categories/{id} — 🔐 Update a category
- **DELETE** /categories/{id} — 🔐 Delete a category