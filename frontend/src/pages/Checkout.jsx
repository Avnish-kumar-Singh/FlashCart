import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import QRCode from 'qrcode'
import { cartApi, paymentApi } from '../services/api'
import { useCart } from '../context/CartContext'
import Icon from '../components/Icon'

const PAYMENT_METHODS = [
  {
    value: 'RAZORPAY',
    label: 'Razorpay',
    icon: '◈',
    detail: 'UPI · Cards · Wallets · Net Banking · QR',
  },
  {
    value: 'UPI',
    label: 'UPI ID',
    icon: '✓',
    detail: 'Pay by UPI ID or QR scan',
  },
  {
    value: 'QR',
    label: 'QR Code',
    icon: '◫',
    detail: 'Generate QR and pay the current amount',
  },
  {
    value: 'CARD',
    label: 'Card',
    icon: '▣',
    detail: 'Debit/Credit card payment',
  },
  {
    value: 'NETBANKING',
    label: 'Net Banking',
    icon: '◌',
    detail: 'Bank transfer via Razorpay',
  },
  {
    value: 'WALLET',
    label: 'Wallet',
    icon: '◍',
    detail: 'Pay via supported digital wallet',
  },
  {
    value: 'COD',
    label: 'Cash on Delivery',
    icon: '▣',
    detail: 'Pay when delivered',
  },
]

function loadRazorpay() {
  return new Promise((resolve, reject) => {
    if (window.Razorpay) {
      return resolve(true)
    }

    const script = document.createElement('script')

    script.src = 'https://checkout.razorpay.com/v1/checkout.js'

    script.onload = () => resolve(true)

    script.onerror = () =>
      reject(
        new Error(
          'Could not load Razorpay Checkout. Check your internet connection.'
        )
      )

    document.body.appendChild(script)
  })
}

export default function Checkout() {
  const { items, refreshCart } = useCart()
  const navigate = useNavigate()
  const checkoutCompleted = useRef(false)

  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')

  const [address, setAddress] = useState({
    name: '',
    phone: '',
    line: '',
    city: '',
    state: '',
    pin: '',
  })

  const [paymentMethod, setPaymentMethod] = useState('RAZORPAY')
  const [upiId, setUpiId] = useState('')
  const [qrCode, setQrCode] = useState('')

  const [coupon, setCoupon] = useState('')
  const [couponApplied, setCouponApplied] = useState(false)
  const [couponQuote, setCouponQuote] = useState(null)
  const [couponOffers, setCouponOffers] = useState([])
  const [couponError, setCouponError] = useState('')
  const [couponBusy, setCouponBusy] = useState(false)
  const [paymentIssuer, setPaymentIssuer] = useState('')

  const subtotal = useMemo(
    () =>
      items.reduce(
        (sum, item) => {
          const unitPrice = item.unit_price_paise ?? item.price_paise ?? 0
          return sum + Number(unitPrice) * Number(item.quantity || 0)
        },
        0
      ),
    [items]
  )

  const discount = couponApplied ? Number(couponQuote?.discount_paise || 0) : 0

  const total = subtotal - discount
  const isRazorpayFlow = paymentMethod !== 'COD'

  useEffect(() => {
    if (!items.length && !checkoutCompleted.current) {
      navigate('/cart')
    }
  }, [items.length, navigate])

  useEffect(() => {
    let active = true
    cartApi.coupons({ payment_method: paymentMethod, issuer: paymentIssuer })
      .then((response) => { if (active) setCouponOffers(response.data.offers || []) })
      .catch(() => { if (active) setCouponOffers([]) })
    return () => { active = false }
  }, [items, subtotal, paymentMethod, paymentIssuer])

  const applyCoupon = async (code = coupon) => {
    const normalizedCode = code.trim().toUpperCase()
    if (!normalizedCode) return
    setCouponBusy(true)
    setCouponError('')
    try {
      const response = await cartApi.validateCoupon({
        code: normalizedCode,
        payment_method: paymentMethod,
        issuer: paymentIssuer,
      })
      setCoupon(normalizedCode)
      setCouponQuote(response.data)
      setCouponApplied(true)
    } catch (err) {
      setCouponApplied(false)
      setCouponQuote(null)
      setCouponError(err.response?.data?.error || 'This coupon could not be applied.')
    } finally {
      setCouponBusy(false)
    }
  }

  const createOrder = async () => {
    const key = crypto.randomUUID()
    const orderPaymentMethod = paymentMethod === 'COD' ? 'COD' : 'RAZORPAY'

    const r = await cartApi.checkout(
      key,
      orderPaymentMethod,
      couponApplied ? coupon.trim().toUpperCase() : '',
      paymentMethod,
      paymentIssuer,
    )

    return r.data
  }

  const generateQrCode = async (order) => {
    const upiHandle = (upiId || 'flashcart@upi').trim() || 'flashcart@upi'
    const amount = (total / 100).toFixed(2)
    const url = `upi://pay?pa=${encodeURIComponent(upiHandle)}&pn=${encodeURIComponent('FlashCart')}&am=${amount}&cu=INR&tn=${encodeURIComponent(`FlashCart order ${order.order_id}`)}`

    try {
      const dataUrl = await QRCode.toDataURL(url, {
        margin: 2,
        scale: 8,
        width: 280,
      })
      setQrCode(dataUrl)
      setBusy(false)
      setMsg('')
    } catch (err) {
      setBusy(false)
      setMsg('Could not generate the QR code for this bill. Please try another payment method.')
    }
  }

  const startRazorpay = async (order) => {
    await loadRazorpay()

    const { data: payment } =
      await paymentApi.createRazorpayOrder(order.order_id)

    const razorpayMethodConfig = (() => {
      if (paymentMethod === 'UPI') {
        return {
          method: 'upi',
          upi: {
            flow: 'collect',
            vpa: (upiId || '').trim() || undefined,
          },
        }
      }
      if (paymentMethod === 'QR') {
        return {
          method: 'upi',
          upi: {
            flow: 'qr',
          },
        }
      }
      if (paymentMethod === 'CARD') {
        return { method: 'card' }
      }
      if (paymentMethod === 'NETBANKING') {
        return { method: 'netbanking' }
      }
      if (paymentMethod === 'WALLET') {
        return { method: 'wallet' }
      }
      return {}
    })()

    const options = {
      key: payment.key_id,

      amount: payment.amount,

      currency: payment.currency || 'INR',

      name: 'FlashCart',

      description: `FlashCart order ${order.order_id.slice(0, 8)}`,

      order_id: payment.razorpay_order_id,

      ...razorpayMethodConfig,

      theme: {
        color: '#2874f0',
      },

      modal: {
        ondismiss: () => {
          setBusy(false)
        },
      },

      prefill: {
        name: address.name,
        contact: address.phone,
      },

      notes: {
        flashcart_order_id: order.order_id,
      },

      handler: async (response) => {
        try {
          setMsg('Verifying payment securely…')

          await paymentApi.verifyRazorpay({
            order_id: order.order_id,

            razorpay_order_id: response.razorpay_order_id,

            razorpay_payment_id: response.razorpay_payment_id,

            razorpay_signature: response.razorpay_signature,
          })

          checkoutCompleted.current = true
          navigate(`/orders/${order.order_id}?paid=1`)
          refreshCart().catch(() => {})
        } catch (err) {
          setBusy(false)

          setMsg(
            err.response?.data?.error ||
              'Payment was received but verification failed. Please contact support with your order ID.'
          )
        }
      },
    }

    const razorpay = new window.Razorpay(options)

    razorpay.on('payment.failed', (response) => {
      setBusy(false)

      setMsg(
        response.error?.description ||
          'Razorpay payment failed. You can retry from this checkout.'
      )
    })

    razorpay.open()
  }

  const submit = async (e) => {
    e.preventDefault()

    if (!items.length) {
      return
    }

    setBusy(true)
    setMsg('')

    try {
      const order = await createOrder()

      if (paymentMethod === 'QR') {
        await generateQrCode(order)
      } else if (isRazorpayFlow) {
        await startRazorpay(order)
      } else {
        checkoutCompleted.current = true
        navigate(`/orders/${order.order_id}`)
        refreshCart().catch(() => {})
      }
    } catch (err) {
      setBusy(false)

      setMsg(
        err.response?.data?.error ||
          'Checkout failed. Please try again.'
      )
    }
  }

  return (
    <section className="page checkout-page">
      <div className="container checkout">

        <div>

          <div className="checkout-crumbs">
            <span>Cart</span>
            <b>›</b>
            <span>Address</span>
            <b>›</b>
            <strong>Payment</strong>
          </div>

          <div className="eyebrow">
            Secure checkout
          </div>

          <h1>
            Complete your order.
          </h1>

          <p className="muted">
            Fast checkout with server-created orders,
            idempotent requests and Razorpay test-mode
            payment verification.
          </p>

          <form
            onSubmit={submit}
            className="form-card checkout-form"
          >

            {/* =========================
                DELIVERY ADDRESS
            ========================== */}

            <div className="section-number">
              <span>1</span>

              <div>
                <h2>Delivery address</h2>

                <small>
                  Where should we deliver your order?
                </small>
              </div>
            </div>

            <div className="form-grid">

              <label>
                Full name

                <input
                  required
                  value={address.name}
                  onChange={(e) =>
                    setAddress({
                      ...address,
                      name: e.target.value,
                    })
                  }
                  placeholder="Avnish Kumar"
                />
              </label>

              <label>
                Mobile number

                <input
                  required
                  inputMode="numeric"
                  value={address.phone}
                  onChange={(e) =>
                    setAddress({
                      ...address,
                      phone: e.target.value,
                    })
                  }
                  placeholder="10-digit mobile"
                />
              </label>

              <label className="full">
                Address

                <input
                  required
                  value={address.line}
                  onChange={(e) =>
                    setAddress({
                      ...address,
                      line: e.target.value,
                    })
                  }
                  placeholder="House / flat, street, area"
                />
              </label>

              <label>
                City

                <input
                  required
                  value={address.city}
                  onChange={(e) =>
                    setAddress({
                      ...address,
                      city: e.target.value,
                    })
                  }
                  placeholder="Pune"
                />
              </label>

              <label>
                State

                <input
                  required
                  value={address.state}
                  onChange={(e) =>
                    setAddress({
                      ...address,
                      state: e.target.value,
                    })
                  }
                  placeholder="Maharashtra"
                />
              </label>

              <label>
                PIN code

                <input
                  required
                  inputMode="numeric"
                  value={address.pin}
                  onChange={(e) =>
                    setAddress({
                      ...address,
                      pin: e.target.value,
                    })
                  }
                  placeholder="411001"
                />
              </label>

            </div>

            <div className="checkout-divider" />

            {/* =========================
                PAYMENT METHOD
            ========================== */}

            <div className="section-number">
              <span>2</span>

              <div>
                <h2>Payment method</h2>

                <small>
                  Choose how you want to pay
                </small>
              </div>
            </div>

            <div className="payment-options checkout-payment-options">

              {PAYMENT_METHODS.map((m) => (
                <button
                  key={m.value}
                  type="button"
                  className={`payment-card ${
                    paymentMethod === m.value
                      ? 'active'
                      : ''
                  }`}
                  onClick={() => {
                    setPaymentMethod(m.value)
                    setPaymentIssuer('')
                    setCouponApplied(false)
                    setCouponQuote(null)
                  }}
                >

                  <span className="payment-icon">
                    {m.icon}
                  </span>

                  <span>
                    <b>{m.label}</b>

                    <small>
                      {m.detail}
                    </small>
                  </span>

                  <i>
                    {paymentMethod === m.value
                      ? '✓'
                      : ''}
                  </i>

                </button>
              ))}

            </div>

            <div className="checkout-divider" />

            <div className="section-number">
              <span>3</span>

              <div>
                <h2>Payment details</h2>

                <small>
                  Add an optional UPI ID for faster processing
                </small>
              </div>
            </div>

            <label>
              UPI ID
              <input
                value={upiId}
                onChange={(e) => setUpiId(e.target.value)}
                placeholder="yourname@upi"
              />
            </label>

            {/* =========================
                RAZORPAY INFORMATION
            ========================== */}

            {isRazorpayFlow && paymentMethod !== 'QR' && (
              <div className="razorpay-info">

                <span>✓</span>

                <div>
                  <b>
                    One secure checkout
                  </b>

                  <small>
                    Razorpay test mode supports the selected {paymentMethod === 'RAZORPAY' ? 'payment method' : paymentMethod.toLowerCase()} flow for this bill amount. No real money is charged.
                  </small>
                </div>

              </div>
            )}

            {paymentMethod === 'QR' && qrCode && (
              <div className="qr-panel">
                <div className="qr-box">
                  <img src={qrCode} alt="UPI QR code for payment" />
                </div>
                <div className="qr-text">
                  <strong>Scan this QR to pay</strong>
                  <span>₹{(total / 100).toLocaleString('en-IN')}</span>
                  <small>Use any UPI app to complete this payment.</small>
                </div>
              </div>
            )}

            {/* =========================
                COD INFORMATION
            ========================== */}

            {paymentMethod === 'COD' && (
              <div className="cod-info">
                Cash on Delivery is simulated for
                this demo. The order will move through
                the existing async FlashCart pipeline.
              </div>
            )}

            {/* =========================
                ERROR MESSAGE
            ========================== */}

            {msg && (
              <div className="notice error">
                {msg}
              </div>
            )}

            {/* =========================
                PAY BUTTON
            ========================== */}

            <button
              className="primary-btn wide checkout-pay-btn"
              disabled={busy}
            >

              {busy
                ? paymentMethod === 'QR'
                  ? 'Generating QR…'
                  : 'Opening secure checkout…'
                : isRazorpayFlow
                  ? paymentMethod === 'QR'
                    ? `Generate QR · ₹${(
                        total / 100
                      ).toLocaleString('en-IN')}`
                    : `Pay ₹${(
                        total / 100
                      ).toLocaleString('en-IN')}`
                  : 'Place COD order'}

              <Icon
                name="arrow"
                size={17}
              />

            </button>

            <div className="checkout-trust">

              <Icon
                name="shield"
                size={15}
              />

              256-bit secure checkout · Your Razorpay
              secret never reaches the browser

            </div>

          </form>

        </div>

        {/* =========================
            ORDER SUMMARY
        ========================== */}

        <aside className="checkout-side">

          <div className="checkout-summary">

            <div className="summary-head">

              <h2>
                Order summary
              </h2>

              <span>
                {items.length} items
              </span>

            </div>

            {items.map((item) => (
              <div
                className="mini-order-item"
                key={item.product_id}
              >

                <div className="mini-order-art">
                  {item.name?.slice(0, 1) || 'P'}
                </div>

                <div>

                  <b>
                    {item.name || 'Product'}
                  </b>

                  <small>
                    Qty {item.quantity}
                  </small>

                </div>

                <strong>
                  ₹
                  {(
                    ((item.unit_price_paise ||
                      item.price_paise ||
                      0) *
                      item.quantity) /
                    100
                  ).toLocaleString('en-IN')}
                </strong>

              </div>
            ))}

            {/* =========================
                COUPON
            ========================== */}

            <div className="coupon-box">
              <div className="coupon-heading">
                <b>Available offers</b>
                <small>{subtotal >= 50000 ? 'Choose one coupon for this order.' : 'Coupons unlock on carts above ₹500.'}</small>
              </div>

              {['CARD', 'WALLET'].includes(paymentMethod) && (
                <label className="coupon-issuer-select">
                  {paymentMethod === 'CARD' ? 'Card issuer' : 'Wallet'}
                  <select value={paymentIssuer} onChange={(e) => { setPaymentIssuer(e.target.value); setCouponApplied(false); setCouponQuote(null) }}>
                    <option value="">Select {paymentMethod === 'CARD' ? 'your bank' : 'your wallet'}</option>
                    {(paymentMethod === 'CARD' ? [['SBI', 'SBI'], ['HDFC', 'HDFC Bank'], ['ICICI', 'ICICI Bank'], ['AXIS', 'Axis Bank']] : [['PHONEPE', 'PhonePe'], ['PAYTM', 'Paytm']]).map(([value, label]) => <option value={value} key={value}>{label}</option>)}
                  </select>
                </label>
              )}

              <div className="coupon-input">
                <input value={coupon} onChange={(e) => { setCoupon(e.target.value.toUpperCase()); setCouponApplied(false); setCouponQuote(null); setCouponError('') }} placeholder="Enter coupon code" />
                <button type="button" onClick={() => applyCoupon()} disabled={couponBusy || !coupon.trim()}>{couponBusy ? 'Checking…' : 'Apply'}</button>
              </div>
              {couponApplied && <span className="coupon-success">✓ {coupon} applied — save ₹{(discount / 100).toFixed(2)}</span>}
              {couponError && <span className="coupon-error">{couponError}</span>}

              {subtotal >= 50000 ? (
                <div className="coupon-offer-list">
                  {couponOffers.map((offer) => (
                    <div className={`coupon-offer ${offer.eligible ? 'available' : ''}`} key={offer.code}>
                      <div><b>{offer.code}</b><span>{offer.description}</span><small>{offer.discount_type === 'PERCENT' ? `${offer.discount_value}% off` : `₹${(offer.discount_value / 100).toFixed(0)} off`} · min ₹{(offer.min_order_paise / 100).toFixed(0)}{offer.eligible_category ? ` · ${offer.eligible_category}` : ''}</small>{!offer.eligible && offer.reason && <small className="coupon-reason">{offer.reason}</small>}</div>
                      <button type="button" disabled={!offer.eligible || couponBusy} onClick={() => applyCoupon(offer.code)}>{offer.eligible ? 'Apply' : 'Unavailable'}</button>
                    </div>
                  ))}
                  {!couponOffers.length && <small>No active coupons right now.</small>}
                </div>
              ) : null}
            </div>

            {/* =========================
                SUMMARY
            ========================== */}

            <div className="summary-line">

              <span>
                Subtotal
              </span>

              <b>
                ₹
                {(subtotal / 100).toLocaleString(
                  'en-IN'
                )}
              </b>

            </div>

            {discount > 0 && (
              <div className="summary-line">

                <span>
                  {coupon} discount
                </span>

                <b className="green">
                  −₹
                  {(discount / 100).toLocaleString(
                    'en-IN'
                  )}
                </b>

              </div>
            )}

            <div className="summary-line">

              <span>
                Delivery
              </span>

              <b className="green">
                FREE
              </b>

            </div>

            <div className="summary-total">

              <span>
                Total
              </span>

              <strong>
                ₹
                {(total / 100).toLocaleString(
                  'en-IN'
                )}
              </strong>

            </div>

          </div>

          {/* =========================
              FLASHCART PROMISE
          ========================== */}

          <div className="checkout-promise">

            <b>
              Why shop on FlashCart?
            </b>

            <span>
              ✓ Secure payments
            </span>

            <span>
              ✓ Easy order tracking
            </span>

            <span>
              ✓ Fast delivery promise
            </span>

          </div>

        </aside>

      </div>
    </section>
  )
}