import { useEffect, useMemo, useState } from 'react'
import { adminApi, analyticsApi, announcementApi, broadcastApi, categoryApi, flashSaleApi, productApi } from '../services/api'
import Icon from '../components/Icon'

const emptyForm = { id: '', name: '', description: '', brand: '', category: 'Electronics', price_paise: '', tax_rate_bps: 0, stock: '', seller_id: '', image_url: '' }
const emptyCategory = { id: '', name: '', slug: '', parent_id: '', description: '', banner_image_url: '', seo_title: '', seo_description: '', sort_order: 0, active: true }
const emptySale = { product_id: '', discount_percent: 20, stock: 10, starts_at: '', ends_at: '' }
const emptyAnnouncement = { message: '', link: '' }
const emptyBroadcast = { channel: 'EMAIL', message: '' }

function money(paise = 0) { return `₹${(Number(paise || 0) / 100).toLocaleString('en-IN')}` }

const TABS = [
  ['catalog', 'Catalog'],
  ['categories', 'Categories'],
  ['sales', 'Flash sales'],
  ['announcements', 'Announcements'],
  ['broadcasts', 'Message queue'],
  ['analytics', 'Analytics'],
  ['orders', 'Orders'],
  ['payments', 'Payments'],
]

export default function Admin() {
  const [tab, setTab] = useState('catalog')
  const [stats, setStats] = useState(null)
  const [products, setProducts] = useState([])
  const [categories, setCategories] = useState([])
  const [orders, setOrders] = useState([])
  const [payments, setPayments] = useState([])
  const [activeSales, setActiveSales] = useState([])
  const [announcements, setAnnouncements] = useState([])
  const [broadcasts, setBroadcasts] = useState([])
  const [analyticsOverview, setAnalyticsOverview] = useState([])
  const [couponAnalytics, setCouponAnalytics] = useState([])

  const [form, setForm] = useState(emptyForm)
  const [categoryForm, setCategoryForm] = useState(emptyCategory)
  const [categoryEditing, setCategoryEditing] = useState(false)
  const [editing, setEditing] = useState(false)
  const [sale, setSale] = useState(emptySale)
  const [saleResult, setSaleResult] = useState(null)
  const [announcementForm, setAnnouncementForm] = useState(emptyAnnouncement)
  const [broadcastForm, setBroadcastForm] = useState(emptyBroadcast)
  const [broadcastResult, setBroadcastResult] = useState(null)

  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const load = async () => {
    setError('')
    try {
      const [s, p, c, o, pay, a, an, b] = await Promise.all([
        adminApi.stats(), productApi.list(), categoryApi.list(), adminApi.orders(), adminApi.payments(),
        flashSaleApi.active(), announcementApi.list(), broadcastApi.list(),
      ])
      setStats(s.data); setProducts(p.data || []); setCategories(c.data || []); setOrders(o.data || []); setPayments(pay.data || [])
      setActiveSales(a.data || []); setAnnouncements(an.data || []); setBroadcasts(b.data || [])
      if (!sale.product_id && p.data?.[0]) setSale((x) => ({ ...x, product_id: p.data[0].id }))
    } catch (e) {
      setError(e.response?.data?.error || 'Unable to load admin data. Make sure you are signed in as an admin.')
    }
  }
  useEffect(() => { load() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (tab === 'analytics') {
      analyticsApi.overview().then((r) => setAnalyticsOverview(r.data || [])).catch(() => setAnalyticsOverview([]))
      analyticsApi.coupons().then((r) => setCouponAnalytics(r.data || [])).catch(() => setCouponAnalytics([]))
    }
  }, [tab])

  // ---- Catalog ----
  const submitProduct = async (e) => {
    e.preventDefault(); setBusy(true); setMessage(''); setError('')
    const body = {
      name: form.name, description: form.description, brand: form.brand, category: form.category,
      price_paise: Number(form.price_paise), tax_rate_bps: Number(form.tax_rate_bps), stock: Number(form.stock), seller_id: form.seller_id || '',
      image_url: form.image_url || '',
    }
    try {
      if (editing) await productApi.update(form.id, body); else await productApi.create(body)
      setMessage(editing ? 'Product updated successfully.' : 'Product created successfully.')
      setForm(emptyForm); setEditing(false); await load()
    } catch (e) { setError(e.response?.data?.error || 'Product operation failed.') } finally { setBusy(false) }
  }
  const editProduct = (p) => setForm({
    id: p.id, name: p.name, description: p.description, brand: p.brand, category: p.category,
    price_paise: p.price_paise, tax_rate_bps: p.tax_rate_bps || 0, stock: p.stock, seller_id: p.seller_id || '', image_url: p.image_url || '',
  })
  const deleteProduct = async (p) => {
    if (!confirm(`Delete "${p.name}"? This cannot be undone.`)) return
    try {
      await productApi.remove(p.id); setMessage('Product deleted.')
      if (form.id === p.id) { setForm(emptyForm); setEditing(false) }
      await load()
    } catch (e) { setError(e.response?.data?.error || 'Delete failed.') }
  }

  const submitCategory = async (e) => {
    e.preventDefault(); setBusy(true); setMessage(''); setError('')
    const body = { ...categoryForm, parent_id: categoryForm.parent_id || null, sort_order: Number(categoryForm.sort_order) }
    try {
      if (categoryEditing) await categoryApi.update(categoryForm.id, body); else await categoryApi.create(body)
      setMessage(categoryEditing ? 'Category updated.' : 'Category created.')
      setCategoryForm(emptyCategory); setCategoryEditing(false); await load()
    } catch (e) { setError(e.response?.data?.error || 'Category operation failed.') } finally { setBusy(false) }
  }
  const editCategory = (category) => {
    setCategoryForm({ ...category, parent_id: category.parent_id || '' })
    setCategoryEditing(true)
  }
  const deleteCategory = async (category) => {
    if (!confirm(`Delete category "${category.name}"? Categories with children cannot be deleted.`)) return
    try {
      await categoryApi.remove(category.id); setMessage('Category deleted.')
      if (categoryForm.id === category.id) { setCategoryForm(emptyCategory); setCategoryEditing(false) }
      await load()
    } catch (e) { setError(e.response?.data?.error || 'Could not delete category.') }
  }

  // ---- Flash sales ----
  const activateSale = async (e) => {
    e.preventDefault(); setBusy(true); setError(''); setMessage(''); setSaleResult(null)
    try {
      const body = {
        ...sale, discount_percent: Number(sale.discount_percent), stock: Number(sale.stock),
        starts_at: new Date(sale.starts_at).toISOString(), ends_at: new Date(sale.ends_at).toISOString(),
      }
      const r = await flashSaleApi.activate(body)
      setSaleResult(r.data)
      setMessage(`Flash sale activated for ${r.data.product_name || 'the product'}.`)
      await load()
    } catch (e) { setError(e.response?.data?.error || 'Could not activate flash sale.') } finally { setBusy(false) }
  }
  const deactivateSale = async (saleId) => {
    if (!confirm('End this flash sale now? Unsold stock returns to the catalog.')) return
    try { await flashSaleApi.deactivate(saleId); setMessage('Flash sale ended.'); await load() }
    catch (e) { setError(e.response?.data?.error || 'Could not end flash sale.') }
  }

  // ---- Announcements ----
  const submitAnnouncement = async (e) => {
    e.preventDefault(); setBusy(true); setError(''); setMessage('')
    try {
      await announcementApi.create(announcementForm)
      setMessage('Announcement is live on the storefront banner.')
      setAnnouncementForm(emptyAnnouncement); await load()
    } catch (e) { setError(e.response?.data?.error || 'Could not publish announcement.') } finally { setBusy(false) }
  }
  const refundPayment = async (payment) => {
    if (!confirm(`Refund ${money(payment.amount_paise)} for payment ${payment.razorpay_payment_id}?`)) return
    try { await adminApi.refundPayment(payment.id, payment.amount_paise); setMessage('Refund request sent to Razorpay.'); await load() }
    catch (e) { setError(e.response?.data?.error || 'Refund failed.') }
  }

  const deactivateAnnouncement = async (id) => {
    try { await announcementApi.deactivate(id); await load() } catch { /* non-critical */ }
  }

  // ---- Broadcasts ----
  const submitBroadcast = async (e) => {
    e.preventDefault(); setBusy(true); setError(''); setMessage(''); setBroadcastResult(null)
    try {
      const r = await broadcastApi.create(broadcastForm)
      setBroadcastResult(r.data)
      setMessage(`Message queued and simulated-sent to ${r.data.recipient_count} customers.`)
      setBroadcastForm(emptyBroadcast); await load()
    } catch (e) { setError(e.response?.data?.error || 'Could not send broadcast.') } finally { setBusy(false) }
  }

  const confirmedRevenue = useMemo(() => money(stats?.revenue_paise), [stats])

  return (
    <section className="admin-page">
      <div className="container">
        <div className="admin-hero">
          <div>
            <div className="eyebrow">Operations console</div>
            <h1>Admin control center</h1>
            <p>Manage catalog inventory, run multiple flash drops at once, message customers, and watch performance from one place.</p>
          </div>
          <div className="admin-badge"><Icon name="shield" size={18} /> ADMIN</div>
        </div>

        {error && <div className="notice error admin-notice">{error}</div>}
        {message && <div className="notice admin-notice">{message}</div>}

        <div className="admin-stats">
          {[['Products', stats?.products ?? '—', 'Catalog'], ['Customers', stats?.users ?? '—', 'Registered users'],
            ['Orders', stats?.orders ?? '—', 'All orders'], ['Revenue', confirmedRevenue, 'Confirmed'],
            ['Live sales', activeSales.length, 'Flash sales running now']].map((x) => (
            <div className="admin-stat" key={x[0]}><span>{x[0]}</span><strong>{x[1]}</strong><small>{x[2]}</small></div>
          ))}
        </div>

        <div className="admin-tabs">
          {TABS.map(([key, label]) => (
            <button key={key} className={`admin-tab ${tab === key ? 'active' : ''}`} onClick={() => setTab(key)}>{label}</button>
          ))}
        </div>

        {tab === 'catalog' && (
          <div className="admin-grid">
            <div className="admin-card product-editor">
              <div className="card-heading">
                <div><div className="eyebrow">Catalog management</div><h2>{editing ? 'Edit product' : 'Add product'}</h2></div>
                {editing && <button className="ghost-btn" onClick={() => { setEditing(false); setForm(emptyForm) }}>Cancel</button>}
              </div>
              <form onSubmit={submitProduct} className="admin-form">
                <label>Name<input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Product name" /></label>
                <div className="two-col">
                  <label>Brand<input value={form.brand} onChange={(e) => setForm({ ...form, brand: e.target.value })} placeholder="Brand" /></label>
                  <label>Category
                    <select value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value })}>
                      <option>Electronics</option><option>Mobiles</option><option>Fashion</option><option>Home</option><option>Appliances</option><option>Beauty</option><option>Grocery</option><option>Snacks</option>
                    </select>
                  </label>
                </div>
                <label>Image URL
                  <input value={form.image_url} onChange={(e) => setForm({ ...form, image_url: e.target.value })} placeholder="https://example.com/product.jpg" />
                </label>
                {form.image_url && (
                  <div className="image-preview">
                    <img src={form.image_url} alt="Preview" onError={(e) => { e.target.style.display = 'none' }} />
                  </div>
                )}
                <label>Description<textarea rows="3" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} placeholder="Short product description" /></label>
                <div className="two-col">
                  <label>Price (₹)<input required type="number" min="0" step="0.01" value={form.price_paise === '' ? '' : Number(form.price_paise) / 100} onChange={(e) => setForm({ ...form, price_paise: Math.round(Number(e.target.value || 0) * 100) })} /></label>
                  <label>Stock<input required type="number" min="0" value={form.stock} onChange={(e) => setForm({ ...form, stock: e.target.value })} /></label>
                </div>
                <label>Tax rate included in price (%)<input type="number" min="0" max="100" step="0.01" value={Number(form.tax_rate_bps || 0) / 100} onChange={(e) => setForm({ ...form, tax_rate_bps: Math.round(Number(e.target.value || 0) * 100) })} /><small>Tax is treated as included in the listed price, so invoice total remains the amount charged.</small></label>
                <button className="primary-btn wide" disabled={busy}>{busy ? (editing ? 'Saving…' : 'Creating…') : (editing ? 'Save changes' : 'Create product')} <Icon name="arrow" size={17} /></button>
              </form>
            </div>

            <div className="admin-card table-card">
              <div className="card-heading"><div><div className="eyebrow">Catalog</div><h2>Products</h2></div><button className="ghost-btn" onClick={load}>Refresh</button></div>
              <div className="table-wrap">
                <table>
                  <thead><tr><th>Image</th><th>Product</th><th>Category</th><th>Price</th><th>Stock</th><th>Actions</th></tr></thead>
                  <tbody>
                    {products.map((p) => (
                      <tr key={p.id}>
                        <td>
                          <div className="table-thumb">
                            {p.image_url ? <img src={p.image_url} alt={p.name} onError={(e) => { e.target.style.display = 'none' }} /> : <span>{p.name?.slice(0, 1)}</span>}
                          </div>
                        </td>
                        <td><strong>{p.name}</strong><small>{p.brand || 'No brand'}</small></td>
                        <td>{p.category || '—'}</td>
                        <td>{money(p.price_paise)}</td>
                        <td><span className={`stock-chip ${p.stock < 10 ? 'low' : ''}`}>{p.stock}</span></td>
                        <td><div className="row-actions"><button onClick={() => editProduct(p)}>Edit</button><button className="danger-text" onClick={() => deleteProduct(p)}>Delete</button></div></td>
                      </tr>
                    ))}
                    {!products.length && <tr><td colSpan="6" className="empty-cell">No products in the catalog.</td></tr>}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        )}

        {tab === 'categories' && (
          <div className="admin-grid">
            <div className="admin-card">
              <div className="card-heading">
                <div><div className="eyebrow">Catalog taxonomy</div><h2>{categoryEditing ? 'Edit category' : 'Create category'}</h2></div>
                {categoryEditing && <button className="ghost-btn" onClick={() => { setCategoryEditing(false); setCategoryForm(emptyCategory) }}>Cancel</button>}
              </div>
              <form onSubmit={submitCategory} className="admin-form">
                <label>Name<input required maxLength="120" value={categoryForm.name} onChange={(e) => setCategoryForm({ ...categoryForm, name: e.target.value })} placeholder="Home audio" /></label>
                <label>URL slug<input required pattern="[a-z0-9]+(-[a-z0-9]+)*" value={categoryForm.slug} onChange={(e) => setCategoryForm({ ...categoryForm, slug: e.target.value.toLowerCase() })} placeholder="home-audio" /></label>
                <div className="two-col">
                  <label>Parent category
                    <select value={categoryForm.parent_id} onChange={(e) => setCategoryForm({ ...categoryForm, parent_id: e.target.value })}>
                      <option value="">No parent</option>
                      {categories.filter((c) => c.id !== categoryForm.id).map((c) => <option value={c.id} key={c.id}>{c.name}</option>)}
                    </select>
                  </label>
                  <label>Sort order<input type="number" min="0" value={categoryForm.sort_order} onChange={(e) => setCategoryForm({ ...categoryForm, sort_order: e.target.value })} /></label>
                </div>
                <label>Description<textarea rows="2" value={categoryForm.description} onChange={(e) => setCategoryForm({ ...categoryForm, description: e.target.value })} /></label>
                <label>Banner image URL<input type="url" value={categoryForm.banner_image_url} onChange={(e) => setCategoryForm({ ...categoryForm, banner_image_url: e.target.value })} placeholder="https://..." /></label>
                <label>SEO title<input maxLength="255" value={categoryForm.seo_title} onChange={(e) => setCategoryForm({ ...categoryForm, seo_title: e.target.value })} /></label>
                <label>SEO description<textarea rows="2" maxLength="500" value={categoryForm.seo_description} onChange={(e) => setCategoryForm({ ...categoryForm, seo_description: e.target.value })} /></label>
                <label className="category-active"><input type="checkbox" checked={categoryForm.active} onChange={(e) => setCategoryForm({ ...categoryForm, active: e.target.checked })} /> Active</label>
                <button className="primary-btn wide" disabled={busy}>{busy ? 'Saving…' : categoryEditing ? 'Save category' : 'Create category'} <Icon name="arrow" size={17} /></button>
              </form>
            </div>
            <div className="admin-card table-card">
              <div className="card-heading"><div><div className="eyebrow">Store taxonomy</div><h2>Categories ({categories.length})</h2></div><button className="ghost-btn" onClick={load}>Refresh</button></div>
              <div className="table-wrap">
                <table>
                  <thead><tr><th>Category</th><th>Parent</th><th>Slug</th><th>Order</th><th>State</th><th>Actions</th></tr></thead>
                  <tbody>
                    {categories.map((category) => (
                      <tr key={category.id}>
                        <td><strong>{category.name}</strong><small>{category.children} child categories</small></td>
                        <td>{categories.find((parent) => parent.id === category.parent_id)?.name || 'Root'}</td>
                        <td><code>{category.slug}</code></td>
                        <td>{category.sort_order}</td>
                        <td><span className={`status-chip ${category.active ? 'live' : ''}`}>{category.active ? 'ACTIVE' : 'INACTIVE'}</span></td>
                        <td><div className="row-actions"><button onClick={() => editCategory(category)}>Edit</button><button className="danger-text" onClick={() => deleteCategory(category)}>Delete</button></div></td>
                      </tr>
                    ))}
                    {!categories.length && <tr><td colSpan="6" className="empty-cell">No categories created yet.</td></tr>}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        )}

        {tab === 'sales' && (
          <div className="admin-grid">
            <div className="admin-card sale-editor">
              <div className="card-heading"><div><div className="eyebrow">High-traffic control</div><h2>Launch flash sale</h2></div><Icon name="bolt" size={23} /></div>
              <form onSubmit={activateSale} className="admin-form">
                <label>Product
                  <select required value={sale.product_id} onChange={(e) => setSale({ ...sale, product_id: e.target.value })}>
                    {products.map((p) => <option value={p.id} key={p.id}>{p.name} · {money(p.price_paise)} · {p.stock} in stock</option>)}
                  </select>
                </label>
                <div className="two-col">
                  <label>Discount %<input type="number" min="1" max="99" value={sale.discount_percent} onChange={(e) => setSale({ ...sale, discount_percent: e.target.value })} /></label>
                  <label>Sale stock<input type="number" min="1" value={sale.stock} onChange={(e) => setSale({ ...sale, stock: e.target.value })} /></label>
                </div>
                <div className="two-col">
                  <label>Starts at<input required type="datetime-local" value={sale.starts_at} onChange={(e) => setSale({ ...sale, starts_at: e.target.value })} /></label>
                  <label>Ends at<input required type="datetime-local" value={sale.ends_at} onChange={(e) => setSale({ ...sale, ends_at: e.target.value })} /></label>
                </div>
                <div className="sale-rule"><Icon name="shield" size={17} /><span>Multiple products can run flash sales at once — each activation is independent and all show up on the storefront together.</span></div>
                <button className="primary-btn wide" disabled={busy}>Activate sale <Icon name="bolt" size={17} /></button>
                {saleResult && <div className="sale-result"><b>Sale is live — it will appear on the storefront's Flash Sale page automatically.</b><code>{saleResult.sale_id}</code></div>}
              </form>
            </div>

            <div className="admin-card table-card">
              <div className="card-heading"><div><div className="eyebrow">Live now</div><h2>Active flash sales ({activeSales.length})</h2></div><button className="ghost-btn" onClick={load}>Refresh</button></div>
              {activeSales.length ? (
                <div className="sales-list">
                  {activeSales.map((s) => {
                    const product = products.find((p) => p.id === s.product_id)
                    return (
                      <div className="sale-list-item" key={s.sale_id}>
                        <div className="sale-list-thumb">
                          {product?.image_url ? <img src={product.image_url} alt={product.name} onError={(e) => { e.target.style.display = 'none' }} /> : <span>⚡</span>}
                        </div>
                        <div className="sale-list-info">
                          <strong>{product?.name || s.product_id}</strong>
                          <span>{money(s.sale_price_paise)} · {s.discount_percent}% off · <span className={`status-chip ${s.state === 'ACTIVE' ? 'live' : ''}`}>{s.state}</span></span>
                          <small>{s.stock_remaining} left · {s.views} views · {s.clicks} clicks</small>
                        </div>
                        <div className="sale-list-actions">
                          <button className="danger-text" onClick={() => deactivateSale(s.sale_id)}>End sale</button>
                        </div>
                      </div>
                    )
                  })}
                </div>
              ) : <div className="empty-cell">No flash sales running. Launch one on the left.</div>}
            </div>
          </div>
        )}

        {tab === 'announcements' && (
          <div className="admin-grid">
            <div className="admin-card">
              <div className="card-heading"><div><div className="eyebrow">Storefront banner</div><h2>Post an announcement</h2></div><Icon name="bell" size={23} /></div>
              <form onSubmit={submitAnnouncement} className="admin-form">
                <label>Message<input required value={announcementForm.message} onChange={(e) => setAnnouncementForm({ ...announcementForm, message: e.target.value })} placeholder="Diwali Sale Begins! 🎉" /></label>
                <label>Link (optional)<input value={announcementForm.link} onChange={(e) => setAnnouncementForm({ ...announcementForm, link: e.target.value })} placeholder="https://yoursite.com/diwali-sale" /></label>
                <div className="sale-rule"><Icon name="bell" size={17} /><span>Shows as a dismissible bell-icon banner at the top of every page.</span></div>
                <button className="primary-btn wide" disabled={busy}>Publish announcement <Icon name="arrow" size={17} /></button>
              </form>
            </div>

            <div className="admin-card table-card">
              <div className="card-heading"><div><div className="eyebrow">History</div><h2>Announcements</h2></div></div>
              {announcements.length ? (
                <div className="announcement-list">
                  {announcements.map((a) => (
                    <div className="announcement-item" key={a.id}>
                      <div>
                        <strong>{a.message}</strong>
                        {a.link && <small><a href={a.link} target="_blank" rel="noopener noreferrer">{a.link}</a></small>}
                      </div>
                      <div className="row-actions">
                        <span className={`status-chip ${a.active ? 'live' : ''}`}>{a.active ? 'ACTIVE' : 'off'}</span>
                        {a.active && <button className="danger-text" onClick={() => deactivateAnnouncement(a.id)}>Turn off</button>}
                      </div>
                    </div>
                  ))}
                </div>
              ) : <div className="empty-cell">No announcements yet.</div>}
            </div>
          </div>
        )}

        {tab === 'broadcasts' && (
          <div className="admin-grid">
            <div className="admin-card">
              <div className="card-heading"><div><div className="eyebrow">Message queue</div><h2>Announce a pre-festival sale</h2></div><Icon name="share" size={23} /></div>
              <form onSubmit={submitBroadcast} className="admin-form">
                <label>Channel
                  <select value={broadcastForm.channel} onChange={(e) => setBroadcastForm({ ...broadcastForm, channel: e.target.value })}>
                    <option value="EMAIL">Email</option>
                    <option value="WHATSAPP">WhatsApp</option>
                    <option value="BOTH">Email + WhatsApp</option>
                  </select>
                </label>
                <label>Message<textarea required rows="3" value={broadcastForm.message} onChange={(e) => setBroadcastForm({ ...broadcastForm, message: e.target.value })} placeholder="Diwali mega sale starts tomorrow at 9 AM — set your alarms!" /></label>
                <div className="sale-rule"><Icon name="shield" size={17} /><span>Sends are simulated (logged server-side) until a real Email/WhatsApp provider is wired in.</span></div>
                <button className="primary-btn wide" disabled={busy}>Send to all customers <Icon name="arrow" size={17} /></button>
                {broadcastResult && <div className="sale-result"><b>Delivered (simulated) to {broadcastResult.recipient_count} customers.</b></div>}
              </form>
            </div>

            <div className="admin-card table-card">
              <div className="card-heading"><div><div className="eyebrow">History</div><h2>Recent broadcasts</h2></div></div>
              <div className="table-wrap">
                <table>
                  <thead><tr><th>Channel</th><th>Message</th><th>Recipients</th><th>Status</th></tr></thead>
                  <tbody>
                    {broadcasts.map((b) => (
                      <tr key={b.id}><td>{b.channel}</td><td>{b.message}</td><td>{b.recipient_count}</td><td><span className="status-chip">{b.status}</span></td></tr>
                    ))}
                    {!broadcasts.length && <tr><td colSpan="4" className="empty-cell">No broadcasts sent yet.</td></tr>}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        )}

        {tab === 'analytics' && (
          <>
          <div className="admin-card table-card">
            <div className="card-heading"><div><div className="eyebrow">Performance</div><h2>Flash sale funnel: views → clicks → orders</h2></div></div>
            <div className="table-wrap">
              <table>
                <thead><tr><th>Sale</th><th>Views</th><th>Clicks</th><th>Confirmed orders</th><th>Revenue</th><th>Click-through</th></tr></thead>
                <tbody>
                  {analyticsOverview.map((row) => (
                    <tr key={row.sale_id}>
                      <td><code>{row.sale_id.slice(0, 8)}…</code></td>
                      <td>{row.views}</td>
                      <td>{row.clicks}</td>
                      <td>{row.confirmed_orders}</td>
                      <td>{money(row.revenue_paise)}</td>
                      <td>{row.views > 0 ? `${((row.clicks / row.views) * 100).toFixed(1)}%` : '—'}</td>
                    </tr>
                  ))}
                  {!analyticsOverview.length && <tr><td colSpan="6" className="empty-cell">No flash-sale analytics yet — run a sale first.</td></tr>}
                </tbody>
              </table>
            </div>
          </div>
          <div className="admin-card table-card">
            <div className="card-heading"><div><div className="eyebrow">Promotion performance</div><h2>Coupon usage</h2></div></div>
            <div className="table-wrap">
              <table>
                <thead><tr><th>Coupon</th><th>Type</th><th>Successful uses</th><th>Discount total</th><th>Reserved</th><th>Released</th></tr></thead>
                <tbody>
                  {couponAnalytics.map((row) => <tr key={row.code}><td><strong>{row.code}</strong><small>{row.description}</small></td><td>{row.offer_type}</td><td>{row.applied_count}</td><td>{money(row.discount_paise)}</td><td>{row.reserved_count}</td><td>{row.released_count}</td></tr>)}
                  {!couponAnalytics.length && <tr><td colSpan="6" className="empty-cell">No coupon analytics available.</td></tr>}
                </tbody>
              </table>
            </div>
          </div>
          </>
        )}

        {tab === 'payments' && (
          <div className="admin-card table-card">
            <div className="card-heading"><div><div className="eyebrow">Payment monitoring</div><h2>Recent transactions</h2></div><button className="ghost-btn" onClick={load}>Refresh</button></div>
            <div className="payment-monitor-cards">
              <div><b>{payments.filter(p => p.status === 'CAPTURED').length}</b><span>Captured</span></div>
              <div><b>{payments.filter(p => p.status === 'CREATED').length}</b><span>Awaiting payment</span></div>
              <div><b>{payments.filter(p => p.status === 'FAILED').length}</b><span>Failed</span></div>
            </div>
            <div className="table-wrap">
              <table><thead><tr><th>Order</th><th>Customer</th><th>Amount</th><th>Method</th><th>Status</th><th>Razorpay payment</th><th>Action</th></tr></thead>
              <tbody>{payments.map(p => <tr key={p.id}><td><code>{p.order_id.slice(0,8)}…</code></td><td>{p.email}</td><td>{money(p.amount_paise)}</td><td>{p.method}</td><td><span className={`status-chip ${p.status==='CAPTURED'?'live':''}`}>{p.status}</span></td><td><code>{p.razorpay_payment_id || '—'}</code></td><td>{p.status === 'CAPTURED' && <button className="danger-text" onClick={(e)=>{e.preventDefault();refundPayment(p)}}>Refund</button>}</td></tr>)}{!payments.length&&<tr><td colSpan="7" className="empty-cell">No Razorpay transactions yet.</td></tr>}</tbody></table>
            </div>
            <div className="sale-rule"><Icon name="shield" size={17}/><span>Refunds use the Razorpay Refund API in Test Mode. Production refunds should add stronger audit controls and role/approval policies.</span></div>
          </div>
        )}

        {tab === 'orders' && (
          <div className="admin-card table-card">
            <div className="card-heading"><div><div className="eyebrow">Order operations</div><h2>Recent orders</h2></div></div>
            <div className="table-wrap">
              <table>
                <thead><tr><th>Order</th><th>Customer</th><th>Total</th><th>Status</th><th>Source</th><th>Payment</th></tr></thead>
                <tbody>
                  {orders.map((o) => (
                    <tr key={o.id}><td><code>{o.id.slice(0, 8)}…</code></td><td>{o.email}</td><td>{money(o.total_paise)}</td><td><span className="status-chip">{o.status}</span></td><td>{o.source}</td><td>{o.payment_method}</td></tr>
                  ))}
                  {!orders.length && <tr><td colSpan="6" className="empty-cell">No orders yet.</td></tr>}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>
    </section>
  )
}
