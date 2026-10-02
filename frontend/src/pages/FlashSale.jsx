import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { flashSaleApi, productApi, subscriptionApi, wishlistApi } from '../services/api'
import { useAuth } from '../context/AuthContext'
import { useCart } from '../context/CartContext'
import Icon from '../components/Icon'
import CountdownTimer from '../components/CountdownTimer'
import SocialShare from '../components/SocialShare'

const PAYMENT_METHODS = [
  { value: 'COD', label: 'Cash on delivery' },
  { value: 'UPI', label: 'UPI' },
  { value: 'CARD', label: 'Card' },
  { value: 'RAZORPAY', label: 'Razorpay' },
]

function SaleCard({ sale, product }) {
  const [qty, setQty] = useState(1)
  const [paymentMethod, setPaymentMethod] = useState('UPI')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  const [subscribed, setSubscribed] = useState(false)
  const [wished, setWished] = useState(false)
  const { isAuthenticated } = useAuth()
  const { add } = useCart()
  const trackedView = useRef(false)

  useEffect(() => {
    if (!trackedView.current) {
      trackedView.current = true
      flashSaleApi.track(sale.sale_id, 'view')
    }
  }, [sale.sale_id])

  const addToCart = async () => {
    try {
      await add(product.id, qty)
      setMsg(`${qty} ${product.name} added to cart.`)
    } catch (e) {
      setMsg(e.response?.data?.error || 'Could not add this item to the cart.')
    }
  }

  const buy = async () => {
    if (!isAuthenticated) {
      setMsg('Please sign in to complete checkout.')
      return
    }
    setBusy(true)
    setMsg('')
    flashSaleApi.track(sale.sale_id, 'click')
    try {
      const key = crypto.randomUUID()
      const r = await flashSaleApi.buy(sale.sale_id, qty, paymentMethod, key)
      setMsg(`Order ${r.data.order_id || ''} accepted (${paymentMethod}). Payment is processed asynchronously.`)
    } catch (e) {
      setMsg(e.response?.data?.error || 'The reservation could not be completed.')
    } finally {
      setBusy(false)
    }
  }

  const subscribe = async () => {
    try {
      await subscriptionApi.subscribe({ product_id: product.id })
      setSubscribed(true)
    } catch {
      setSubscribed(true) // likely already subscribed — treat as success either way
    }
  }

  const toggleWish = async () => {
    if (!isAuthenticated) { window.location.href = '/login'; return }
    try {
      if (wished) { await wishlistApi.remove(product.id); setWished(false) }
      else { await wishlistApi.add(product.id); setWished(true) }
    } catch { /* non-critical */ }
  }

  const discountPercent = sale.discount_percent || 0
  const lowStock = sale.stock_remaining > 0 && sale.stock_remaining <= 10
  const shareUrl = `${window.location.origin}/flash-sale`

  return (
    <div className="sale-product">
      <div className="sale-art">
        {product.image_url
          ? <img src={product.image_url} alt={product.name} onError={e => { e.target.style.display = 'none' }} />
          : <><span>F</span><b>FLASH</b></>}
        <button className={`wish-btn floating ${wished ? 'active' : ''}`} onClick={toggleWish} aria-label="Save to wishlist">
          <Icon name="heart" size={16} />
        </button>
      </div>
      <div className="sale-copy">
        <div className="sale-meta-row">
          <span className="sale-live">● LIVE · {sale.state}</span>
          <CountdownTimer targetMs={sale.ends_at_ms} />
        </div>
        <h2>{product.name}</h2>
        <p>{product.brand} · {product.category}</p>
        <div className="sale-price">
          ₹{((sale.sale_price_paise || 0) / 100).toLocaleString('en-IN')}{' '}
          <del>₹{((product.price_paise || 0) / 100).toLocaleString('en-IN')}</del>
          {discountPercent > 0 && <span className="discount-inline">Save {discountPercent}%</span>}
        </div>
        <div className="stock-bar">
          <div style={{ width: `${Math.min(100, (sale.stock_remaining / Math.max(1, sale.stock_remaining + 10)) * 100)}%` }} />
        </div>
        <div className="sale-stock-indicator">
          {sale.stock_remaining > 0
            ? <span className={lowStock ? 'low-stock' : ''}>{lowStock ? `Only ${sale.stock_remaining} left!` : `${sale.stock_remaining} in stock`}</span>
            : <span className="low-stock">Sold out</span>}
          <span>Max 5 / user</span>
        </div>

        <div className="payment-method-row">
          <label>Pay with</label>
          <div className="payment-options">
            {PAYMENT_METHODS.map(m => (
              <button
                key={m.value}
                type="button"
                className={`payment-chip ${paymentMethod === m.value ? 'active' : ''}`}
                onClick={() => setPaymentMethod(m.value)}
              >
                {m.label}
              </button>
            ))}
          </div>
        </div>

        <div className="buy-row">
          <div className="qty">
            <button onClick={() => setQty(Math.max(1, qty - 1))}>−</button>
            <b>{qty}</b>
            <button onClick={() => setQty(Math.min(5, qty + 1))}>+</button>
          </div>
          <button type="button" onClick={addToCart} className="secondary-btn sale-add">
            Add to cart
          </button>
          <button disabled={busy || sale.state !== 'ACTIVE' || sale.stock_remaining <= 0} onClick={buy} className="primary-btn sale-buy">
            {busy ? 'Reserving…' : 'Buy now'} <Icon name="bolt" size={18} />
          </button>
        </div>
        {msg && <div className="notice">{msg}</div>}

        <div className="sale-footer-row">
          <button className={`ghost-btn small ${subscribed ? 'disabled' : ''}`} onClick={subscribe} disabled={subscribed}>
            <Icon name="bell" size={14} /> {subscribed ? 'Subscribed' : 'Notify me next time'}
          </button>
        </div>
        <SocialShare url={shareUrl} text={`${product.name} is on flash sale for ₹${((sale.sale_price_paise || 0) / 100).toLocaleString('en-IN')} on FlashCart!`} />
      </div>
    </div>
  )
}

export default function FlashSale() {
  const [sales, setSales] = useState([])
  const [products, setProducts] = useState({})
  const [loaded, setLoaded] = useState(false)
  const [subscribeEmail, setSubscribeEmail] = useState('')
  const [subscribeMsg, setSubscribeMsg] = useState('')
  const { isAuthenticated } = useAuth()

  // Discover every currently-live sale instead of a single hardcoded slot —
  // this is what actually fixes "only one product shows in flash sales":
  // admin activation of any number of products all surface here at once.
  useEffect(() => {
    let cancelled = false

    const refresh = async () => {
      try {
        const { data } = await flashSaleApi.active()
        if (cancelled) return
        const list = data || []
        setSales(list)

        const missingIds = list.map(s => s.product_id).filter(id => !products[id])
        if (missingIds.length) {
          const fetched = await Promise.all(missingIds.map(id => productApi.get(id).then(r => [id, r.data]).catch(() => [id, null])))
          if (!cancelled) {
            setProducts(prev => {
              const next = { ...prev }
              fetched.forEach(([id, p]) => { if (p) next[id] = p })
              return next
            })
          }
        }
      } catch {
        if (!cancelled) setSales([])
      } finally {
        if (!cancelled) setLoaded(true)
      }
    }

    refresh()
    const interval = setInterval(refresh, 5000)
    return () => { cancelled = true; clearInterval(interval) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const subscribeAnyEmail = async (e) => {
    e.preventDefault()
    setSubscribeMsg('')
    try {
      await subscriptionApi.subscribe({ email: isAuthenticated ? undefined : subscribeEmail })
      setSubscribeMsg('You\'re subscribed! We\'ll alert you the moment a new flash sale goes live.')
      setSubscribeEmail('')
    } catch (e2) {
      setSubscribeMsg(e2.response?.data?.error || 'Could not subscribe right now.')
    }
  }

  return (
    <section className="sale-page">
      <div className="container">
        <div className="sale-header">
          <span className="pill dark"><Icon name="bolt" size={15} /> Limited-time drop</span>
          <h1>Flash Sale</h1>
          <p>Atomic Redis reservations keep limited inventory fair when demand explodes.</p>
        </div>

        {sales.length > 0 ? (
          <div className="sale-grid">
            {sales.map(sale => products[sale.product_id] && (
              <SaleCard key={sale.sale_id} sale={sale} product={products[sale.product_id]} />
            ))}
          </div>
        ) : (
          loaded && (
            <div className="sale-placeholder">
              <div className="sale-orb">⚡</div>
              <h2>No flash sale is live right now.</h2>
              <p>An admin can launch one from the <Link to="/admin">admin control center</Link> — it shows up here automatically, no configuration needed.</p>
              <Link to="/products" className="secondary-btn">Browse products</Link>

              <form className="notify-form" onSubmit={subscribeAnyEmail}>
                <Icon name="bell" size={16} />
                {!isAuthenticated && (
                  <input
                    type="email"
                    required
                    placeholder="you@example.com"
                    value={subscribeEmail}
                    onChange={e => setSubscribeEmail(e.target.value)}
                  />
                )}
                <button className="primary-btn" type="submit">Notify me about the next sale</button>
              </form>
              {subscribeMsg && <div className="notice">{subscribeMsg}</div>}
            </div>
          )
        )}

        <div className="sale-principles">
          <div><Icon name="bolt" /><b>Atomic reservation</b><span>Redis Lua prevents overselling.</span></div>
          <div><Icon name="shield" /><b>Idempotent buying</b><span>Retries won't create duplicates.</span></div>
          <div><Icon name="package" /><b>Async fulfillment</b><span>Kafka workers process orders.</span></div>
        </div>
      </div>
    </section>
  )
}
