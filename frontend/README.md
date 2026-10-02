# FlashCart Frontend

Production-minded React/Vite storefront for the FlashCart Go distributed e-commerce backend.

## Stack
- React 19 + Vite
- React Router
- Axios
- Responsive custom CSS

## Local setup
```bash
npm install
npm run dev
```
Open http://localhost:5173.

The Vite dev server proxies `/api/*` to `http://localhost:8080/*`.
For a deployed frontend, set `VITE_API_URL` to the public Go API URL.

## Admin
The backend bootstraps a development admin account from:
- email: `admin@flashcart.local`
- password: `Admin@12345`

Change these values in `docker-compose.yml` before any real deployment.

Sign in with the admin account. The Admin link appears in the header and opens `/admin`.

The admin console can:
- create products
- edit products
- delete products
- view catalog stock
- view customer/order/revenue counters
- view recent orders
- activate flash sales

## Flash sale
After activating a sale from the Admin console, copy its `sale_id` into:

```env
VITE_FLASH_SALE_ID=<sale-id>
```

Then restart the Vite dev server.

## API contract
The frontend uses the backend endpoints:
- `/healthz`
- `/users/register`, `/users/login`, `/users/refresh`
- `/products/`
- `/cart/`
- `/cart/items`
- `/cart/checkout`
- `/flash-sales/activate`
- `/flash-sales/{saleID}`
- `/flash-sales/{saleID}/buy`
- `/orders/{id}`
- `/admin/stats`
- `/admin/orders`

All authenticated requests automatically receive the JWT from localStorage. Flash-sale and checkout requests generate idempotency keys.

## Authentication note

The frontend automatically retries a protected API request once when an access token expires, using the stored refresh token. If the refresh token is also expired or invalid, the stored session is cleared and the user should sign in again.

For local admin testing, use the development admin configured by the backend environment variables:
- Email: `admin@flashcart.local`
- Password: `Admin@12345`
