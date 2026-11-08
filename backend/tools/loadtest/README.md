# ⚡ Load Test Toolkit — ShopSwift

Load tests for the ShopSwift gateway and flash sale path, written for [k6](https://k6.io/).

> ⚠️ **SAFETY WARNING**: Run these only against local or staging stacks. Never run them against production or unapproved endpoints.

---

## 📋 Test Suites

### 1. Gateway Baseline (`k6-loadtest.js`)
Ramps 5 → 10 virtual users over about 2 minutes. Each iteration calls:
- `GET /health`
- `GET /products`
- `POST /auth/login`, only when `LOADTEST_EMAIL` and `LOADTEST_PASSWORD` are set

Thresholds:
- `http_req_failed` rate `< 1%`
- `health_duration` p95 `< 500 ms`
- `products_duration` p95 `< 1500 ms`
- `login_duration` p95 `< 2000 ms`
- `http_req_duration` p95 `< 2000 ms` (overall)

### 2. Flash Sale Contention (`k6-flashsale.js`)
100 VUs compete for 25 slots on product `flash-sale-sku-pro` (override with `FLASH_PRODUCT_ID`).

Flow:
1. `POST /inventory/flash-sale/configure`: sets `total_slots: 25` and `lease_ttl_seconds: 120`.
2. `POST /inventory/flash-sale/enter`: each VU joins the waiting room.
3. `GET /inventory/flash-sale/status`: VUs poll their position.
4. `POST /inventory/flash-sale/claim`: admitted VUs claim their slot.

Checks:
- Exactly 25 leases are granted (`flash_leases_granted` count = 25). This checks for overselling.
- `flash_enter_duration` p95 `< 150 ms`.

---

## 🚀 Prerequisites

1. **Local stack running**:
   ```bash
   cd backend && ./scripts/dev-up.sh
   ```

2. **Install k6**:
   ```bash
   # macOS
   brew install k6
   ```
   For Linux, see the install steps at [k6.io](https://k6.io/docs/get-started/installation/).

3. **Demo credentials (optional, enables the login scenario)**:
   ```bash
   export LOADTEST_EMAIL=alice.johnson@shopswift-demo.test
   export LOADTEST_PASSWORD=Demo123!
   ```

---

## 🏃 Running the Tests

Launcher (runs a gateway health check first):
```bash
cd backend/tools/loadtest
./run-loadtest.sh
```

Or run one script directly:
```bash
k6 run --env GATEWAY_URL=http://localhost:8080 k6-loadtest.js
k6 run --env GATEWAY_URL=http://localhost:8080 k6-flashsale.js
```

The flash sale suite needs admin rights for `/configure`. Check the script for the token it sends before running it against a non-local stack.

---

## 📊 Recording Results

Record results in your own notes or CI. This repo does not include a `baseline-notes.md` file.
