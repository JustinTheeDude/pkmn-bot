async function req(path, options = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const text = await res.text()
  let data
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { raw: text }
  }
  if (!res.ok) {
    throw new Error((data && data.error) || text || `HTTP ${res.status}`)
  }
  return data
}

export const api = {
  status: () => req('/api/status'),
  getSettings: () => req('/api/settings'),
  saveSettings: (s) => req('/api/settings', { method: 'PUT', body: JSON.stringify(s) }),
  listProducts: () => req('/api/products'),
  addProduct: (p) => req('/api/products', { method: 'POST', body: JSON.stringify(p) }),
  deleteProduct: (i) => req(`/api/products/${i}`, { method: 'DELETE' }),
  getProfile: () => req('/api/profile'),
  saveProfile: (p) => req('/api/profile', { method: 'PUT', body: JSON.stringify(p) }),
  startCheckout: (i, opts) => req(`/api/checkout/${i}`, { method: 'POST', body: JSON.stringify(opts || { headed: true }) }),
  checkoutRuns: () => req('/api/checkout/runs'),
  checkoutRun: (id) => req(`/api/checkout/runs/${id}`),
  listDrops: () => req('/api/drops'),
  scheduleDrop: (d) => req('/api/drops', { method: 'POST', body: JSON.stringify(d) }),
  cancelDrop: (id) => req(`/api/drops/${id}`, { method: 'DELETE' }),
  getDiscover: () => req('/api/discover'),
  saveDiscover: (d) => req('/api/discover', { method: 'PUT', body: JSON.stringify(d) }),
  listDiscovered: () => req('/api/discovered'),
}
