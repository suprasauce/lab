import type { BacktestPosition, BacktestTrade } from './api/backtests'

export interface ChartPoint {
  time: number
  value: number | null
}

export interface TotalPnLSeries {
  id: string
  label: string
  color: string
  points: ChartPoint[]
}

function observations(position: BacktestPosition): ChartPoint[] {
  return (position.total_pnl ?? [])
    .map((point) => ({ time: Date.parse(point.timestamp), value: point.pnl }))
    .filter((point) => Number.isFinite(point.time) && Number.isFinite(point.value))
    .sort((a, b) => a.time - b.time)
}

function withGaps(points: ChartPoint[]): ChartPoint[] {
  return points.flatMap((point, index) => index > 0 && point.time - points[index - 1].time > 5 * 60_000
    ? [{ time: points[index - 1].time + 5 * 60_000, value: null }, point]
    : [point])
}

export function positionTotalPnL(position: BacktestPosition): ChartPoint[] {
  return withGaps(observations(position))
}

// Sum matching snapshots only. Positions contribute zero before their first entry;
// after entry, missing snapshots make the simulation total unavailable.
export function simulationTotalPnL(positions: BacktestPosition[], trades: BacktestTrade[]): ChartPoint[] {
  const histories = positions.map(observations)
  if (histories.some((history) => history.length === 0)) return []
  const starts = positions.map((position, index) => {
    const fills = trades.filter((trade) => trade.simulation_id === position.simulation_id && trade.tradingsymbol === position.tradingsymbol)
    return fills.length > 0 ? Math.min(...fills.map((trade) => Date.parse(trade.timestamp))) + 5 * 60_000 : histories[index][0].time
  })
  const lookups = histories.map((history) => new Map(history.map((point) => [point.time, point.value])))
  const times = [...new Set(histories.flatMap((history) => history.map((point) => point.time)))].sort((a, b) => a - b)
  return withGaps(times.map((time) => {
    let value = 0
    for (let i = 0; i < histories.length; i++) {
      if (time < starts[i]) continue
      const current = lookups[i].get(time)
      if (current == null) return { time, value: null }
      value += current
    }
    return { time, value }
  }))
}
