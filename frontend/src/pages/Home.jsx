import { Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { productApi, announcementApi, flashSaleApi } from '../services/api'
import ProductCard from '../components/ProductCard'
import Icon from '../components/Icon'
import SectionTitle from '../components/SectionTitle'
import { useAuth } from '../context/AuthContext'

const cats = [
  ['Mobiles', '📱', 'Smartphones & accessories'],
  ['Electronics', '💻', 'Laptops, audio & gadgets'],
  ['Fashion', '👟', 'Trending styles'],
  ['Home', '🛋️', 'Make your space better'],
  ['Appliances', '⚡', 'Upgrade everyday life'],
  ['Beauty', '✨', 'Care & wellness'],
  ['Grocery', '🛒', 'Daily essentials'],
]

export default function Home() {
  const [products, setProducts] = useState([])
  const [announcement, setAnnouncement] = useState(null)
  const [sales, setSales] = useState([])
  const { welcomeMessage, clearWelcomeMessage } = useAuth()

  useEffect(() => {
    productApi.list().then(r => setProducts(r.data || [])).catch(() => {})
    announcementApi.active().then(r => setAnnouncement(r.data)).catch(() => {})
    flashSaleApi.active().then(r => setSales(r.data || [])).catch(() => {})
  }, [])

  return <>
    {welcomeMessage && <div className="welcome-toast"><Icon name="check" size={18}/><span>{welcomeMessage}</span><button onClick={clearWelcomeMessage}><Icon name="close" size={14}/></button></div>}
    {announcement?.message && <div className="announcement-strip"><span>📣</span><b>{announcement.message}</b>{announcement.link && <a href={announcement.link}>View deal →</a>}</div>}

    <section className="home-hero">
      <div className="container hero-shell">
        <div className="hero-copy">
          <span className="pill"><Icon name="bolt" size={15}/> BIG SAVINGS · FAST DELIVERY</span>
          <h1>Everything you love.<br/><em>Better prices.</em></h1>
          <p>Discover top products, limited-time deals and a checkout experience built for India's busiest shopping moments.</p>
          <div className="hero-cta"><Link to="/products" className="primary-btn">Shop now <Icon name="arrow"/></Link><Link to="/flash-sale" className="secondary-btn">⚡ Today's deals</Link></div>
          <div className="hero-promises"><span>✓ Genuine products</span><span>✓ Secure payments</span><span>✓ Easy returns</span></div>
        </div>
        <div className="hero-banner">
          <div className="hero-banner-top"><span>FLASHCART FESTIVAL</span><b>LIMITED TIME</b></div>
          <h2>Up to <strong>70% OFF</strong></h2>
          <p>On electronics, fashion & more</p>
          <Link to="/flash-sale">Shop flash deals →</Link>
          <div className="hero-product-orb">⚡</div>
          <div className="hero-mini-card"><b>{sales.length || '100'}+</b><span>deals live</span></div>
        </div>
      </div>
    </section>

    <section className="category-section">
      <div className="container"><SectionTitle eyebrow="Explore" title="Shop by category" description="Find what you need in one tap."/>
        <div className="category-grid marketplace-categories">{cats.map(([name,icon,desc]) => <Link className="category-card" to={`/products?category=${encodeURIComponent(name)}`} key={name}><div className="market-cat-icon">{icon}</div><b>{name}</b><span>{desc}</span></Link>)}</div>
      </div>
    </section>

    <section className="deal-strip"><div className="container deal-strip-inner"><div><span className="eyebrow light">LIMITED-TIME OFFERS</span><h2>⚡ Flash deals are live</h2><p>Prices drop. Stock moves fast.</p></div><div className="deal-clock"><b>00</b><i>:</i><b>18</b><i>:</i><b>42</b><small>HRS · MIN · SEC</small></div><Link to="/flash-sale" className="white-btn">View all deals <Icon name="arrow"/></Link></div></section>

    <section className="products-section">
      <div className="container">
        <SectionTitle eyebrow="Top picks" title="Trending right now" description="Popular picks from the FlashCart marketplace." to="/products"/>
        <div className="product-grid">{products.length ? products.slice(0, 8).map((p,i) => <ProductCard key={p.id} product={p} index={i}/>) : [1,2,3,4].map(i => <div className="skeleton-card" key={i}><div className="skeleton-art"/><div className="skeleton-line"/><div className="skeleton-line short"/></div>)}</div>
      </div>
    </section>

    <section className="recommend-banner"><div className="container"><div><span className="eyebrow">SMART SHOPPING</span><h2>Deals picked for every kind of shopper.</h2><p>Search by brand, filter by price and save favourites to your wishlist.</p></div><Link to="/products" className="primary-btn">Explore marketplace <Icon name="arrow"/></Link></div></section>

    <section className="stats marketplace-stats"><div className="container stats-grid"><div><b>⚡ High-traffic ready</b><span>Redis-backed flash-sale reservations</span></div><div><b>🔐 Secure checkout</b><span>Razorpay test-mode verification</span></div><div><b>📦 Easy tracking</b><span>Async order state transitions</span></div><div><b>📊 Admin control</b><span>Catalog, sales & analytics</span></div></div></section>
  </>
}
