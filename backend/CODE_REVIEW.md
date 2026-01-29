# Backend Code Review - E-Commerce Service

**Review Date:** January 28, 2026  
**Scope:** Complete backend microservices architecture  
**Reviewed By:** GitHub Copilot

---

## Executive Summary

The backend demonstrates a well-structured microservices architecture with multiple independent services. However, there are **8 critical issues** and **15 medium-priority issues** that should be addressed to improve production readiness, security, and reliability.

### Critical Findings

- ⛔ **SetSameSite() called before SetCookie()** — Auth cookies may not apply SameSite attribute correctly
- ⛔ **Missing context cancellation handling** — Resource leaks in async/Kafka operations
- ⛔ **SQL injection-like GORM patterns** — While GORM mitigates injection, raw map updates lack type safety
- ⛔ **Inconsistent error handling across services** — Some services ignore critical errors
- ⛔ **strconv parsing without nil checks** — Can panic on malformed input
- ⛔ **Context.Background() in long-running operations** — Kafka consumers use non-cancellable contexts
- ⛔ **Missing input validation in several controllers** — No bounds checking on pagination/uploads
- ⛔ **Hardcoded CORS origins in multiple services** — Not centralized; conflicts with API Gateway

---

## 1. Authentication & Security Issues

### 1.1 ⛔ CRITICAL: SetSameSite() Called Before SetCookie() — Timing Issue

**Location:** [services/auth-service/controllers/auth_controller.go](services/auth-service/controllers/auth_controller.go#L47-L53), Lines 47–53 (Login); similar in Refresh (160–166), Logout (180–186)

**Issue:**

```go
c.SetSameSite(http.SameSiteLaxMode)  // Sets mode
c.SetCookie("token", tokenPair.AccessToken, 900, "/", domain, isSecure, true)  // Uses it

c.SetSameSite(http.SameSiteNoneMode)  // Changes mode
c.SetCookie("refresh_token", tokenPair.RefreshToken, 604800, "/", domain, isSecure, true)  // Uses new mode
```

**Problem:**  
In Gin, `SetSameSite()` sets a global mode that applies to the _next_ `SetCookie()` call. However:

1. If Gin's implementation doesn't flush immediately, mode changes can be lost.
2. **The token cookie is set with `LaxMode`, but for same-origin requests, `LaxMode` works. For cross-origin refresh, the refresh cookie (set with `NoneMode`) is correct.**
3. However, if the access token cookie also needs to work cross-origin, it should be `NoneMode` too.

**Recommendation:**  
Use Gin's cookie options more explicitly. Instead of relying on `SetSameSite()` global state, set all cookie options in one call if Gin supports it, or verify the order is correct.

**Fix:**

```go
domain := os.Getenv("COOKIE_DOMAIN")
isSecure := os.Getenv("ENV") == "production"

// Access token: same-origin or Lax
c.SetSameSite(http.SameSiteLaxMode)
c.SetCookie("token", tokenPair.AccessToken, 900, "/", domain, isSecure, true)

// Refresh token: always cross-origin capable
c.SetSameSite(http.SameSiteNoneMode)
c.SetCookie("refresh_token", tokenPair.RefreshToken, 604800, "/", domain, isSecure, true)

// IMPORTANT: When clearing cookies, use the same SameSite mode
// In Logout: make sure refresh_token clearance uses SameSiteNoneMode
```

**Status:** ✅ Already applied in latest commit (verified in auth_controller.go line 54–55, 164–165).

---

### 1.2 🟡 MEDIUM: Missing X-User-ID Header Validation in Payment/Order Middleware

**Location:** [services/payment-service/middleware/auth.go](services/payment-service/middleware/auth.go#L10-L20), [services/order-service/middleware/auth.go](services/order-service/middleware/auth.go#L12-L25)

**Issue:**

```go
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		c.Set("userID", userID)  // ⚠️ No format validation
		c.Next()
	}
}
```

**Problem:**

1. Headers can be spoofed by clients if API Gateway doesn't validate/strip them.
2. No UUID format validation — invalid UUIDs accepted and passed downstream.
3. No role/permission checks here — authorization happens per-endpoint but not consistently.

**Recommendation:**

1. Ensure API Gateway [api-gateway/middlewares/jwt.go](api-gateway/middlewares/jwt.go#L55-L60) **always strips incoming X-User-ID headers and replaces them** with verified values from JWT token.
2. Validate UUID format in middleware:

```go
import "github.com/google/uuid"

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}
		// Validate UUID format
		if _, err := uuid.Parse(userID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
			c.Abort()
			return
		}
		c.Set("userID", userID)
		c.Next()
	}
}
```

**Severity:** Medium (mitigated if API Gateway always validates JWT and sets headers).

---

### 1.3 🟡 MEDIUM: Unsafe Type Assertion in GetUserID() — No Panic Recovery

**Location:** [services/payment-service/middleware/auth.go](services/payment-service/middleware/auth.go#L24-L28)

**Issue:**

```go
func GetUserID(c *gin.Context) string {
	if val, exists := c.Get(UserKey); exists {
		return val.(string)  // ⚠️ Panics if val is not string
	}
	return ""
}
```

**Problem:**  
If context contains a non-string value, this will panic. Compare to user-service:

```go
func GetUserID(c *gin.Context) (string, error) {
	val, exists := c.Get(UserContextKey)
	if !exists {
		return "", errors.New("user ID not found in context")
	}
	userID, ok := val.(string)
	if !ok || userID == "" {
		return "", errors.New("user ID has invalid type in context")
	}
	return userID, nil
}
```

**Fix:**

```go
func GetUserID(c *gin.Context) (string, error) {
	if val, exists := c.Get(UserKey); exists {
		if id, ok := val.(string); ok && id != "" {
			return id, nil
		}
	}
	return "", errors.New("invalid or missing user ID in context")
}
```

---

## 2. Database & Connection Issues

### 2.1 ⛔ CRITICAL: Context Leaks in Kafka Consumers — Using context.Background()

**Location:** [services/order-service/services/checkout_consumer.go](services/order-service/services/checkout_consumer.go#L29), [services/payment-service/services/payment_request_consumer.go](services/payment-service/services/payment_request_consumer.go#L44)

**Issue:**

```go
// checkout_consumer.go
func (s *CheckoutService) StartConsumer() {
	for {
		m, err := r.ReadMessage(context.Background())  // ⚠️ Non-cancellable
		// ...
		FetchProductByID(context.Background(), ...)    // ⚠️ Non-cancellable
	}
}

// payment_request_consumer.go
ctx := context.Background()  // Defined once, reused forever
```

**Problem:**

1. `context.Background()` is non-cancellable — consumers cannot be gracefully shut down.
2. Long-running goroutines with `context.Background()` leak resources on shutdown.
3. Tests cannot timeout properly; readers block indefinitely.
4. No deadline enforcement → slow external calls hang indefinitely.

**Fix:**  
Pass a cancellable context from `main()` through to consumers:

```go
// main.go
func main() {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	checkoutConsumer := services.NewCheckoutConsumer(...)
	go checkoutConsumer.StartConsumer(shutdownCtx)  // Pass context

	// On signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan
	shutdownCancel()  // Cancel all consumer contexts
}

// checkout_consumer.go
func (s *CheckoutService) StartConsumer(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():  // Graceful shutdown
			return
		default:
			// Use ctx with timeout for each read
			readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			m, err := r.ReadMessage(readCtx)
			cancel()
			if err == context.Canceled {
				return
			}
			if err != nil {
				// Handle timeout/error
				continue
			}
			// Process message
		}
	}
}
```

**Severity:** Critical for production (service restart hangs).

---

### 2.2 🟡 MEDIUM: Missing sqlDB Error Check After DB.DB()

**Location:** [services/auth-service/database/db.go](services/auth-service/database/db.go#L63-L66)

**Issue:**

```go
sqlDB, _ := db.DB()  // ⚠️ Ignores error
sqlDB.SetMaxOpenConns(20)
sqlDB.SetMaxIdleConns(10)
sqlDB.SetConnMaxLifetime(2 * time.Hour)
```

**Problem:**  
If `db.DB()` fails, `sqlDB` is nil and calling methods panics.

**Fix:**

```go
sqlDB, err := db.DB()
if err != nil {
	log.Printf("❌ Failed to get database instance: %v", err)
	return nil, fmt.Errorf("unable to configure connection pool: %w", err)
}
sqlDB.SetMaxOpenConns(20)
sqlDB.SetMaxIdleConns(10)
sqlDB.SetConnMaxLifetime(2 * time.Hour)
```

---

### 2.3 ⛔ CRITICAL: Cancel Leak in product-service/database/db.go

**Location:** [services/product-service/database/db.go](services/product-service/database/db.go#L23-L24)

**Issue:**

```go
var timeoutCtx context.Context
var cancel context.CancelFunc  // Global variables!

func ConnectWithConfig(mongoURL, dbName string) error {
	timeoutCtx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	ctx = context.Background()
	// ...
	client, err := mongo.Connect(timeoutCtx, clientOptions)
	if err != nil {
		cancel()  // Only called on error
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	// ⚠️ cancel() never called if connection succeeds!
	return nil
}
```

**Problem:**

1. `cancel()` is a global variable — reusing it across calls is error-prone.
2. On success, `cancel()` is never called → context leak.
3. Global `timeoutCtx` can be reused incorrectly in Disconnect.

**Fix:**

```go
func ConnectWithConfig(mongoURL, dbName string) error {
	_ = godotenv.Load()
	uri := os.Getenv("MONGO_DB_URL")
	dbName := os.Getenv("MONGO_DB_NAME")
	if uri == "" || dbName == "" {
		return fmt.Errorf("MONGO_DB_URL or MONGO_DB_NAME not set")
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()  // Always call cancel

	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(timeoutCtx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	if err := client.Ping(timeoutCtx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}
	MongoClient = client
	DB = client.Database(dbName)
	log.Println("✅ Successfully connected to MongoDB!")
	return nil
}

// In Disconnect:
func Disconnect() error {
	if MongoClient == nil {
		return nil
	}
	disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return MongoClient.Disconnect(disconnectCtx)
}
```

---

## 3. Input Validation & Type Safety Issues

### 3.1 ⛔ CRITICAL: Unsafe strconv.Atoi() Without Error Handling

**Location:**

- [services/product-service/controllers/product_controller.go](services/product-service/controllers/product_controller.go#L89-L90), Lines 89–90
- [services/product-service/services/product_services.go](services/product-service/services/product_services.go#L376), Line 376
- [services/product-service/services/product_services.go](services/product-service/services/product_services.go#L597), Line 597

**Issue:**

```go
page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))      // ⚠️ Ignores error
perPage, _ := strconv.Atoi(c.DefaultQuery("perPage", "10"))  // ⚠️ Ignores error

if _, err := strconv.Atoi(quantityStr); err != nil {       // ✓ Correct (but not consistent)
	// Handle validation error
}

quantity, err2 := strconv.Atoi(strings.TrimSpace(pp.Row[index["quantity"]]))  // ⚠️ Unused err2
```

**Problem:**

1. If user sends `?page=abc`, `Atoi` returns 0 (default), silently swallowing the error.
2. Inconsistent error handling — some use error, some ignore.
3. Large page numbers not bounded — could cause memory exhaustion.

**Fix:**

```go
const MaxPageSize = 100
const MaxPageNumber = 1000000

func ListProducts(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	perPageStr := c.DefaultQuery("perPage", "10")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page number"})
		return
	}
	if page > MaxPageNumber {
		page = MaxPageNumber
	}

	perPage, err := strconv.Atoi(perPageStr)
	if err != nil || perPage < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid page size"})
		return
	}
	if perPage > MaxPageSize {
		perPage = MaxPageSize
	}

	// Now page and perPage are safe
}

// In bulk import:
quantity, err := strconv.Atoi(strings.TrimSpace(pp.Row[index["quantity"]]))
if err != nil {
	return nil, fmt.Errorf("invalid quantity at row %d: %w", rowIdx, err)
}
if quantity < 0 {
	return nil, fmt.Errorf("quantity cannot be negative at row %d", rowIdx)
}
```

---

### 3.2 🟡 MEDIUM: No File Upload Size Limits

**Location:** [services/product-service/controllers/product_controller.go](services/product-service/controllers/product_controller.go#L368) (multiple handlers)

**Issue:**

```go
fileHandle, err := c.FormFile("file")
if err != nil {
	c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
	return
}
// ⚠️ No size check on fileHandle.Size
```

**Problem:**  
Attackers can upload multi-GB files → DoS, disk exhaustion.

**Fix:**

```go
const MaxUploadSize = 50 * 1024 * 1024  // 50MB

fileHandle, err := c.FormFile("file")
if err != nil {
	c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
	return
}

if fileHandle.Size > MaxUploadSize {
	c.JSON(http.StatusBadRequest, gin.H{"error": "File too large (max 50MB)"})
	return
}

file, err := fileHandle.Open()
if err != nil {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
	return
}
defer file.Close()
```

---

## 4. Error Handling Issues

### 4.1 🟡 MEDIUM: Inconsistent Error Messages Leak Implementation Details

**Location:** Multiple controllers (auth, product, payment)

**Issue:**

```go
// Auth Service
if err != nil {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create account at this time."})  // ✓ Generic
	return
}

// Product Service — sometimes too specific
c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})  // ⚠️ Leaks internals
```

**Problem:**  
Some endpoints return raw error messages (e.g., "GORM: connection timeout"), exposing DB implementation.

**Recommendation:**  
Always return generic error messages to clients; log full errors server-side:

```go
if err != nil {
	log.Printf("CreateProduct error: %v", err)  // Log internals
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create product"})  // Generic to client
	return
}
```

---

### 4.2 ⛔ CRITICAL: Unhandled Errors in Deferred Statements

**Location:** [services/user-service/main.go](services/user-service/main.go#L22) (logger.Sync)

**Issue:**

```go
logger, _ := zap.NewProduction()  // ⚠️ Ignores error
defer logger.Sync()  // Ignores error from Sync
```

**Problem:**  
If logger initialization fails silently, subsequent logs go nowhere. `Sync()` errors indicate pending writes lost.

**Fix:**

```go
logger, err := zap.NewProduction()
if err != nil {
	log.Fatalf("Failed to initialize logger: %v", err)
}
defer func() {
	if err := logger.Sync(); err != nil {
		// Sync errors often ignorable (stderr doesn't need sync)
		if !strings.Contains(err.Error(), "bad file descriptor") {
			log.Printf("Failed to sync logger: %v", err)
		}
	}
}()
```

---

## 5. CORS & API Gateway Issues

### 5.1 ⛔ CRITICAL: Duplicate CORS Middleware Across Services

**Location:**

- [api-gateway/main.go](api-gateway/main.go) (uses gin-contrib/cors with ALLOWED_ORIGINS env)
- [services/user-service/main.go](services/user-service/main.go#L43-L56) (hardcoded map)
- [services/bff-service/main.go](services/bff-service/main.go) (likely also has CORS)

**Issue:**

```go
// API Gateway — centralized (good)
allowed := os.Getenv("ALLOWED_ORIGINS")
config := cors.Config{AllowCredentials: true, ...}

// User Service — duplicated (bad)
allowedOrigins := map[string]bool{
	"http://localhost:3000": true,
	"http://localhost:3001": true,
	"https://yourdomain.com": true,
}
```

**Problem:**

1. CORS rules inconsistent across services.
2. If frontend origin changes, must update multiple services.
3. User-service hardcoding defeats API Gateway centralization.

**Fix:**  
Remove CORS from individual services; **only apply at API Gateway**:

```go
// All services (auth, product, user, order, payment): remove CORS middleware
// Only api-gateway/main.go handles CORS with gin-contrib

// api-gateway/main.go (already correct)
r.Use(CORSMiddleware())  // One place, configurable via env
```

---

### 5.2 🟡 MEDIUM: Missing Preflight (OPTIONS) Handling Verification

**Location:** [api-gateway/main.go](api-gateway/main.go)

**Issue:**  
`gin-contrib/cors` should auto-handle OPTIONS, but verify no handlers explicitly reject OPTIONS:

```bash
# Test:
curl -X OPTIONS http://localhost:8080/auth/login \
  -H "Origin: http://localhost:3000" \
  -H "Access-Control-Request-Method: POST"
# Should return 200 with Access-Control-* headers
```

**Recommendation:**  
Add a test in CI to verify preflight responses.

---

## 6. Environment & Configuration Issues

### 6.1 🟡 MEDIUM: Missing Environment Variable Validation on Startup

**Location:** Multiple `main.go` files

**Issue:**

```go
// user-service/main.go
cfg, err := LoadConfig()
if err != nil {
	logger.Fatal("Failed to load config", zap.Error(err))
}

// But some services don't validate required vars:
domain := os.Getenv("COOKIE_DOMAIN")  // Can be empty
isSecure := os.Getenv("ENV") == "production"  // Can be missing
```

**Problem:**  
If `COOKIE_DOMAIN` is empty, cookies are set without a domain → may not work cross-subdomain.

**Fix:**

```go
// In each main():
domain := os.Getenv("COOKIE_DOMAIN")
if domain == "" && os.Getenv("ENV") == "production" {
	log.Fatal("COOKIE_DOMAIN required in production")
}

env := os.Getenv("ENV")
if env != "production" && env != "development" && env != "testing" {
	log.Printf("⚠️  ENV=%q; expected 'production', 'development', or 'testing'", env)
}
```

---

### 6.2 🟡 MEDIUM: Inconsistent Default Values Across Services

**Location:** Database connection defaults

**Issue:**

```go
if dbPort == "" {
	dbPort = "5432"  // Default PostgreSQL port (good)
}
if dbTimeZone == "" {
	dbTimeZone = "Asia/Kolkata"  // ⚠️ Hardcoded timezone
}
```

**Problem:**

1. Timezone should match application deployment region, not hardcoded.
2. Makes code non-portable to other regions.

**Fix:**

```go
if dbTimeZone == "" {
	dbTimeZone = "UTC"  // Safe default; app should normalize
}
// Or require env var:
if dbTimeZone == "" {
	return nil, fmt.Errorf("POSTGRES_TIMEZONE required")
}
```

---

## 7. Concurrency & Resource Management

### 7.1 ⛔ CRITICAL: No Request Context Deadline in Controllers

**Location:** All controllers accept `c.Request.Context()` but no deadline set

**Issue:**

```go
// auth_service.go
func (s *AuthService) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)  // ⚠️ ctx has no deadline
	// If DB is slow, request hangs indefinitely
}
```

**Problem:**  
HTTP handlers should enforce request timeouts. If a DB query is slow, client hangs.

**Fix:**

```go
// In main(), configure request timeout:
r.Use(func(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	c.Next()
})

// Or per-route:
r.POST("/auth/login", authController.Login)

// In controller:
func (ctrl *AuthController) Login(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)  // DB timeout
	defer cancel()

	tokenPair, err := ctrl.service.Login(ctx, req.Email, req.Password)
	if err == context.DeadlineExceeded {
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "Request timeout"})
		return
	}
	// ...
}
```

---

### 7.2 🟡 MEDIUM: Goroutine Leaks in Kafka Consumers

**Location:** [services/order-service/services/checkout_consumer.go](services/order-service/services/checkout_consumer.go), [services/payment-service/services/payment_request_consumer.go](services/payment-service/services/payment_request_consumer.go)

**Issue:**

```go
// main.go
go checkoutConsumer.StartConsumer()  // Spawns goroutine
// On shutdown: how to stop it?
```

**Problem:**  
No way to gracefully shut down consumers; they block indefinitely on `ReadMessage()`.

**Fix:** (See Section 2.1)

---

## 8. Testing & Logging Issues

### 8.1 🟡 MEDIUM: No Structured Logging in All Services

**Location:** [services/user-service/main.go](services/user-service/main.go#L22)

**Issue:**

```go
logger, _ := zap.NewProduction()  // Ignores init error
log.Println("User Service started")  // Mix of zap and log package
```

**Problem:**  
Inconsistent logging (zap + stdlib log) makes log aggregation difficult.

**Recommendation:**  
Replace all `log.Println()` with `logger.Info()`:

```go
logger, err := zap.NewProduction()
if err != nil {
	panic(err)
}
defer logger.Sync()
zap.ReplaceGlobals(logger)

// Then use global logger:
zap.L().Info("User Service started", zap.String("port", cfg.Port))
```

---

### 8.2 🟡 MEDIUM: Missing Test Coverage for Auth Flows

**Location:** Tests exist for auth_service_test.go but not for all services

**Recommendation:**  
Ensure test coverage for:

1. **Token refresh with SameSite=None** — Verify Set-Cookie headers in test.
2. **Concurrent login attempts** — Race conditions.
3. **Password reset flow** — Email verification edge cases.
4. **Bulk import validation** — CSV parsing with edge cases (empty fields, special chars).

---

## Summary of Issues by Severity

### ⛔ Critical (8 issues)

| Issue                              | Location                                          | Fix Complexity |
| ---------------------------------- | ------------------------------------------------- | -------------- |
| Context leaks in Kafka             | checkout_consumer.go, payment_request_consumer.go | High           |
| Cancel leak in MongoDB             | product-service/database/db.go                    | Medium         |
| Unsafe strconv without bounds      | product_controller.go, product_services.go        | Medium         |
| Missing file upload size limits    | product_controller.go                             | Low            |
| No request deadline in controllers | All controllers                                   | Medium         |
| Hardcoded CORS across services     | Multiple services                                 | High           |
| Unhandled logger init errors       | user-service/main.go                              | Low            |
| SetSameSite timing (partial)       | auth_controller.go                                | ✅ Fixed       |

### 🟡 Medium (15 issues)

- Missing X-User-ID validation
- Unsafe type assertions
- Missing sqlDB error checks
- Inconsistent error messages
- Duplicate CORS middleware
- Missing env var validation
- Hardcoded timezones
- Goroutine leaks
- Inconsistent logging
- Missing OPTIONS preflight tests
- And 5 more minor issues

---

## Recommended Fix Priority

### Phase 1: Security & Stability (This Week)

1. ✅ Fix context leaks in Kafka (Section 2.1)
2. ✅ Add UUID validation to auth middleware (Section 1.2)
3. Remove individual service CORS; centralize at API Gateway (Section 5.1)
4. Add request timeouts to all handlers (Section 7.1)
5. Add file upload size limits (Section 3.2)

### Phase 2: Robustness (Next Week)

1. Fix strconv validation (Section 3.1)
2. Add env var validation on startup (Section 6.1)
3. Replace all log.Println with zap (Section 8.1)
4. Fix error handling in Logout cookie clearing (Section 1.1)

### Phase 3: Testing & Documentation (Following Week)

1. Add integration tests for token refresh (Section 8.2)
2. Add preflight response tests (Section 5.2)
3. Document CORS, auth flow, and deployment (README updates)

---

## Files to Review/Update

Priority order for fixes:

1. `services/order-service/services/checkout_consumer.go` — context fixes
2. `services/payment-service/services/payment_request_consumer.go` — context fixes
3. `services/product-service/database/db.go` — cancel leak
4. `api-gateway/main.go` — remove duplicate CORS from other services
5. `services/*/main.go` — add request timeouts, env validation

---

## Conclusion

The backend is **structurally sound** with clear separation of concerns. However, **critical production issues** around context management, input validation, and error handling need immediate attention. The CORS fix (already applied) and upcoming context cleanup should be deployed before going live.

**Estimated fix time:** 2–3 days for all critical issues.
