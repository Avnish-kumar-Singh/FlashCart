import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
import { useCart } from '../context/CartContext'
import Icon from './Icon'
import ThemePicker from './ThemePicker'

export default function Header() {
  const { user, isAdmin, logout } = useAuth()
  const { count } = useCart()
  const navigate = useNavigate()
  const [q, setQ] = useState('')
  const [open, setOpen] = useState(false)
  const [themeOpen, setThemeOpen] = useState(false)
  const submit = e => { e.preventDefault(); navigate(`/products${q ? `?q=${encodeURIComponent(q)}` : ''}`); setOpen(false) }

  return <>
    <div className="top-strip"><div className="container top-inner"><span>⚡ Flash deals · Fast delivery · Secure checkout</span><span>India's demo marketplace · Razorpay test mode</span></div></div>
    <header className="header">
      <div className="container header-main">
        <button className="mobile-menu" onClick={() => setOpen(v => !v)} aria-label="Menu"><Icon name={open ? 'close' : 'menu'}/></button>
        <Link to="/" className="brand"><span className="brand-mark">F</span><span>flash<span>cart</span></span><small>Shop smart. Shop fast.</small></Link>
        <form className="search" onSubmit={submit}><span className="search-all">All ▾</span><Icon name="search" size={18}/><input value={q} onChange={e => setQ(e.target.value)} placeholder="Search products, brands and more"/><button>Search</button></form>
        <nav className={`nav-actions ${open ? 'show' : ''}`}>
          <button className="theme-trigger" onClick={() => { setThemeOpen(true); setOpen(false) }} aria-label="Customize theme" title="Customize theme"><Icon name="palette" size={17}/><span>Theme</span></button>
          <Link to="/products" onClick={() => setOpen(false)}>Products</Link>
          <Link to="/flash-sale" className="sale-link" onClick={() => setOpen(false)}><Icon name="bolt" size={16}/> Deals</Link>
          {user && <Link to="/wishlist" onClick={() => setOpen(false)}><Icon name="heart" size={16}/> Wishlist</Link>}
          {isAdmin && <Link to="/admin" className="admin-link" onClick={() => setOpen(false)}><Icon name="shield" size={16}/> Admin</Link>}
          {user ? <div className="account-wrap"><button className="account-btn" onClick={() => setOpen(v => !v)}><Icon name="user" size={18}/><span>Hello, {isAdmin ? 'Admin' : 'Account'}</span><small>Account & Lists</small></button>{open && <div className="account-menu"><b>{isAdmin ? 'Administrator' : 'Your Account'}</b><small>{user.id}</small>{isAdmin && <Link to="/admin">Admin console</Link>}<Link to="/account">My Profile</Link><Link to="/orders">My Orders</Link><Link to="/wishlist">Wishlist</Link><button onClick={() => { logout(); navigate('/') }}>Logout</button></div>}</div> : <Link to="/login" className="login-btn">Sign in</Link>}
          <Link to="/cart" className="cart-btn" onClick={() => setOpen(false)}><Icon name="cart" size={21}/><span>Cart</span>{count > 0 && <b>{count}</b>}</Link>
        </nav>
      </div>
      <div className="category-nav"><div className="container"><Link to="/products">☰ All</Link><Link to="/products?category=Mobiles">Mobiles</Link><Link to="/products?category=Electronics">Electronics</Link><Link to="/products?category=Fashion">Fashion</Link><Link to="/products?category=Home">Home & Kitchen</Link><Link to="/products?category=Appliances">Appliances</Link><Link to="/products?category=Beauty">Beauty</Link><Link to="/flash-sale">Today's Deals</Link></div></div>
    </header>
    {themeOpen && <ThemePicker onClose={() => setThemeOpen(false)}/>}
  </>
}
