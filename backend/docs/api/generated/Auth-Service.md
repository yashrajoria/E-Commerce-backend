# Auth Service API

Endpoints:

- **POST** /auth/register — Register a new user account
- **POST** /auth/login — Login and receive auth cookies
- **POST** /auth/logout — Logout and clear auth cookies
- **POST** /auth/refresh — Refresh access token using the refresh cookie
- **GET** /auth/status — Check authentication status
- **POST** /auth/verify-email — Verify email address with OTP code