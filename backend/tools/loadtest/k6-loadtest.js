// k6 load test for the ShopSwift API gateway (local stack only).
//
// Usage:
//   k6 run --env GATEWAY_URL=http://localhost:8080 k6-loadtest.js
//
// Optional (skips the login scenario when unset):
//   LOADTEST_EMAIL=alice.johnson@shopswift-demo.test
//   LOADTEST_PASSWORD=Demo123!
//
// Thresholds are tuned for a local Docker stack on a dev laptop. Re-baseline
// on your hardware before raising VUs (see baseline-notes.md).

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend, Rate } from 'k6/metrics';

const GATEWAY = __ENV.GATEWAY_URL || 'http://localhost:8080';
const EMAIL = __ENV.LOADTEST_EMAIL || '';
const PASSWORD = __ENV.LOADTEST_PASSWORD || '';

const loginFailRate = new Rate('login_failed');
const loginTrend = new Trend('login_duration');
const healthTrend = new Trend('health_duration');
const productsTrend = new Trend('products_duration');

export const options = {
  stages: [
    { duration: '30s', target: 5 },   // ramp-up
    { duration: '1m', target: 10 },   // steady
    { duration: '20s', target: 0 },   // ramp-down
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],            // < 1% errors overall
    http_req_duration: ['p(95)<2000'],         // global safety net
    health_duration: ['p(95)<500'],            // health must stay snappy
    products_duration: ['p(95)<1500'],
    login_duration: ['p(95)<2000'],
    login_failed: ['rate<0.05'],
  },
};

export default function () {
  // 1. Health — unauthenticated, must always be fast and 200.
  const healthStart = Date.now();
  const healthRes = http.get(`${GATEWAY}/health`);
  healthTrend.add(Date.now() - healthStart);
  check(healthRes, {
    'health status is 200': (r) => r.status === 200,
  });

  // 2. Product listing — unauthenticated catalog read through the gateway.
  const productsStart = Date.now();
  const productsRes = http.get(`${GATEWAY}/products`);
  productsTrend.add(Date.now() - productsStart);
  check(productsRes, {
    'products status is 200': (r) => r.status === 200,
  });

  // 3. Login — only when credentials are provided (seeded demo user).
  if (EMAIL && PASSWORD) {
    const loginStart = Date.now();
    const loginRes = http.post(
      `${GATEWAY}/auth/login`,
      JSON.stringify({ email: EMAIL, password: PASSWORD }),
      { headers: { 'Content-Type': 'application/json' } },
    );
    loginTrend.add(Date.now() - loginStart);
    const ok = check(loginRes, {
      'login status is 200': (r) => r.status === 200,
    });
    loginFailRate.add(!ok);
    if (!ok) {
      console.warn(`login failed: status=${loginRes.status} body=${loginRes.body}`);
    }
  } else {
    console.warn('LOADTEST_EMAIL/LOADTEST_PASSWORD not set — skipping login scenario');
  }

  sleep(0.5);
}
