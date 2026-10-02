import { useEffect, useMemo, useState } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { productApi } from '../services/api'
import ProductCard from '../components/ProductCard'
import Icon from '../components/Icon'

const CATEGORIES = ['All', 'Mobiles', 'Electronics', 'Fashion', 'Home', 'Appliances', 'Beauty', 'Grocery']

export default function Products() {
  const [params, setParams] = useSearchParams()
  const [products, setProducts] = useState([])
  const [loading, setLoading] = useState(true)
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [viewMode, setViewMode] = useState('grid')
  const q = params.get('q') || ''
  const category = params.get('category') || ''
  const brand = params.get('brand') || ''
  const sort = params.get('sort') || 'featured'
  const min = Number(params.get('min') || 0)
  const max = Number(params.get('max') || 0)
  const [searchInput, setSearchInput] = useState(q)

  useEffect(() => {
    setLoading(true)
    const query = {}
    if (category) query.category = category
    if (brand) query.brand = brand
    if (q) query.q = q
    productApi.list(query).then(r => setProducts(r.data || [])).catch(() => setProducts([])).finally(() => setLoading(false))
  }, [category, brand, q])

  useEffect(() => setSearchInput(q), [q])

  const brands = useMemo(() => [...new Set(products.map(p => p.brand).filter(Boolean))].sort(), [products])

  const visible = useMemo(() => {
    let list = [...products]
    if (min) list = list.filter(p => p.price_paise / 100 >= min)
    if (max) list = list.filter(p => p.price_paise / 100 <= max)
    if (sort === 'price-low') list.sort((a,b) => a.price_paise - b.price_paise)
    if (sort === 'price-high') list.sort((a,b) => b.price_paise - a.price_paise)
    if (sort === 'newest') list.reverse()
    return list
  }, [products, min, max, sort])

  const set = (next) => {
    const base = {
      ...(q ? { q } : {}), ...(category ? { category } : {}), ...(brand ? { brand } : {}),
      ...(sort !== 'featured' ? { sort } : {}), ...(min ? { min } : {}), ...(max ? { max } : {}), ...next,
    }
    Object.keys(base).forEach(k => { if (base[k] === undefined || base[k] === '') delete base[k] })
    setParams(base)
  }

  const clear = () => setParams(q ? { q } : {})
  const activeFilters = [
    ...(category ? [{ key: 'category', label: category }] : []),
    ...(brand ? [{ key: 'brand', label: brand }] : []),
    ...(min ? [{ key: 'min', label: `Above ₹${min}` }] : []),
    ...(max ? [{ key: 'max', label: `Below ₹${max}` }] : []),
  ]

  return (
    <section className="page products-page">
      <div className="container">
        <div className="catalog-hero">
          <div>
            <div className="eyebrow">FlashCart marketplace</div>
            <h1>{q ? <>Results for <em>“{q}”</em></> : category ? `${category} collection` : 'Shop smarter. Shop faster.'}</h1>
            <p>Great prices, trusted brands and a checkout built for high-traffic shopping.</p>
          </div>
          <div className="catalog-badges"><span><Icon name="bolt" size={14}/> Flash deals</span><span><Icon name="package" size={14}/> Free delivery</span><span><Icon name="shield" size={14}/> Secure payments</span></div>
        </div>

        <div className="market-search">
          <Icon name="search" size={18}/>
          <form onSubmit={e => { e.preventDefault(); set({ q: searchInput.trim() || undefined }) }}>
            <input value={searchInput} onChange={e => setSearchInput(e.target.value)} placeholder="Search for products, brands and more"/>
          </form>
          <button onClick={() => set({ q: searchInput.trim() || undefined })}>Search</button>
        </div>

        <div className="category-pills">
          {CATEGORIES.map(c => <button key={c} className={(c === 'All' ? !category : category === c) ? 'active' : ''} onClick={() => set({ category: c === 'All' ? undefined : c })}>{c}</button>)}
        </div>

        <div className="catalog-toolbar">
          <button className="filter-toggle" onClick={() => setFiltersOpen(v => !v)}><Icon name="menu" size={16}/> Filters</button>
          <div className="catalog-count"><strong>{visible.length}</strong><span>products</span><small>{category || 'All departments'}{q ? ` · matching “${q}”` : ''}</small></div>
          <div className="sort-box"><span>Sort by</span><select value={sort} onChange={e => set({ sort: e.target.value })}><option value="featured">Featured</option><option value="newest">Newest</option><option value="price-low">Price: Low to High</option><option value="price-high">Price: High to Low</option></select></div>
          <div className="view-switch" role="group" aria-label="Product layout">
            <button className={viewMode === 'grid' ? 'active' : ''} onClick={() => setViewMode('grid')} aria-label="Grid view" title="Grid view"><Icon name="grid" size={17}/></button>
            <button className={viewMode === 'list' ? 'active' : ''} onClick={() => setViewMode('list')} aria-label="List view" title="List view"><Icon name="list" size={17}/></button>
          </div>
        </div>

        {activeFilters.length > 0 && <div className="active-filters"><span>Active filters</span>{activeFilters.map(filter => <button key={filter.key} onClick={() => set({ [filter.key]: undefined })}>{filter.label}<Icon name="close" size={13}/></button>)}<button className="clear-filters" onClick={clear}>Clear all</button></div>}

        <div className={`catalog-layout ${filtersOpen ? 'filters-open' : ''}`}>
          <aside className="filters-card">
            <div className="filter-head"><div><span className="eyebrow">Refine results</span><h3>Filters</h3></div><button onClick={clear}>Clear all</button></div>
            <div className="filter-group"><b>Brand</b>{brands.length ? brands.map(b => <label key={b}><input type="radio" name="brand" checked={brand === b} onChange={() => set({ brand: b })}/><span>{b}</span></label>) : <small className="muted">Brand filters appear when products have brands.</small>}</div>
            <div className="filter-group"><b>Price</b><div className="price-inputs"><input type="number" min="0" placeholder="Min" value={min || ''} onChange={e => set({ min: e.target.value || undefined })}/><span>—</span><input type="number" min="0" placeholder="Max" value={max || ''} onChange={e => set({ max: e.target.value || undefined })}/></div></div>
            <div className="filter-group"><b>Customer rating</b><label><input type="checkbox" checked readOnly/><span>★★★★☆ & up</span></label><label><input type="checkbox" checked readOnly/><span>Top rated</span></label></div>
            <div className="filter-benefit"><span>✓</span><div><b>Easy returns</b><small>Demo marketplace policy</small></div></div>
          </aside>

          <main>
            {loading ? <div className={`product-grid ${viewMode === 'list' ? 'list-mode' : ''}`}>{[1,2,3,4,5,6].map(i => <div className="skeleton-card" key={i}><div className="skeleton-art"/><div className="skeleton-line"/><div className="skeleton-line short"/></div>)}</div>
            : visible.length ? <div className={`product-grid ${viewMode === 'list' ? 'list-mode' : ''}`}>{visible.map((p,i) => <ProductCard key={p.id} product={p} index={i}/>)}</div>
            : <div className="empty-state"><div className="empty-icon">⌁</div><h2>No products found</h2><p>Try another search, category or price range.</p><button className="secondary-btn" onClick={clear}>Clear filters</button></div>}
          </main>
        </div>

        <div className="catalog-confidence">
          <div><span>🛡</span><b>Secure payments</b><small>Razorpay test checkout</small></div>
          <div><span>↺</span><b>Easy returns</b><small>Simple demo workflow</small></div>
          <div><span>⚡</span><b>Fast checkout</b><small>Built for traffic spikes</small></div>
          <div><span>✓</span><b>Verified inventory</b><small>Stock-aware ordering</small></div>
        </div>
      </div>
    </section>
  )
}
