import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { productApi, wishlistApi } from '../services/api'
import { useCart } from '../context/CartContext'
import { useAuth } from '../context/AuthContext'
import Icon from '../components/Icon'
import SocialShare from '../components/SocialShare'

function parseDescription(description) {
  const sections = String(description || '').split(/\n\n+/).map((section) => {
    const [title, ...lines] = section.split('\n')
    return { title, content: lines.join('\n') }
  }).filter((section) => section.title && section.content)

  return sections.length ? sections : [{ title: 'Overview', content: description || 'Product details have not been provided.' }]
}

export default function ProductDetails() {
  const { id } = useParams()
  const [p, setP] = useState(null)
  const [error, setError] = useState('')
  const [added, setAdded] = useState(false)
  const [wished, setWished] = useState(false)
  const { add } = useCart()
  const { isAuthenticated } = useAuth()

  useEffect(() => { productApi.get(id).then(r => setP(r.data)).catch(() => setError('Product not found')) }, [id])

  if (error) return <section className="page"><div className="container empty-state"><h2>{error}</h2><Link className="primary-btn" to="/products">Back to products</Link></div></section>
  if (!p) return <section className="page"><div className="container loading-state">Loading product…</div></section>

  const price = (p.price_paise || 0) / 100
  const discount = p.price_paise > 50000 ? 12 : 8
  const mrp = Math.round(price / (1 - discount / 100))
  const descriptionSections = parseDescription(p.description)
  const addItem = async () => {
    try { await add(p.id); setAdded(true); setTimeout(() => setAdded(false), 1800) } catch {}
  }
  const toggleWish = async () => {
    if (!isAuthenticated) { window.location.href = '/login'; return }
    try {
      if (wished) { await wishlistApi.remove(p.id); setWished(false) }
      else { await wishlistApi.add(p.id); setWished(true) }
    } catch {}
  }

  return <section className="page product-detail-page">
    <div className="container">
      <div className="breadcrumbs"><Link to="/">Home</Link><span>›</span><Link to="/products">{p.category || 'Products'}</Link><span>›</span><b>{p.name}</b></div>

      <div className="detail-grid">
        <div className="detail-gallery">
          <div className="detail-art">
            {p.image_url ? <img src={p.image_url} alt={p.name} onError={e => { e.target.style.display = 'none' }}/> : <div className="detail-placeholder">{p.category?.slice(0,1) || 'P'}</div>}
            <button className={`detail-wish ${wished ? 'active' : ''}`} onClick={toggleWish}><Icon name="heart" size={20}/></button>
          </div>
          <div className="gallery-trust"><span>🛡 Secure payments</span><span>↺ 7-day demo returns</span><span>🚚 Free delivery</span></div>
        </div>

        <div className="detail-copy">
          <div className="detail-brand-row"><span className="rating">★ 4.6</span><span>1,284 ratings</span><span>•</span><b>{p.brand || 'FlashCart Select'}</b></div>
          <h1>{p.name}</h1>
          <p className="detail-brand">{p.brand || 'FlashCart Select'} · {p.category || 'Featured'}</p>
          <div className="detail-price-row"><strong>₹{price.toLocaleString('en-IN')}</strong><del>₹{mrp.toLocaleString('en-IN')}</del><span>{discount}% off</span></div>
          <p className="tax-note">Inclusive of all taxes · Free delivery</p>

          <div className="offer-box"><div className="offer-title">🏷 Available offers</div><div><b>10% Instant discount</b><span>Demo bank offer on eligible payments</span></div><div><b>5% cashback</b><span>Selected wallet / UPI offers in test checkout</span></div><div><b>No-cost EMI</b><span>Available on eligible cards in supported Razorpay checkout</span></div></div>

          <div className="delivery-box"><Icon name="package" size={20}/><div><b>Delivery available</b><span>Enter your PIN at checkout for delivery details</span></div></div>

          <div className="feature-list"><span><Icon name="check"/> {p.stock > 0 ? `In stock · ${p.stock} available` : 'Currently out of stock'}</span><span><Icon name="shield"/> Secure Razorpay checkout</span><span><Icon name="bolt"/> Fast order processing</span></div>

          <div className="detail-actions"><button className="primary-btn" onClick={addItem} disabled={!p.stock}>{added ? <><Icon name="check"/> Added</> : <>Add to cart <Icon name="cart"/></>}</button><Link className="secondary-btn" to="/flash-sale"><Icon name="bolt"/> View flash deals</Link></div>
          <SocialShare url={typeof window !== 'undefined' ? window.location.href : ''} text={`Check out ${p.name} on FlashCart!`}/>
        </div>
      </div>

      <div className="detail-tabs">
        <div className="detail-description">
          <div className="detail-description-heading"><span className="eyebrow">Product information</span><h2>About this item</h2></div>
          <div className="detail-description-sections">
            {descriptionSections.map((section) => {
              const lines = section.content.split('\n').filter(Boolean)
              const bullets = lines.length > 0 && lines.every((line) => line.startsWith('- '))
              return <section className="detail-description-section" key={section.title}>
                <h3>{section.title}</h3>
                {bullets
                  ? <ul>{lines.map((line) => <li key={line}>{line.slice(2)}</li>)}</ul>
                  : <p>{section.content}</p>}
              </section>
            })}
          </div>
        </div>
        <div><b>Ratings & reviews</b><p>★ 4.6/5 · 1,284 ratings · Customers love the value and delivery experience.</p><div className="review-bars"><span><i style={{width:'82%'}}/>5 ★</span><span><i style={{width:'68%'}}/>4 ★</span><span><i style={{width:'22%'}}/>3 ★</span></div></div>
        <div><b>Seller information</b><p>{p.brand || 'FlashCart Select'} · Verified marketplace seller · Fast dispatch</p></div>
      </div>
    </div>
  </section>
}
