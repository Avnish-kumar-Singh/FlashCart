# FlashCart Upgrade — Manual Setup Checklist

## 1. Razorpay Test Mode
- Create/login to Razorpay.
- Switch to Test Mode.
- Generate API keys.
- Copy the Test Key ID and Test Key Secret into root `.env`.
- Never commit `.env`.

Example:
```env
RAZORPAY_KEY_ID=rzp_test_xxxxxxxxx
RAZORPAY_KEY_SECRET=xxxxxxxxxxxxxxxx
RAZORPAY_WEBHOOK_SECRET=xxxxxxxxxxxxxxxx
```

## 2. Restart backend
From the project root:
```powershell
docker compose down
docker compose up -d --build
```

Do NOT use `docker compose down -v` unless you intentionally want to erase your local PostgreSQL data.

The API now creates the new payment, address and coupon tables at startup, so old FlashCart database volumes are upgraded automatically.

## 3. Frontend
```powershell
cd frontend
npm install
npm run dev
```

Open:
`http://localhost:5173`

## 4. Test account
Development admin:
```text
admin@flashcart.local
Admin@12345
```

Change these credentials before any real deployment.

## 5. Test marketplace
- Open Products.
- Use category/brand/price filters.
- Open a product.
- Add to wishlist/cart.
- Open Account and add an address.
- Checkout.

## 6. Test coupon
Use:
```text
FLASH10
```
Minimum order:
```text
₹500
```
Discount:
```text
10%
```

## 7. Test Razorpay
- Select Razorpay at checkout.
- Click Pay.
- Razorpay Checkout opens.
- Use the official Razorpay Test Mode payment details from their documentation/dashboard.
- After success, FlashCart verifies the signature on the Go backend.
- Order should become `CONFIRMED`.
- Admin → Payments should show the captured transaction.

## 8. Test refund
- Login as admin.
- Open Admin → Payments.
- Refund a captured Test Mode payment.
- Verify the payment becomes `REFUNDED`.

## 9. Production-only manual work
These are intentionally NOT faked:
- OTP provider
- Google/Apple/social login credentials
- Real email provider
- WhatsApp Business API credentials
- production Razorpay live keys
- production webhook URL/HTTPS
- production refund approval/audit workflow
- real bank/cashback offer contracts
