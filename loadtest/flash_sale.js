import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

export const options = {
  scenarios: {
    flash_sale: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 1000),
      timeUnit: '1s',
      duration: __ENV.DURATION || '30s',
      preAllocatedVUs: Number(__ENV.VUS || 100),
      maxVUs: Number(__ENV.MAX_VUS || 2000),
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
  },
};

const failed = new Rate('flash_sale_failed');
const tokens = (__ENV.TOKENS || '').split(',').map((s) => s.trim()).filter(Boolean);

export default function () {
  if (!__ENV.BASE_URL || !__ENV.SALE_ID || tokens.length === 0) {
    throw new Error('Set BASE_URL, SALE_ID and TOKENS (comma-separated authenticated access tokens).');
  }

  const token = tokens[(__VU - 1) % tokens.length];
  const idempotencyKey = `k6-${__VU}-${__ITER}`;
  const res = http.post(`${__ENV.BASE_URL}/flash-sales/${__ENV.SALE_ID}/buy`, JSON.stringify({ quantity: 1 }), {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    },
    tags: { endpoint: 'flash_sale_buy' },
  });

  const ok = check(res, {
    'accepted or expected rejection': (r) => [202, 200, 409, 410, 425, 429].includes(r.status),
  });
  failed.add(!ok);
  sleep(0.01);
}
