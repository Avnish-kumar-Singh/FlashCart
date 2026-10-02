# FlashCart — Marketplace + Flash Sale + Razorpay Demo

A Flipkart/Amazon-inspired full-stack marketplace built with **Go, React/Vite, PostgreSQL, Redis, Kafka/Redpanda, Prometheus, Grafana and Razorpay Test Mode**.

See [the enterprise admin architecture blueprint](docs/ENTERPRISE_ADMIN_BLUEPRINT.md) for target service boundaries, workflows, data domains, security controls, deployment guidance, verification gates, and phased rollout. It distinguishes current implementation from planned production capabilities.

## What was upgraded

### Storefront
- Amazon/Flipkart-inspired header, category navigation and dense marketplace UI.
- Homepage hero campaign, dynamic announcement strip, category lanes, flash-deal banner and recommendations.
- Product listing with search, category pills, brand filter, price filter and sorting.
- Product detail page with offers, delivery information, ratings/reviews presentation, wishlist and sharing.
- Floating AI shopping assistant for catalog-grounded product suggestions and comparisons.
- Cart, wishlist, order history and order tracking.
- Account page with saved delivery addresses.
- Coupon support with demo coupon `FLASH10` (10% off, minimum ₹500).
- Responsive mobile layout.

### Payments
- Razorpay **Test Mode** checkout.
- Server-side Razorpay order creation.
- Server-side HMAC signature verification.
- Payment records in PostgreSQL.
- Razorpay webhook endpoint.
- Admin payment monitoring.
- COD remains available for demo orders.
- Razorpay Checkout is explicitly configured to show a UPI block (including scan-and-pay QR) plus cards, net banking and wallets — you still need those methods turned on in your Razorpay test dashboard for them to actually work, but the frontend no longer relies solely on Razorpay's default block layout.

### Admin
- Dashboard / sales overview / users / orders / revenue.
- Product CRUD and inventory.
- Flash-sale management.
- Announcement and broadcast queue.
- Flash-sale analytics.
- Recent order operations.
- Payment monitoring.

### AI shopping assistant

- Set `GROQ_API_KEY` in the project-root `.env`; it is read only by the Go API and must never be added to a `VITE_` variable. `GROQ_MODEL` defaults to `openai/gpt-oss-120b`.
- Restart/rebuild the API after changing the key: `docker compose up -d --build api`.
- The assistant uses the active search/category/brand or product page plus up to eight matching catalog records. Requests are limited to eight per client IP per minute.
- This demo has no verified review feed. The assistant explicitly reports that customer-review summaries are unavailable rather than generating review claims.
- Avoid entering personal or payment information into the assistant. User messages and relevant catalog context are sent to Groq for completion.

## Manual setup — required once

1. Create/sign in to a Razorpay account and switch to **Test Mode**.
2. Generate a **Test Key ID** and **Test Key Secret**.
3. Copy `.env.example` to `.env` at the project root.
4. Replace:
   - `RAZORPAY_KEY_ID`
   - `RAZORPAY_KEY_SECRET`
   - `RAZORPAY_WEBHOOK_SECRET` (optional for local Checkout verification; recommended when configuring webhooks)
5. Restart the API:
   ```powershell
   docker compose up -d --build
   ```
6. If your existing PostgreSQL volume is from an older FlashCart version, the API now self-creates the new payment/address/coupon tables at startup. You do **not** need to delete the database volume.
7. Start frontend:
   ```powershell
   cd frontend
   npm install
   npm run dev
   ```
8. Open `http://localhost:5173`.

### Razorpay test flow

1. Register/login as a normal FlashCart user.
2. Add a product to cart.
3. Go to checkout.
4. Select **Razorpay**.
5. Apply `FLASH10` if the cart is at least ₹500.
6. Click Pay.
7. Razorpay Checkout opens.
8. Complete a test payment using Razorpay's official test credentials.
9. FlashCart sends `razorpay_order_id`, `razorpay_payment_id` and `razorpay_signature` to the Go API.
10. The Go API verifies the signature with the server-side secret and changes:
   `PAYMENT_PENDING -> PAID -> CONFIRMED`.

The order worker deliberately does **not** auto-charge Razorpay orders with the old simulator.

## Important security notes

- Never place `RAZORPAY_KEY_SECRET` in React/Vite environment variables.
- Only the public Razorpay Key ID is sent to the browser.
- Never mark an order paid merely because the browser says the payment succeeded.
- Signature verification is performed on the Go server.
- For production, configure Razorpay webhooks and verify webhook signatures.
- Test Mode is for demo/testing; no real customer money should be used.

### Invoices

- A confirmed order gets one immutable, numbered PDF invoice from server-side order and customer records. Product tax rates are configured as tax included in the listed price, so the invoice total matches the amount charged.
- The confirmation screen offers **Download PDF** and **Print Invoice**. Invoice endpoints require the signed-in order owner and return private, non-cacheable PDFs.
- Invoice creation and email dispatch are written together to a PostgreSQL outbox. The notification worker polls this outbox, leases jobs safely across replicas, and retries SMTP failures with capped exponential backoff. A recovery scan also creates invoices for newly confirmed orders if an event is missed.
- The notification worker emails the PDF attachment through SMTP with STARTTLS. Set `SMTP_HOST`, `SMTP_PORT` (usually `587`), `SMTP_USERNAME`, `SMTP_PASSWORD`, and `SMTP_FROM` in the project `.env`, then run `docker compose up -d --build notificationworker`.
- Without SMTP settings, invoices are still generated and available to download/print; email jobs stay queued and begin retrying after SMTP is configured. SMTP acceptance is retryable, but final inbox delivery depends on the provider, recipient server, and spam filtering. Use a real tax rate appropriate to each product and jurisdiction.

### Coupons

- Coupon offers unlock in checkout at a server-calculated cart subtotal of ₹500. Seeded codes include `SBI10`, `HDFC12`, `ICICI8`, `AXIS10`, `PHONEPE50`, `PAYTM30`, `FESTIVE10`, `NEWUSER15`, `GROCERY5`, `SNACKS10`, `SAVE50`, `SAVE100`, and `FLASH10`.
- Offers are listed and previewed from current database prices and cart contents. Checkout locks and revalidates the coupon, max-use count, category subtotal, first-order eligibility, payment rail, and customer redemption before writing the order. One coupon is accepted per order, with per-user redemption history and admin usage analytics.
- Bank and wallet coupons require a matching instrument/issuer selection; Razorpay payment metadata is checked after payment. A mismatch triggers a full refund, releases inventory and the coupon reservation, and cancels the order. Actual issuer metadata availability depends on the Razorpay account/payment method configuration.
- Category offers match the product category exactly (case-insensitive). Assign products to `Grocery` or `Snacks` in Admin > Catalog for those codes to apply.

## Existing distributed-systems stack

```text
React/Vite
   |
   v
Go API ---- PostgreSQL
   |           |
   |           +-- products / users / orders / payments / coupons / addresses
   |
   +---- Redis -------- cart / flash-sale inventory / idempotency
   |
   +---- Kafka/Redpanda
             |
             +---- Order Worker
             +---- Notification Worker
```

Razorpay sits beside the order/payment boundary:

```text
Checkout
  |
  +--> FlashCart order (PAYMENT_PENDING)
  |
  +--> Razorpay Order
          |
          +--> Razorpay Checkout
                    |
                    +--> payment_id + signature
                              |
                              v
                       Go verification
                              |
                              v
                       PAID -> CONFIRMED
```

## Admin login

Development defaults:

```text
Email:    admin@flashcart.local
Password: Admin@12345
```

Change them before any non-demo deployment.

## Monitoring

- Prometheus: `http://localhost:9091`
- Grafana: `http://localhost:3000`
- API health: `http://localhost:8080/healthz`

## Build/test

Backend:
```powershell
go mod tidy
go test ./...
go vet ./...
```

Frontend:
```powershell
cd frontend
npm install
npm run build
```

## Provider-dependent features

OTP login, Google/Apple/social login, real email delivery, WhatsApp Business messaging, live bank offers/cashback and production refund automation require provider credentials/configuration. The demo UI is prepared for these workflows, but no fake credentials or fake successful external transactions are claimed as real integrations.

## Festival automation

- `festivalworker` syncs national/regional public holidays for the current and next year, schedules seven-day reminders, starts festival flash sales at local midnight, and restores unsold stock at the next local midnight. Flash-sale prices are temporary Redis prices; base product prices are not overwritten.
- India uses Calendarific because Nager.Date currently does not support `IN`. Obtain a Calendarific API key and set `CALENDARIFIC_API_KEY` in the project-root `.env`. `HOLIDAY_CALENDAR_PROVIDER=nager` is available for countries supported by Nager.Date.
- Defaults are `HOLIDAY_COUNTRY_CODE=IN`, `FESTIVAL_TIMEZONE=Asia/Kolkata`, `FESTIVAL_DISCOUNT_PERCENT=10`, `FESTIVAL_STOCK_PER_PRODUCT=10`, and `FESTIVAL_PRODUCTS_PER_EVENT=20`. Adjust the limits in `.env` as needed.
- Generate VAPID credentials locally with `go run ./cmd/vapidkeys`. Put `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY`, and `VAPID_SUBJECT` in `.env`. Keep the private key secret. Customers must opt in through the festival banner to receive browser push.
- Festival email uses SMTP, SMS uses Twilio, and browser push uses VAPID subscriptions. Configure those providers separately; unavailable channels are logged and are not represented as delivered.
- Start/rebuild the automated services with `docker compose up -d --build`. Calendar sync status and sale activity are persisted in PostgreSQL and the festival worker is scraped by Prometheus.

## Festival automation

- The `festivalworker` syncs Calendarific public holidays for `HOLIDAY_COUNTRY_CODE` (default `IN`) for the current and following year, stores national/regional events, sends seven-day reminders, starts configured flash sales at local midnight, and restores unsold stock at the next local midnight. Festival sale prices live in Redis; catalog prices are not permanently rewritten.
- Calendarific is the default because Nager.Date does not currently list India as a supported country. Create a Calendarific API key and set `CALENDARIFIC_API_KEY`; optionally set `HOLIDAY_CALENDAR_PROVIDER=nager` only for a country supported by Nager.Date.
- Default automation settings: `FESTIVAL_TIMEZONE=Asia/Kolkata`, `FESTIVAL_DISCOUNT_PERCENT=10`, `FESTIVAL_STOCK_PER_PRODUCT=10`, `FESTIVAL_PRODUCTS_PER_EVENT=20`. Override these in the project `.env` before starting with `docker compose up -d --build`.
- Generate browser push credentials locally with `go run ./cmd/vapidkeys`, put the output into `.env` as `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY`, and set `VAPID_SUBJECT` to a contact URI. Never expose or commit the private key. Customers must opt in to browser notifications from the festival banner.
- Festival announcements and seasonal accents appear on the storefront automatically. Email requires SMTP and SMS requires Twilio settings; without a configured channel the worker logs that channel as unavailable. Browser push requires VAPID credentials plus user opt-in.
