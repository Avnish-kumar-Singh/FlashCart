import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import ProtectedRoute from './components/ProtectedRoute'
import AdminRoute from './components/AdminRoute'
import Home from './pages/Home'
import Products from './pages/Products'
import ProductDetails from './pages/ProductDetails'
import FlashSale from './pages/FlashSale'
import Wishlist from './pages/Wishlist'
import Cart from './pages/Cart'
import Checkout from './pages/Checkout'
import Orders from './pages/Orders'
import Login from './pages/Login'
import Register from './pages/Register'
import Admin from './pages/Admin'
import NotFound from './pages/NotFound'
import Profile from './pages/Profile'
import AIShoppingAssistant from './components/AIShoppingAssistant'

export default function App(){return <>
  <Routes>
    <Route path="/login" element={<Login/>}/><Route path="/register" element={<Register/>}/>
    <Route element={<Layout/>}>
      <Route path="/" element={<Home/>}/><Route path="/products" element={<Products/>}/><Route path="/products/:id" element={<ProductDetails/>}/><Route path="/flash-sale" element={<FlashSale/>}/>
      <Route path="/wishlist" element={<ProtectedRoute><Wishlist/></ProtectedRoute>}/>
      <Route path="/cart" element={<ProtectedRoute><Cart/></ProtectedRoute>}/><Route path="/checkout" element={<ProtectedRoute><Checkout/></ProtectedRoute>}/><Route path="/orders" element={<ProtectedRoute><Orders/></ProtectedRoute>}/><Route path="/account" element={<ProtectedRoute><Profile/></ProtectedRoute>}/><Route path="/orders/:id" element={<ProtectedRoute><Orders/></ProtectedRoute>}/>
      <Route path="/admin" element={<AdminRoute><Admin/></AdminRoute>}/>
      <Route path="*" element={<NotFound/>}/>
    </Route>
  </Routes>
  <AIShoppingAssistant />
</>}
