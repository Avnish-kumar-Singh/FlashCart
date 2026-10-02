import { Link } from 'react-router-dom'
import { useCart } from '../context/CartContext'
import { useAuth } from '../context/AuthContext'
import { wishlistApi } from '../services/api'
import Icon from './Icon'
import { useState } from 'react'

const colors=['tone-blue','tone-purple','tone-green','tone-orange','tone-slate']
export default function ProductCard({product,index=0}){
 const {add}=useCart(); const {isAuthenticated}=useAuth(); const [added,setAdded]=useState(false)
 const [wished,setWished]=useState(false); const [wishBusy,setWishBusy]=useState(false)
 const price=(product.price_paise||0)/100
 const stock=Number(product.stock||0)
 const addToCart=async e=>{e.preventDefault();e.stopPropagation(); try{await add(product.id);setAdded(true);setTimeout(()=>setAdded(false),1400)}catch{}}
 const toggleWish=async e=>{
   e.preventDefault();e.stopPropagation()
   if(!isAuthenticated){window.location.href='/login';return}
   setWishBusy(true)
   try{
     if(wished){await wishlistApi.remove(product.id);setWished(false)}
     else{await wishlistApi.add(product.id);setWished(true)}
   }catch{}finally{setWishBusy(false)}
 }
 return <article className="product-card">
   <Link to={`/products/${product.id}`} className={`product-art ${colors[index%colors.length]}`} aria-label={`View ${product.name}`}>
     {product.image_url
       ? <img src={product.image_url} alt={product.name} className="product-image" onError={e=>{e.target.style.display='none'}}/>
       : <span>{product.category?.slice(0,1)||'P'}</span>}
     <div className="art-glow"/>
   </Link>
   <button className={`wish-btn ${wished?'active':''}`} onClick={toggleWish} disabled={wishBusy} aria-label={`Save ${product.name} to wishlist`} title="Save to wishlist"><Icon name="heart" size={16}/></button>
   <div className="product-info">
     <div className="product-taxonomy"><span>{product.category || 'Marketplace'}</span><span className={stock < 6 ? 'stock-warning' : 'stock-ready'}>{stock <= 0 ? 'Out of stock' : stock < 6 ? `Only ${stock} left` : 'In stock'}</span></div>
     <Link to={`/products/${product.id}`} className="product-card-title"><h3>{product.name}</h3></Link>
     <p>{product.brand || 'FlashCart Select'}</p>
     <div className="price-row"><strong>₹{price.toLocaleString('en-IN')}</strong><span>Free delivery</span></div>
     <button className={`add-btn ${added?'added':''}`} onClick={addToCart} disabled={stock <= 0}>{added?<><Icon name="check" size={16}/> Added</>:stock <= 0 ? 'Unavailable' : <>Add to cart <Icon name="arrow" size={15}/></>}</button>
   </div>
 </article>
}
