import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from '../context/AuthContext'
export default function AdminRoute({children}){const {isAuthenticated,isAdmin}=useAuth();const location=useLocation();if(!isAuthenticated)return <Navigate to="/login" replace state={{from:location.pathname}}/>;if(!isAdmin)return <Navigate to="/" replace/>;return children}
