// k6 load test for the ShopSwift Flash Sale Virtual Waiting Room & FairPass Token Lease.
//
// Usage:
//   k6 run --env GATEWAY_URL=http://localhost:8080 k6-flashsale.js
//
// Demonstrates mathematically strict concurrency control:
//   - Exactly N limited inventory slots (e.g. 25 slots)
//   - 100+ concurrent Virtual Users (VUs) rushing simultaneously
//   - Zero over-subscription / race conditions
//   - Sub-100ms Redis Lua response times under contention

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend, Rate } from 'k6/metrics';

const GATEWAY = __ENV.GATEWAY_URL || 'http://localhost:8080';
const PRODUCT_ID = __ENV.FLASH_PRODUCT_ID || 'flash-sale-sku-pro';
const TOTAL_SLOTS = 25;

const leasesGranted = new Counter('flash_leases_granted');
const usersQueued = new Counter('flash_users_queued');
const claimsSucceeded = new Counter('flash_claims_succeeded');
const flashDuration = new Trend('flash_enter_duration');
const queueFailRate = new Rate('flash_error_rate');

export const options = {
  scenarios: {
    flash_rush: {
      executor: 'per-vu-iterations',
      vus: 100, // 100 concurrent buyers competing for 25 slots
      iterations: 1,
      maxDuration: '30s',
    },
  },
  thresholds: {
    flash_error_rate: ['rate<0.01'],         // < 1% HTTP/system failures
    flash_enter_duration: ['p(95)<150'],     // p95 latency under 150ms
    flash_leases_granted: [`count==${TOTAL_SLOTS}`], // Invariant: exactly 25 slots granted
  },
};

// 1. Setup: Configure flash sale with exactly TOTAL_SLOTS
export function setup() {
  const configUrl = `${GATEWAY}/inventory/flash-sale/configure`;
  const payload = JSON.stringify({
    product_id: PRODUCT_ID,
    active: true,
    total_slots: TOTAL_SLOTS,
    lease_ttl_seconds: 120,
  });

  const res = http.post(configUrl, payload, {
    headers: {
      'Content-Type': 'application/json',
      'X-User-Role': 'admin',
      'X-User-ID': 'admin-bootstrap',
    },
  });

  check(res, {
    'flash sale configured': (r) => r.status === 200,
  });

  return { productId: PRODUCT_ID };
}

// 2. Default execution: Each VU tries to acquire a lease simultaneously
export default function (data) {
  const vuId = `buyer-vu-${__VU}-${Date.now()}`;
  const enterUrl = `${GATEWAY}/inventory/flash-sale/enter`;
  const enterPayload = JSON.stringify({
    product_id: data.productId,
    quantity: 1,
    user_id: vuId,
  });

  const start = Date.now();
  const res = http.post(enterUrl, enterPayload, {
    headers: { 'Content-Type': 'application/json' },
  });
  flashDuration.add(Date.now() - start);

  const ok = check(res, {
    'enter response status is 200': (r) => r.status === 200,
  });

  if (!ok) {
    queueFailRate.add(1);
    return;
  }

  queueFailRate.add(0);

  const body = JSON.parse(res.body);

  if (body.status === 'GRANTED') {
    leasesGranted.add(1);
    check(body, {
      'has lease token': (b) => !!b.lease_token,
      'has expires_at': (b) => !!b.expires_at,
    });

    // Simulate finalizing checkout: claim the lease token
    const claimUrl = `${GATEWAY}/inventory/flash-sale/claim`;
    const claimPayload = JSON.stringify({
      product_id: data.productId,
      user_id: vuId,
      lease_token: body.lease_token,
    });

    const claimRes = http.post(claimUrl, claimPayload, {
      headers: {
        'Content-Type': 'application/json',
        'X-User-ID': vuId,
      },
    });

    const claimOk = check(claimRes, {
      'claim status is 200': (r) => r.status === 200,
    });
    if (claimOk) {
      claimsSucceeded.add(1);
    }
  } else if (body.status === 'QUEUED') {
    usersQueued.add(1);
    check(body, {
      'has queue position': (b) => b.position > 0,
      'has total in queue': (b) => b.total_in_queue > 0,
    });

    // Simulate waiting customer checking their queue rank
    sleep(0.1);
    const statusUrl = `${GATEWAY}/inventory/flash-sale/status?product_id=${data.productId}&user_id=${vuId}`;
    const statusRes = http.get(statusUrl);
    check(statusRes, {
      'status poll returns 200': (r) => r.status === 200,
    });
  }
}

// 3. Teardown: Clean up and log final results
export function teardown(data) {
  const resetUrl = `${GATEWAY}/inventory/flash-sale/reset?product_id=${data.productId}`;
  http.post(resetUrl, null, {
    headers: {
      'X-User-Role': 'admin',
      'X-User-ID': 'admin-bootstrap',
    },
  });
}
