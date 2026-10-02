import { useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { invoiceApi, orderApi } from '../services/api'
import Icon from '../components/Icon'

const money = p => `₹${(Number(p || 0) / 100).toLocaleString('en-IN')}`

export default function Orders() {
  const { id } = useParams()
  const [search] = useSearchParams()
  const [orders,setOrders]=useState([])
  const [order,setOrder]=useState(null)
  const [err,setErr]=useState('')
  const [trackOrderId,setTrackOrderId]=useState('')
  const [trackResult,setTrackResult]=useState(null)
  const [trackErr,setTrackErr]=useState('')
  const [trackingLoaded,setTrackingLoaded]=useState(false)

  const statusFlow = [
    { key: 'CREATED', label: 'Processing' },
    { key: 'PAYMENT_PENDING', label: 'Payment pending' },
    { key: 'PAID', label: 'Shipped' },
    { key: 'CONFIRMED', label: 'Out for delivery' },
    { key: 'DELIVERED', label: 'Delivered' },
  ]

  const normalizeStatus = (status) => {
    const value = (status || '').toUpperCase()
    if (value === 'CREATED' || value === 'PAYMENT_PENDING') return 'Processing'
    if (value === 'PAID') return 'Shipped'
    if (value === 'CONFIRMED') return 'Out for delivery'
    if (value === 'ORDER_CANCELLED') return 'Cancelled'
    return value || 'Processing'
  }

  useEffect(()=>{ if(id) {
      orderApi.get(id).then(r=>setOrder(r.data)).catch(e=>setErr(e.response?.data?.error||'Order not found'))
      orderApi.tracking(id).then(r=>setTrackResult(r.data)).catch(()=>setTrackResult(null)).finally(()=>setTrackingLoaded(true))
    } else {
      orderApi.list().then(r=>setOrders(r.data||[])).catch(e=>setErr(e.response?.data?.error||'Could not load orders'))
    }
  },[id])

  const trackOrder = async (e) => {
    e.preventDefault()
    const value = trackOrderId.trim()
    if (!value) return
    setTrackErr('')
    try {
      const r = await orderApi.tracking(value)
      setTrackResult(r.data)
    } catch (e) {
      setTrackErr(e.response?.data?.error || 'Order not found. Please check the ID and try again.')
      setTrackResult(null)
    }
  }

  if(id) return <section className="page"><div className="container order-page">{err?<div className="empty-state"><h2>{err}</h2><Link to="/products" className="primary-btn">Continue shopping</Link></div>:!order?<div className="loading-state">Loading order…</div>:<><div className="order-success"><div className="success-icon"><Icon name="check" size={30}/></div><div><span className="eyebrow">{search.get('paid')?'Payment confirmed':'Order received'}</span><h1>{search.get('paid')?'Payment successful!':'Your order is on its way.'}</h1><p>Order <b>{order.order_id||id}</b> is moving through the FlashCart order pipeline.</p></div></div><div className="order-card"><div className="order-card-head"><div><span className="eyebrow">Order status</span><h2>{normalizeStatus(order.status)}</h2></div><Link to="/orders" className="ghost-btn">All orders</Link></div><div className="status-track">{(trackResult?.timeline || []).map(step => <span key={step.key} className={step.done || step.current ? 'done' : ''}>{step.label}</span>)}</div>{trackResult && <div className="tracking-status"><div className="tracking-status-head"><span className="eyebrow">Live status</span><b>{normalizeStatus(trackResult.status)}</b></div><div className="tracking-meta"><span>Courier: {trackResult.courier_name}</span><span>Tracking: {trackResult.tracking_number}</span></div></div>}{!trackingLoaded && <div className="notice">📦 Loading tracking details…</div>}<div className="order-detail-grid"><div><span>Payment</span><b>{order.payment_method}</b></div><div><span>Total</span><b>{money(order.total_paise)}</b></div><div><span>Items</span><b>{order.items?.reduce((n,i)=>n+i.quantity,0)||0}</b></div></div>{order.status==='CONFIRMED'&&<InvoiceActions orderID={id}/>}<Link to="/products" className="secondary-btn">Continue shopping</Link></div></>}</div></section>
  return <section className="page orders-list-page"><div className="container"><div className="profile-head"><div><div className="eyebrow">Account</div><h1>My orders</h1><p>Track your purchases and payment status.</p></div><Link to="/products" className="primary-btn">Continue shopping</Link></div>
    <div className="profile-card tracking-card" style={{marginBottom:'18px'}}>
      <div className="card-heading"><div><div className="eyebrow">Order lookup</div><h2>Track order by ID</h2></div></div>
      <form onSubmit={trackOrder} className="track-order-form">
        <input value={trackOrderId} onChange={e=>setTrackOrderId(e.target.value)} placeholder="Enter order ID" />
        <button type="submit" className="primary-btn">Track now</button>
      </form>
      {trackErr && <div className="notice error">{trackErr}</div>}
      {trackResult && <div className="tracking-status"><div className="tracking-status-head"><span className="eyebrow">Live status</span><b>{normalizeStatus(trackResult.status)}</b></div><div className="tracking-steps">{statusFlow.map(step => (
        <span key={step.key} className={trackResult.status && (trackResult.status === step.key || ['CREATED','PAYMENT_PENDING','PAID','CONFIRMED'].includes(trackResult.status) && step.key === 'CREATED')}>
          {step.label}
        </span>
      ))}</div></div>}
    </div>
    {err?<div className="notice error">{err}</div>:orders.length?<div className="orders-list">{orders.map(o=><Link to={`/orders/${o.id}`} className="order-list-card" key={o.id}><div className="order-list-icon"><Icon name="package" size={22}/></div><div className="order-list-main"><b>Order #{o.id.slice(0,8).toUpperCase()}</b><span>{new Date(o.created_at).toLocaleString()} · {o.payment_method}</span></div><strong>{money(o.total_paise)}</strong><span className={`status-chip ${o.status==='CONFIRMED'?'live':''}`}>{normalizeStatus(o.status)}</span><span>›</span></Link>)}</div>:<div className="empty-state"><div className="empty-icon">📦</div><h2>No orders yet</h2><p>Your placed orders will appear here.</p><Link to="/products" className="primary-btn">Start shopping</Link></div>}</div></section>
}

function InvoiceActions({ orderID }) {
  const [invoice, setInvoice] = useState(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let active = true
    invoiceApi.get(orderID)
      .then((response) => { if (active) setInvoice(response.data) })
      .catch((err) => { if (active) setError(err.response?.data?.error || 'Invoice is not available yet.') })
    return () => { active = false }
  }, [orderID])

  const openPDF = async (shouldPrint) => {
    const printWindow = shouldPrint ? window.open('about:blank', '_blank') : null
    if (shouldPrint && !printWindow) {
      setError('Allow pop-ups to print the invoice.')
      return
    }
    setBusy(true)
    setError('')
    try {
      const response = await invoiceApi.pdf(orderID)
      const url = URL.createObjectURL(new Blob([response.data], { type: 'application/pdf' }))
      if (shouldPrint) {
        printWindow.onload = () => {
          printWindow.focus()
          printWindow.print()
          window.setTimeout(() => URL.revokeObjectURL(url), 60000)
        }
        printWindow.location.href = url
      } else {
        const link = document.createElement('a')
        link.href = url
        link.download = `${invoice?.invoice_number || 'flashcart-invoice'}.pdf`
        document.body.appendChild(link)
        link.click()
        link.remove()
        window.setTimeout(() => URL.revokeObjectURL(url), 60000)
      }
    } catch (err) {
      printWindow?.close()
      setError(err.response?.data?.error || 'Could not load the invoice PDF.')
    } finally {
      setBusy(false)
    }
  }

  return <div className="invoice-actions">
    <div><span className="eyebrow">Purchase invoice</span><strong>{invoice?.invoice_number || (error ? 'Invoice unavailable' : 'Preparing invoice…')}</strong>{invoice && <small>{invoice.email_status === 'SENT' ? `Emailed to ${invoice.customer_email}` : invoice.email_status === 'FAILED' || invoice.email_status === 'SENDING' ? 'Email delivery is retrying' : 'Email delivery pending'}</small>}</div>
    <div className="invoice-action-buttons">
      <button className="ghost-btn" disabled={!invoice || busy} onClick={() => openPDF(false)}>Download PDF</button>
      <button className="primary-btn" disabled={!invoice || busy} onClick={() => openPDF(true)}>Print Invoice</button>
    </div>
    {error && <small className="invoice-error">{error}</small>}
  </div>
}
