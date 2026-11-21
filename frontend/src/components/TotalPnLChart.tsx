import { useId, useMemo, useState } from 'react'
import { formatMoney } from '../format'
import type { TotalPnLSeries } from '../total_pnl'

const axisDate = new Intl.DateTimeFormat('en-IN', { timeZone: 'Asia/Kolkata', day: '2-digit', month: 'short', year: '2-digit' })
const axisTime = new Intl.DateTimeFormat('en-IN', { timeZone: 'Asia/Kolkata', day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
const exactDate = new Intl.DateTimeFormat('en-IN', { timeZone: 'Asia/Kolkata', dateStyle: 'medium', timeStyle: 'medium' })
const axisMoney = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', notation: 'compact', maximumFractionDigits: 1 })
const plot = { width: 1000, height: 360, left: 88, right: 24, top: 28, bottom: 62 }

export default function TotalPnLChart({ title, series }: { title: string; series: TotalPnLSeries[] }) {
  const [hidden, setHidden] = useState<Set<string>>(() => new Set())
  const [hoverTime, setHoverTime] = useState<number | null>(null)
  const clipId = useId()
  const visible = useMemo(() => series.filter((line) => !hidden.has(line.id)), [series, hidden])
  const data = useMemo(() => {
    let minTime = Infinity, maxTime = -Infinity, minValue = 0, maxValue = 0
    const values = new Map<number, { id: string; label: string; color: string; value: number }[]>()
    for (const line of visible) {
      for (const point of line.points) {
        minTime = Math.min(minTime, point.time)
        maxTime = Math.max(maxTime, point.time)
        if (point.value === null) continue
        minValue = Math.min(minValue, point.value)
        maxValue = Math.max(maxValue, point.value)
        const rows = values.get(point.time) ?? []
        rows.push({ id: line.id, label: line.label, color: line.color, value: point.value })
        values.set(point.time, rows)
      }
    }
    if (minTime === maxTime) { minTime -= 5 * 60_000; maxTime += 5 * 60_000 }
    const padding = (maxValue - minValue || 1) * 0.08
    return { minTime, maxTime, minValue: minValue - padding, maxValue: maxValue + padding, values, times: [...values.keys()].sort((a, b) => a - b) }
  }, [visible])
  const width = plot.width - plot.left - plot.right
  const height = plot.height - plot.top - plot.bottom
  const x = (time: number) => plot.left + (time - data.minTime) / (data.maxTime - data.minTime) * width
  const y = (value: number) => plot.top + (data.maxValue - value) / (data.maxValue - data.minValue) * height
  const hoverRows = hoverTime === null ? [] : data.values.get(hoverTime) ?? []
  const tooltipX = hoverTime === null ? 0 : Math.max(plot.left, Math.min(x(hoverTime) + 12, plot.width - 300))

  function nearestTime(target: number) {
    let low = 0, high = data.times.length
    while (low < high) {
      const mid = Math.floor((low + high) / 2)
      if (data.times[mid] < target) low = mid + 1
      else high = mid
    }
    const before = data.times[Math.max(0, low - 1)]
    const after = data.times[Math.min(low, data.times.length - 1)]
    return Math.abs(target - before) <= Math.abs(after - target) ? before : after
  }

  return (
    <section className="total_pnl-chart" aria-label={title}>
      <div className="total_pnl-chart-header"><h2 className="section-title">{title}</h2><span>Total P&amp;L · INR · IST</span></div>
      {series.length > 1 && (
        <div className="total_pnl-legend" aria-label="Toggle simulation lines">
          {series.map((line) => (
            <button key={line.id} type="button" className="subtle-button" aria-pressed={!hidden.has(line.id)} onClick={() => {
              setHidden((previous) => {
                const next = new Set(previous)
                if (next.has(line.id)) next.delete(line.id)
                else next.add(line.id)
                return next
              })
              setHoverTime(null)
            }}>
              <span className="total_pnl-swatch" style={{ background: line.color }} />{line.label}
            </button>
          ))}
        </div>
      )}
      {data.times.length === 0 ? (
        <p className="total_pnl-empty">{visible.length === 0 && series.length > 0 ? 'Select a simulation in the legend to show its line.' : 'No reliable Total P&L history is available for this view.'}</p>
      ) : (
        <div className="total_pnl-chart-scroll">
          <svg className="total_pnl-chart-svg" viewBox={`0 0 ${plot.width} ${plot.height}`} role="img" aria-label={`${title}. Use left and right arrow keys to inspect snapshots.`} tabIndex={0}
            onPointerMove={(event) => {
              const bounds = event.currentTarget.getBoundingClientRect()
              const coordinate = (event.clientX - bounds.left) / bounds.width * plot.width
              if (coordinate < plot.left || coordinate > plot.width - plot.right) { setHoverTime(null); return }
              setHoverTime(nearestTime(data.minTime + (coordinate - plot.left) / width * (data.maxTime - data.minTime)))
            }} onPointerLeave={() => setHoverTime(null)} onBlur={() => setHoverTime(null)}
            onKeyDown={(event) => {
              if (event.key === 'Escape') { setHoverTime(null); return }
              if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
              event.preventDefault()
              const index = hoverTime === null ? -1 : data.times.indexOf(hoverTime)
              const next = event.key === 'Home' ? 0 : event.key === 'End' ? data.times.length - 1 : event.key === 'ArrowRight' ? index + 1 : index === -1 ? data.times.length - 1 : index - 1
              setHoverTime(data.times[Math.max(0, Math.min(data.times.length - 1, next))])
            }}>
            <title>{title}: timestamped total P&amp;L, including realized and unrealized P&amp;L</title>
            <defs><clipPath id={clipId}><rect x={plot.left} y={plot.top} width={width} height={height} /></clipPath></defs>
            {[0, 1, 2, 3, 4].map((tick) => {
              const value = data.minValue + (data.maxValue - data.minValue) * tick / 4
              return <g key={tick}><line className="total_pnl-grid-line" x1={plot.left} x2={plot.width - plot.right} y1={y(value)} y2={y(value)} /><text className="total_pnl-axis-label" x={plot.left - 12} y={y(value) + 4} textAnchor="end">{axisMoney.format(value)}</text></g>
            })}
            <line className="total_pnl-zero-line" x1={plot.left} x2={plot.width - plot.right} y1={y(0)} y2={y(0)} />
            {[0, 1, 2, 3, 4].map((tick) => {
              const time = data.minTime + (data.maxTime - data.minTime) * tick / 4
              return <text key={tick} className="total_pnl-axis-label" x={x(time)} y={plot.height - 30} textAnchor={tick === 0 ? 'start' : tick === 4 ? 'end' : 'middle'}>{(data.maxTime - data.minTime < 2 * 86_400_000 ? axisTime : axisDate).format(time)}</text>
            })}
            <text className="total_pnl-axis-label" x={plot.left} y={plot.height - 8}>Date / time (IST)</text>
            <g clipPath={`url(#${clipId})`}>
              {visible.map((line) => {
                const path = line.points.filter((point) => point.value !== null).map((point, index) =>
                  `${index === 0 ? 'M' : 'L'}${x(point.time)},${y(point.value!)}`,
                ).join(' ')
                return <path key={line.id} d={path} fill="none" stroke={line.color} strokeWidth={1.6} vectorEffect="non-scaling-stroke" />
              })}
            </g>
            {hoverTime !== null && hoverRows.length > 0 && (
              <g pointerEvents="none">
                <line className="total_pnl-crosshair" x1={x(hoverTime)} x2={x(hoverTime)} y1={plot.top} y2={plot.top + height} />
                {hoverRows.map((row) => <circle key={row.id} cx={x(hoverTime)} cy={y(row.value)} r={3.5} fill={row.color} />)}
                <rect className="total_pnl-tooltip" x={tooltipX} y={plot.top + 8} width={282} height={38 + hoverRows.length * 22} rx={8} />
                <text className="total_pnl-tooltip-date" x={tooltipX + 12} y={plot.top + 30}>{exactDate.format(hoverTime)} IST</text>
                {hoverRows.map((row, index) => <g key={row.id}><circle cx={tooltipX + 15} cy={plot.top + 49 + index * 22} r={3} fill={row.color} /><text className="total_pnl-tooltip-value" x={tooltipX + 26} y={plot.top + 53 + index * 22}>{series.length === 1 ? 'Total P&L' : row.label}</text><text className="total_pnl-tooltip-value" textAnchor="end" x={tooltipX + 270} y={plot.top + 53 + index * 22}>{formatMoney(row.value)}</text></g>)}
              </g>
            )}
          </svg>
        </div>
      )}
      <p className="total_pnl-chart-note">Total P&L includes cumulative realized and current unrealized P&L. Lines connect available snapshots across missing data; no additional values are recorded.</p>
    </section>
  )
}
