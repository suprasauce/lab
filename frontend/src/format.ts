const currency = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', minimumFractionDigits: 2, maximumFractionDigits: 2 })
const dateTime = new Intl.DateTimeFormat('en-IN', { dateStyle: 'medium', timeStyle: 'short' })
const date = new Intl.DateTimeFormat('en-IN', { dateStyle: 'medium' })

export function formatMoney(value: number | null) {
  return value === null ? '—' : currency.format(value)
}

export function formatTimestamp(value: string | null) {
  return value === null ? '—' : dateTime.format(new Date(value))
}

export function formatExpiry(value: string) {
  // Expiry is a calendar date, not a UTC timestamp.
  return date.format(new Date(`${value}T00:00:00`))
}
