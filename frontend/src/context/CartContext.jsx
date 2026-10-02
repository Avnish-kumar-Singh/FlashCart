import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { cartApi, productApi } from '../services/api'
import { useAuth } from './AuthContext'

const CART_KEY = 'flashcart_guest_cart'
const CartContext = createContext(null)

function readGuestCart() {
  try {
    const raw = localStorage.getItem(CART_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

function writeGuestCart(items) {
  localStorage.setItem(CART_KEY, JSON.stringify(items))
}

export function CartProvider({ children }) {
  const { isAuthenticated } = useAuth()
  const [items, setItems] = useState(() => readGuestCart())
  const [loading, setLoading] = useState(false)

  const syncGuestCart = useCallback((nextItems) => {
    setItems(nextItems)
    writeGuestCart(nextItems)
  }, [])

  const refreshCart = useCallback(async () => {
    if (!isAuthenticated) {
      syncGuestCart(readGuestCart())
      return
    }
    setLoading(true)
    try {
      const { data } = await cartApi.get()
      const enriched = await Promise.all((data.items || []).map(async (item) => {
        try {
          const { data: product } = await productApi.get(item.product_id)
          return {
            ...item,
            name: product?.name || item.name || '',
            price_paise: product?.price_paise ?? item.price_paise ?? 0,
            unit_price_paise: product?.price_paise ?? item.unit_price_paise ?? item.price_paise ?? 0,
            image_url: product?.image_url || item.image_url || '',
          }
        } catch {
          return {
            ...item,
            name: item.name || '',
            price_paise: item.price_paise ?? 0,
            unit_price_paise: item.unit_price_paise ?? item.price_paise ?? 0,
            image_url: item.image_url || '',
          }
        }
      }))
      setItems(enriched)
      localStorage.removeItem(CART_KEY)
    } finally {
      setLoading(false)
    }
  }, [isAuthenticated, syncGuestCart])

  useEffect(() => {
    refreshCart()
  }, [refreshCart])

  useEffect(() => {
    if (!isAuthenticated) {
      syncGuestCart(readGuestCart())
      return
    }

    const guestItems = readGuestCart()
    if (!guestItems.length) return

    const mergeGuestCart = async () => {
      for (const item of guestItems) {
        try {
          await cartApi.add({ product_id: item.product_id, quantity: item.quantity })
        } catch {
          // Ignore transient sync errors; user can retry later.
        }
      }
      localStorage.removeItem(CART_KEY)
      await refreshCart()
    }

    mergeGuestCart()
  }, [isAuthenticated, refreshCart, syncGuestCart])

  const add = async (productId, quantity = 1) => {
    if (!isAuthenticated) {
      const existing = readGuestCart()
      const idx = existing.findIndex((item) => item.product_id === productId)
      const next = [...existing]

      if (idx >= 0) {
        next[idx] = { ...next[idx], quantity: next[idx].quantity + quantity }
      } else {
        try {
          const { data } = await productApi.get(productId)
          next.push({
            product_id: productId,
            quantity,
            name: data?.name || '',
            price_paise: data?.price_paise || 0,
            unit_price_paise: data?.price_paise || 0,
            image_url: data?.image_url || '',
          })
        } catch {
          next.push({ product_id: productId, quantity, name: '', price_paise: 0, unit_price_paise: 0, image_url: '' })
        }
      }

      syncGuestCart(next)
      return
    }

    await cartApi.add({ product_id: productId, quantity })
    await refreshCart()
  }

  const update = async (productId, quantity) => {
    if (!isAuthenticated) {
      const next = readGuestCart()
        .map((item) => item.product_id === productId ? { ...item, quantity } : item)
        .filter((item) => item.quantity > 0)
      syncGuestCart(next)
      return
    }
    await cartApi.update(productId, { quantity })
    await refreshCart()
  }

  const remove = async (productId) => {
    if (!isAuthenticated) {
      const next = readGuestCart().filter((item) => item.product_id !== productId)
      syncGuestCart(next)
      return
    }
    await cartApi.remove(productId)
    await refreshCart()
  }

  const value = useMemo(() => ({ items, count: items.reduce((n, i) => n + i.quantity, 0), loading, refreshCart, add, update, remove }), [items, loading, refreshCart])
  return <CartContext.Provider value={value}>{children}</CartContext.Provider>
}
export const useCart = () => useContext(CartContext)
