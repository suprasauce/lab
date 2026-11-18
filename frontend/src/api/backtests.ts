import { request } from './client'

export type BacktestStatus = 'PENDING' | 'COMPLETED' | 'FAILED'

export interface BacktestListItem {
  id: string
  status: BacktestStatus
  created_at: string
}

export interface BacktestTrade {
  id: string
  simulation_id: string
  tradingsymbol: string
  timestamp: string
  price: number
  quantity: number
}

export interface BacktestPosition {
  simulation_id: string
  tradingsymbol: string
  quantity: number
  average_price: number
  realized_pnl: number
  unrealized_pnl?: number | null
  total_pnl?: number | null
}

export type Backtest = { script: string } & (
  | { id: string; status: 'PENDING' }
  | { id: string; status: 'COMPLETED'; summary: { total_trades: number; total_pnl: number }; trades: BacktestTrade[]; positions: BacktestPosition[] }
  | { id: string; status: 'FAILED'; error?: string }
)

export function createBacktest(script: string, signal?: AbortSignal) {
  return request<{ id: string }>('/api/backtests', { method: 'POST', body: JSON.stringify({ script }), signal })
}

export function getBacktest(id: string, signal?: AbortSignal) {
  return request<Backtest>(`/api/backtests/${encodeURIComponent(id)}`, { signal })
}

export function listBacktests(signal?: AbortSignal) {
  return request<{ backtests: BacktestListItem[] }>('/api/backtests', { signal })
}
