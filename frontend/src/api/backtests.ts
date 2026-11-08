import { request } from './client'

export type BacktestStatus = 'PENDING' | 'COMPLETED' | 'FAILED'

export interface BacktestListItem {
  id: string
  status: BacktestStatus
  created_at: string
}

export interface BacktestTrade {
  expiry: string
  underlying: string
  strike: number
  option_type: 'CALL' | 'PUT'
  side: 'LONG' | 'SHORT'
  entry_timestamp: string
  exit_timestamp: string | null
  entry_price: number
  exit_price: number | null
  quantity: number
  pnl: number | null
}

export type Backtest =
  | { id: string; status: 'PENDING' }
  | { id: string; status: 'COMPLETED'; summary: { total_trades: number; total_pnl: number }; trades: BacktestTrade[] }
  | { id: string; status: 'FAILED'; error?: string }

export function createBacktest(script: string, signal?: AbortSignal) {
  return request<{ id: string }>('/api/backtests', { method: 'POST', body: JSON.stringify({ script }), signal })
}

export function getBacktest(id: string, signal?: AbortSignal) {
  return request<Backtest>(`/api/backtests/${encodeURIComponent(id)}`, { signal })
}

export function listBacktests(signal?: AbortSignal) {
  return request<{ backtests: BacktestListItem[] }>('/api/backtests', { signal })
}
