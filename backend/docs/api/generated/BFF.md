# BFF API

Endpoints:

- **GET** /bff/home — Home page — aggregated products & categories
- **GET** /bff/profile — Profile page — user profile & order history
- **POST** /bff/auth/register — Register a new user account
- **POST** /bff/auth/login — Login and receive auth cookies
- **POST** /bff/auth/logout — Logout and clear auth cookies
- **POST** /bff/auth/refresh — Refresh the access token using the refresh cookie
- **GET** /bff/auth/status — Check authentication status
- **POST** /bff/auth/verify-email — Verify email address with OTP code
- **POST** /bff/auth/resend-verification — Resend email verification code
- **GET** /bff/products — List products with filtering, sorting, and pagination
- **GET** /bff/products/{id} — Get a single product by ID
- **GET** /bff/categories — Get the full category tree
- **GET** /bff/cart — Get the current user's cart
- **POST** /bff/cart/add — Add one or more items to the cart
- **DELETE** /bff/cart/remove/{product_id} — Remove an item from the cart
- **DELETE** /bff/cart/clear — Clear all items from the cart
- **POST** /bff/cart/checkout — Initiate checkout for the current cart
- **POST** /bff/checkout — (Deprecated) Initiate checkout — alias for POST /bff/cart/checkout
- **GET** /bff/orders — List the current user's orders
- **GET** /bff/orders/{id} — Get order detail
- **PUT** /bff/users/profile — Update the authenticated user's profile
- **POST** /bff/users/change-password — Change the authenticated user's password
- **GET** /bff/payment/status/by-order/{order_id} — Get payment status for an order
- **POST** /bff/promotions/validate — Validate a promotion coupon
- **POST** /bff/payment/verify-payment — Verify payment after Stripe redirect