# Inventory Service API

Endpoints:

- **GET** /inventory/{productId} — 🔐 Get stock level for a product
- **PUT** /inventory/{productId} — 🔐 Update stock quantity for a product
- **POST** /inventory — 🔐 Create or upsert stock record for a product
- **POST** /inventory/check — Check availability for multiple items
- **POST** /inventory/reserve — Reserve stock for an order (idempotent)
- **POST** /inventory/release — Release previously reserved stock
- **POST** /inventory/confirm — Confirm reserved stock after successful payment