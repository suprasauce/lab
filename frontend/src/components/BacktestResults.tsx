import { useMemo } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import type { BacktestPosition, BacktestSummary, BacktestTrade } from '../api/backtests'
import { formatMoney, formatTimestamp } from '../format'
import { positionMTM, simulationMTM, type MTMSeries } from '../mtm'
import MTMChart from './MTMChart'

function pnlClass(value: number) {
  return value === 0 ? '' : value < 0 ? 'negative-value' : 'positive-value'
}

function simulationSearch(id: string, symbol?: string) {
  const params = new URLSearchParams({ simulation: id })
  if (symbol !== undefined) params.set('position', symbol)
  return `?${params}`
}

export default function BacktestResults({ summary, positions = [], trades = [] }: {
  summary: BacktestSummary
  positions: BacktestPosition[]
  trades: BacktestTrade[]
}) {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const simulationId = params.get('simulation')
  const symbol = params.get('position')
  const simulations = useMemo(() => {
    const ids = [...new Set([...positions, ...trades].map((item) => item.simulation_id))]
    return ids.map((id, index) => {
      const items = positions.filter((position) => position.simulation_id === id)
      const fills = trades.filter((trade) => trade.simulation_id === id)
      return {
        id, positions: items, trades: fills,
        realizedPnL: items.reduce((total, position) => total + position.realized_pnl, 0),
        label: `Simulation ${id}`, color: `hsl(${(260 + index * 137.508) % 360} 65% 68%)`,
        points: simulationMTM(items, fills),
      }
    })
  }, [positions, trades])
  const selected = simulations.find((simulation) => simulation.id === simulationId)
  const position = selected?.positions.find((item) => item.tradingsymbol === symbol)
  const positionTrades = useMemo(() => trades.filter((trade) => trade.simulation_id === simulationId && trade.tradingsymbol === symbol)
    .sort((a, b) => Date.parse(a.timestamp) - Date.parse(b.timestamp)), [trades, simulationId, symbol])
  const positionSeries = useMemo<MTMSeries[]>(() => position && selected ? [{
    id: `${selected.id}/${position.tradingsymbol}`, label: 'Position MTM', color: selected.color,
    points: positionMTM(position, positionTrades),
  }] : [], [position, selected, positionTrades])

  return (
    <>
      <nav className="result-breadcrumbs" aria-label="Backtest results navigation">
        <Link to={{ search: '' }} aria-current={simulationId === null ? 'page' : undefined}>Overview</Link>
        {simulationId !== null && <>
          <ChevronRight size={14} aria-hidden="true" />
          <Link to={{ search: simulationSearch(simulationId) }} aria-current={symbol === null ? 'page' : undefined}>Simulation {simulationId}</Link>
        </>}
        {simulationId !== null && symbol !== null && <><ChevronRight size={14} aria-hidden="true" /><span aria-current="page">{symbol}</span></>}
      </nav>

      {simulationId === null ? (
        <>
          <dl className="summary-panel">
            <div><dt>Total P&amp;L</dt><dd className={pnlClass(summary.total_pnl)}>{formatMoney(summary.total_pnl)}</dd></div>
            <div><dt>Total trades</dt><dd>{summary.total_trades}</dd></div>
          </dl>
          <MTMChart title="Backtest MTM" series={simulations} />
          <h2 className="section-title">Simulations</h2>
          {simulations.length === 0 ? <p className="panel-message">No simulation positions or trades were recorded.</p> : (
            <div className="simulation-grid">
              {simulations.map((simulation) => (
                <Link key={simulation.id} className="simulation-card" to={{ search: simulationSearch(simulation.id) }}>
                  <span className="simulation-card-heading">Simulation {simulation.id}<ChevronRight size={16} aria-hidden="true" /></span>
                  <span className={`simulation-card-pnl ${pnlClass(simulation.realizedPnL)}`}>{formatMoney(simulation.realizedPnL)}</span>
                  <span className="simulation-card-caption">Realized P&amp;L <span>{simulation.positions.length} {simulation.positions.length === 1 ? 'position' : 'positions'}</span></span>
                </Link>
              ))}
            </div>
          )}
        </>
      ) : !selected ? (
        <p className="panel-message">This simulation is not available. Return to the overview to select a simulation.</p>
      ) : symbol === null ? (
        <>
          <h2 className="result-detail-title">Simulation {selected.id}</h2>
          <dl className="summary-panel">
            <div><dt>Realized P&amp;L</dt><dd className={pnlClass(selected.realizedPnL)}>{formatMoney(selected.realizedPnL)}</dd></div>
          </dl>
          <h2 className="section-title">Positions</h2>
          {selected.positions.length === 0 ? <p className="panel-message">No positions in this simulation.</p> : (
            <div className="table-panel" tabIndex={0} role="region" aria-label={`Positions in simulation ${selected.id}`}>
              <table className="data-table positions-table">
                <thead><tr>{['Trading Symbol', 'Quantity (lots)', 'Average Price', 'Realized P&L', 'Status'].map((label) => <th scope="col" key={label}>{label}</th>)}</tr></thead>
                <tbody>{selected.positions.map((item) => (
                  <tr key={`${item.simulation_id}/${item.tradingsymbol}`} onClick={() => navigate({ search: simulationSearch(item.simulation_id, item.tradingsymbol) })}>
                    <td><button type="button" className="subtle-button" aria-label={`View position ${item.tradingsymbol}`}>{item.tradingsymbol}</button></td>
                    <td className="number-cell">{item.quantity}</td><td className="number-cell">{formatMoney(item.average_price)}</td>
                    <td className={`number-cell ${pnlClass(item.realized_pnl)}`}>{formatMoney(item.realized_pnl)}</td>
                    <td>{item.quantity === 0 ? 'CLOSED' : 'OPEN'}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </>
      ) : !position ? (
        <p className="panel-message">This position is not available in simulation {selected.id}.</p>
      ) : (
        <>
          <h2 className="result-detail-title">{position.tradingsymbol}</h2>
          <dl className="summary-panel position-summary">
            <div><dt>Quantity (lots)</dt><dd>{position.quantity}</dd></div>
            <div><dt>Average Price</dt><dd>{formatMoney(position.average_price)}</dd></div>
            <div><dt>Realized P&amp;L</dt><dd className={pnlClass(position.realized_pnl)}>{formatMoney(position.realized_pnl)}</dd></div>
            <div><dt>Status</dt><dd>{position.quantity === 0 ? 'CLOSED' : 'OPEN'}</dd></div>
          </dl>
          <MTMChart key={`${position.simulation_id}/${position.tradingsymbol}`} title="Position MTM" series={positionSeries} />
          <h2 className="section-title">Trades</h2>
          {positionTrades.length === 0 ? <p className="panel-message">No executed trades for this position.</p> : (
            <div className="table-panel" tabIndex={0} role="region" aria-label={`Trades for ${position.tradingsymbol}`}>
              <table className="data-table trades-table">
                <thead><tr>{['Execution Date/Time', 'Side', 'Quantity (lots)', 'Execution Price'].map((label) => <th scope="col" key={label}>{label}</th>)}</tr></thead>
                <tbody>{positionTrades.map((trade) => (
                  <tr key={`${trade.simulation_id}/${trade.id}`}>
                    <td>{formatTimestamp(trade.timestamp)}</td><td>{trade.quantity > 0 ? 'BUY' : 'SELL'}</td>
                    <td className="number-cell">{Math.abs(trade.quantity)}</td><td className="number-cell">{formatMoney(trade.price)}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </>
      )}
    </>
  )
}
