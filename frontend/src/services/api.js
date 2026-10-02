import axios from "axios"

const baseURL = import.meta.env.VITE_API_URL || "/api"
const api = axios.create({
  baseURL,
  headers: { "Content-Type": "application/json" },
  timeout: 10000,
})

const ACCESS = "flashcart_access_token"
const REFRESH = "flashcart_refresh_token"
const USER = "flashcart_user"
let refreshPromise = null

api.interceptors.request.use((config) => {
  const token = localStorage.getItem(ACCESS)
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

api.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config
    if (!original || error.response?.status !== 401 || original._retry) {
      return Promise.reject(error)
    }

    // Never refresh when the refresh/login/register endpoint itself fails.
    const authEndpoint = ["/users/login", "/users/register", "/users/refresh"].some((path) =>
      original.url?.includes(path),
    )
    if (authEndpoint) return Promise.reject(error)

    const refreshToken = localStorage.getItem(REFRESH)
    if (!refreshToken) return Promise.reject(error)

    original._retry = true

    try {
      if (!refreshPromise) {
        refreshPromise = axios
          .post(`${baseURL}/users/refresh`, { refresh_token: refreshToken }, {
            headers: { "Content-Type": "application/json" },
            timeout: 10000,
          })
          .then(({ data }) => {
            localStorage.setItem(ACCESS, data.access_token)
            if (data.refresh_token) localStorage.setItem(REFRESH, data.refresh_token)
            if (data.user_id) {
              const existing = JSON.parse(localStorage.getItem(USER) || "{}")
              localStorage.setItem(USER, JSON.stringify({
                ...existing,
                id: data.user_id,
                role: data.role || existing.role || "USER",
              }))
            }
            return data.access_token
          })
          .finally(() => {
            refreshPromise = null
          })
      }

      const newAccessToken = await refreshPromise
      original.headers = original.headers || {}
      original.headers.Authorization = `Bearer ${newAccessToken}`
      return api(original)
    } catch (refreshError) {
      localStorage.removeItem(ACCESS)
      localStorage.removeItem(REFRESH)
      localStorage.removeItem(USER)
      return Promise.reject(refreshError)
    }
  },
)

export const authApi = {
  register: (body) => api.post("/users/register", body),
  login: (body) => api.post("/users/login", body),
  refresh: (body) => api.post("/users/refresh", body),
}

export const productApi = {
  list: (params) => api.get("/products/", { params }),
  get: (id) => api.get(`/products/${id}`),
  create: (body) => api.post("/products/", body),
  update: (id, body) => api.put(`/products/${id}`, body),
  remove: (id) => api.delete(`/products/${id}`),
}

export const cartApi = {
  get: () => api.get("/cart/"),
  add: (body) => api.post("/cart/items", body),
  update: (id, body) => api.put(`/cart/items/${id}`, body),
  remove: (id) => api.delete(`/cart/items/${id}`),
  coupons: (params) => api.get("/cart/coupons", { params }),
  validateCoupon: (body) => api.post("/cart/coupons/validate", body),
  checkout: (key, paymentMethod, couponCode = '', paymentInstrument = '', paymentIssuer = '') =>
    api.post("/cart/checkout", { payment_method: paymentMethod, coupon_code: couponCode, payment_instrument: paymentInstrument, payment_issuer: paymentIssuer }, { headers: { "Idempotency-Key": key } }),
}

export const flashSaleApi = {
  current: () => api.get(`/flash-sales/current`),
  active: () => api.get(`/flash-sales/active`),
  status: (id) => api.get(`/flash-sales/${id}`),
  buy: (id, quantity, paymentMethod, key) =>
    api.post(`/flash-sales/${id}/buy`, { quantity, payment_method: paymentMethod }, { headers: { "Idempotency-Key": key } }),
  activate: (body) => api.post("/flash-sales/activate", body),
  deactivate: (id) => api.post(`/flash-sales/${id}/deactivate`),
  track: (id, event) => api.post(`/flash-sales/${id}/track`, { event }).catch(() => {}),
}

export const announcementApi = {
  active: () => api.get("/announcements/active"),
  list: () => api.get("/admin/announcements"),
  create: (body) => api.post("/admin/announcements", body),
  deactivate: (id) => api.post(`/admin/announcements/${id}/deactivate`),
}

export const broadcastApi = {
  list: () => api.get("/admin/broadcasts"),
  create: (body) => api.post("/admin/broadcasts", body),
}

export const wishlistApi = {
  list: () => api.get("/wishlist/"),
  add: (productId) => api.post(`/wishlist/${productId}`),
  remove: (productId) => api.delete(`/wishlist/${productId}`),
}

export const subscriptionApi = {
  subscribe: (body) => api.post("/subscriptions", body),
}

export const analyticsApi = {
  overview: () => api.get("/admin/analytics/flash-sales"),
  flashSale: (id) => api.get(`/admin/analytics/flash-sales/${id}`),
  coupons: () => api.get("/admin/analytics/coupons"),
}

export const orderApi = {
  list: () => api.get("/orders/"),
  get: (id) => api.get(`/orders/${id}`),
  tracking: (id) => api.get(`/orders/${id}/tracking`),
}
export const invoiceApi = {
  get: (orderId) => api.get(`/orders/${orderId}/invoice`),
  pdf: (orderId) => api.get(`/orders/${orderId}/invoice.pdf`, { responseType: "blob" }),
}
export const adminApi = { stats: () => api.get("/admin/stats"), orders: () => api.get("/admin/orders"), payments: () => api.get("/admin/payments"), refundPayment: (id, amount_paise) => api.post(`/admin/payments/${id}/refund`, { amount_paise }) }
export const categoryApi = {
  list: () => api.get("/admin/categories"),
  create: (body) => api.post("/admin/categories", body),
  update: (id, body) => api.put(`/admin/categories/${id}`, body),
  remove: (id) => api.delete(`/admin/categories/${id}`),
}
export const healthApi = { check: () => api.get("/healthz") }
export const aiApi = {
  suggest: (body) => api.post("/ai/suggestions", body, { timeout: 45000 }),
}
export const festivalApi = {
  current: () => api.get("/festivals/current"),
  vapidKey: () => api.get("/push/public-key"),
  subscribePush: (subscription) => api.post("/account/push-subscriptions", subscription),
  unsubscribePush: (endpoint) => api.delete("/account/push-subscriptions", { data: { endpoint } }),
}
export default api
export const paymentApi = {
  createRazorpayOrder: (orderId) => api.post("/payments/razorpay/order", { order_id: orderId }),
  verifyRazorpay: (body) => api.post("/payments/razorpay/verify", body),
  status: (orderId) => api.get(`/payments/orders/${orderId}`),
}

export const accountApi = {
  addresses: () => api.get("/account/addresses"),
  createAddress: (body) => api.post("/account/addresses", body),
  deleteAddress: (id) => api.delete(`/account/addresses/${id}`),
  theme: () => api.get("/account/theme"),
  saveTheme: (theme) => api.put("/account/theme", { theme }),
}
