import { useEffect, useState } from 'react'
import { useAuth } from '../context/AuthContext'
import { accountApi } from '../services/api'
import { Link } from 'react-router-dom'

const empty = { label:'Home', full_name:'', phone:'', line1:'', line2:'', city:'', state:'', pincode:'', is_default:true }

export default function Profile() {
  const { user, isAdmin } = useAuth()
  const [addresses,setAddresses]=useState([])
  const [form,setForm]=useState(empty)
  const [msg,setMsg]=useState('')
  const load=()=>accountApi.addresses().then(r=>setAddresses(r.data||[])).catch(()=>{})
  useEffect(()=>{load()},[])
  const save=async e=>{e.preventDefault();setMsg('');try{await accountApi.createAddress(form);setForm(empty);await load();setMsg('Address saved successfully.')}catch(err){setMsg(err.response?.data?.error||'Could not save address.')}}
  const remove=async id=>{try{await accountApi.deleteAddress(id);load()}catch{}}
  return <section className="page profile-page"><div className="container">
    <div className="profile-head"><div><div className="eyebrow">My account</div><h1>Account & profile</h1><p>Manage your personal details, saved addresses and orders.</p></div><Link to="/orders" className="secondary-btn">View orders →</Link></div>
    <div className="profile-grid">
      <div className="profile-card identity-card"><div className="profile-avatar">{isAdmin?'A':'F'}</div><div><h2>{isAdmin?'Administrator':'FlashCart customer'}</h2><p>{user?.id}</p></div><span className="verified-pill">✓ Verified account</span></div>
      <div className="profile-card"><div className="card-heading"><div><div className="eyebrow">Delivery</div><h2>Saved addresses</h2></div></div>
        {addresses.length ? addresses.map(a=><div className="address-row" key={a.id}><div><b>{a.label} {a.is_default&&<small>DEFAULT</small>}</b><p>{a.full_name}, {a.phone}<br/>{a.line1}{a.line2&&`, ${a.line2}`}<br/>{a.city}, {a.state} - {a.pincode}</p></div><button className="danger-text" onClick={()=>remove(a.id)}>Remove</button></div>) : <div className="empty-cell">No saved addresses yet.</div>}
      </div>
      <form className="profile-card admin-form" onSubmit={save}><div className="card-heading"><div><div className="eyebrow">Add new</div><h2>Delivery address</h2></div></div>
        <div className="two-col"><label>Label<input value={form.label} onChange={e=>setForm({...form,label:e.target.value})}/></label><label>Full name<input required value={form.full_name} onChange={e=>setForm({...form,full_name:e.target.value})}/></label></div>
        <div className="two-col"><label>Phone<input required value={form.phone} onChange={e=>setForm({...form,phone:e.target.value})}/></label><label>PIN code<input required value={form.pincode} onChange={e=>setForm({...form,pincode:e.target.value})}/></label></div>
        <label>Address line 1<input required value={form.line1} onChange={e=>setForm({...form,line1:e.target.value})}/></label><label>Address line 2<input value={form.line2} onChange={e=>setForm({...form,line2:e.target.value})}/></label>
        <div className="two-col"><label>City<input required value={form.city} onChange={e=>setForm({...form,city:e.target.value})}/></label><label>State<input value={form.state} onChange={e=>setForm({...form,state:e.target.value})}/></label></div>
        {msg&&<div className="notice">{msg}</div>}<button className="primary-btn wide">Save address</button>
      </form>
    </div>
  </div></section>
}
