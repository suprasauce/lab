import type { BacktestPosition, BacktestTrade } from './api/backtests'

export interface ChartPoint {
  time: number
  value: number | null
}

export interface UnrealizedPnLSeries {
  id: string
  label: string
  color: string
  points: ChartPoint[]
}

interface OpenInterval {
  start: number
  end: number
}

function observations(position: BacktestPosition): ChartPoint[] {
  return (position.unrealized_pnl ?? [])
    .map((point) => ({ time: Date.parse(point.timestamp), value: point.pnl }))
    .filter((point) => Number.isFinite(point.time) && Number.isFinite(point.value))
    .sort((a, b) => a.time - b.time)
}

function openIntervals(position: BacktestPosition, trades: BacktestTrade[]): OpenInterval[] | null {
  const fills = trades.filter((trade) => trade.simulation_id === position.simulation_id && trade.tradingsymbol === position.tradingsymbol)
    .sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp))
  if (fills.length === 0) return null
  const intervals: OpenInterval[] = []
  let quantity = 0
  for (const fill of fills) {
    const time = Date.parse(fill.timestamp)
    if (!Number.isFinite(time) || !Number.isFinite(fill.quantity) || fill.quantity === 0) return null
    const next = quantity + fill.quantity
    if (quantity === 0) intervals.push({ start: time, end: Infinity })
    else if (next === 0) intervals[intervals.length - 1].end = time
    else if (Math.sign(next) !== Math.sign(quantity)) return null
    quantity = next
  }
  return quantity === position.quantity ? intervals : null
}

// Plot only recorded values, breaking the line when a symbol closes/reopens.
export function positionUnrealizedPnL(position: BacktestPosition, trades: BacktestTrade[]): ChartPoint[] {
  const intervals = openIntervals(position, trades)
  const points: ChartPoint[] = []
  let previousInterval = -1
  for (const point of observations(position)) {
    // A snapshot at the closing timestamp can be the final observed bar close.
    const interval = intervals?.findIndex((open) => point.time > open.start && point.time <= open.end) ?? -1
    if (points.length > 0 && (interval !== previousInterval || interval === -1)) points.push({ time: point.time, value: null })
    points.push(point)
    if (interval === -1) points.push({ time: point.time, value: null })
    previousInterval = interval
  }
  return points
}

function latestAt(points: ChartPoint[], time: number): ChartPoint | undefined {
  let low = 0
  let high = points.length
  while (low < high) {
    const mid = Math.floor((low + high) / 2)
    if (points[mid].time <= time) low = mid + 1
    else high = mid
  }
  return points[low - 1]
}

// Inputs belong to one simulation. Never add realized P&L or treat missing data as zero.
export function simulationUnrealizedPnL(positions: BacktestPosition[], trades: BacktestTrade[]): ChartPoint[] {
  const histories = positions.map((position) => ({ points: observations(position), intervals: openIntervals(position, trades) }))
  if (histories.some((history) => history.intervals === null)) return []
  const times = new Set<number>()
  for (const history of histories) {
    history.points.forEach((point) => times.add(point.time))
    history.intervals?.forEach((interval) => {
      times.add(interval.start)
      if (Number.isFinite(interval.end)) times.add(interval.end)
    })
  }
  return [...times].sort((a, b) => a - b).map((time) => {
    let value = 0
    let openPositions = 0
    for (const history of histories) {
      const interval = history.intervals?.find((open) => time >= open.start && time < open.end)
      if (!interval) continue
      openPositions++
      const point = latestAt(history.points, time)
      // Entry fills use bar opens; the first eligible completed snapshot is later.
      // A snapshot from an earlier opening must not mark a reopened position.
      if (!point || point.time <= interval.start || point.time >= interval.end) return { time, value: null }
      value += point.value!
    }
    return { time, value: openPositions > 0 ? value : null }
  })
}
