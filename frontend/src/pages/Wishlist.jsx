import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { wishlistApi } from '../services/api'
import Icon from '../components/Icon'

function money(paise = 0) { return `₹${(Number(paise || 0) / 100).toLocaleString('en-IN')}` }

export default function Wishlist() {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(true)

  const load = () => {
    setLoading(true)
    wishlistApi.list().then(r => setItems(r.data || [])).catch(() => setItems([])).finally(() => setLoading(false))
  }
  useEffect(() => { load() }, [])

  const remove = async (productId) => {
    try { await wishlistApi.remove(productId); load() } catch { /* non-critical */ }
  }

  return (
    <section className="page">
      <div className="container">
        <div className="page-hero">
          <div>
            <div className="eyebrow">Saved for later</div>
            <h1>Your wishlist</h1>
            <p>Keep an eye on price and stock for products you're waiting to buy.</p>
          </div>
        </div>

        {loading ? (
          <div className="loading-state">Loading your wishlist…</div>
        ) : items.length ? (
          <div className="wishlist-grid">
            {items.map(it => (
              <div className="wishlist-card" key={it.product_id}>
                <Link to={`/products/${it.product_id}`} className="wishlist-art">
                  {it.image_url
                    ? <img src={it.image_url} alt={it.name} onError={e => { e.target.style.display = 'none' }} />
                    : <span>{it.name?.slice(0, 1) || 'P'}</span>}
                </Link>
                <div className="wishlist-info">
                  <Link to={`/products/${it.product_id}`}><h3>{it.name}</h3></Link>
                  <div className="price-row"><strong>{money(it.price_paise)}</strong></div>
                  <span className={it.stock > 0 ? '' : 'low-stock'}>{it.stock > 0 ? `${it.stock} in stock` : 'Out of stock'}</span>
                  <div className="wishlist-actions">
                    <Link to={`/products/${it.product_id}`} className="secondary-btn small">View product</Link>
                    <button className="ghost-btn small danger-text" onClick={() => remove(it.product_id)}>
                      <Icon name="trash" size={14} /> Remove
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="empty-state">
            <div><Icon name="heart" size={32} /></div>
            <h2>Your wishlist is empty</h2>
            <p>Tap the heart on any product to save it here.</p>
            <Link to="/products" className="secondary-btn">Browse products</Link>
          </div>
        )}
      </div>
    </section>
  )
}
