// Thin wrapper over the ERPX API. Every call returns parsed JSON and throws an
// Error carrying the server's message, which the pages surface directly.
async function request(path, options = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (res.status === 204) return null

  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new Error(body?.message || body?.error || `Request failed (${res.status})`)
  }
  return body
}

export const api = {
  vendors: (q = '') => request(`/api/vendors?q=${encodeURIComponent(q)}`),
  createVendor: (v) => request('/api/vendors', { method: 'POST', body: JSON.stringify(v) }),
  updateVendor: (code, v) =>
    request(`/api/vendors/${encodeURIComponent(code)}`, { method: 'PUT', body: JSON.stringify(v) }),
  deleteVendor: (code) =>
    request(`/api/vendors/${encodeURIComponent(code)}`, { method: 'DELETE' }),
  matchVendor: (params) => request(`/api/vendors/match?${new URLSearchParams(params)}`),

  invoices: () => request('/api/invoices'),
  deleteInvoice: (id) => request(`/api/invoices/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  events: () => request('/api/events'),
  clearEvents: () => request('/api/events', { method: 'DELETE' }),
}

export function formatMoney(value, currency) {
  if (value === undefined || value === null) return ''
  const formatted = new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value)
  return currency ? `${formatted} ${currency}` : formatted
}

export function formatTime(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleString()
}
