import { useRef, useState } from 'react'
import { Layers, List, RotateCcw } from 'lucide-react'
import type { BacktestPosition, BacktestTrade } from '../api/backtests'
import { formatMoney, formatTimestamp } from '../format'

function pnlClass(value: number | null | undefined) {
  return value == null || value === 0 ? '' : value < 0 ? 'negative-value' : 'positive-value'
}

export default function BacktestResults({ positions = [], trades = [] }: {
  positions: BacktestPosition[]
  trades: BacktestTrade[]
}) {
  const [tab, setTab] = useState<'positions' | 'trades'>('positions')
  const [positionFilter, setPositionFilter] = useState<{ simulationId: string; symbol: string } | null>(null)
  const tradesTab = useRef<HTMLButtonElement>(null)
  const filteredTrades = trades.filter((trade) => positionFilter === null || (
    trade.simulation_id === positionFilter.simulationId && trade.tradingsymbol === positionFilter.symbol
  ))
  const hasUnrealizedPnL = positions.some((position) => position.unrealized_pnl != null)

  function showPositionTrades(position: BacktestPosition) {
    setPositionFilter({ simulationId: position.simulation_id, symbol: position.tradingsymbol })
    setTab('trades')
    tradesTab.current?.focus()
  }

  return (
    <>
      <div className="backtest-tabs" role="tablist" aria-label="Backtest results">
        {(['positions', 'trades'] as const).map((value) => (
          <button key={value} type="button" role="tab" id={`result-${value}-tab`}
            ref={value === 'trades' ? tradesTab : undefined}
            aria-selected={tab === value} aria-controls={`result-${value}-panel`}
            tabIndex={tab === value ? 0 : -1}
            onClick={() => setTab(value)}
            onKeyDown={(event) => {
              if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
              event.preventDefault()
              const next = event.key === 'Home' ? 'positions' : event.key === 'End' ? 'trades' : value === 'positions' ? 'trades' : 'positions'
              setTab(next)
              document.getElementById(`result-${next}-tab`)?.focus()
            }}>
            {value === 'positions' ? <Layers size={17} aria-hidden="true" /> : <List size={17} aria-hidden="true" />}
            {value === 'positions' ? 'Positions' : 'Trades'}
          </button>
        ))}
      </div>
      {tab === 'positions' && (
        <section role="tabpanel" id="result-positions-panel" aria-labelledby="result-positions-tab">
          {positions.length === 0 ? <p className="panel-message">No positions in this backtest.</p> : (
            <div className="table-panel" tabIndex={0} role="region" aria-label="Backtest positions">
              <table className="data-table positions-table">
                <thead><tr>
                  {['Simulation ID', 'Trading Symbol', 'Quantity', 'Average Price', 'Realized P&L', ...(hasUnrealizedPnL ? ['Unrealized P&L'] : []), 'Total P&L', 'Status'].map((label) => <th scope="col" key={label}>{label}</th>)}
                </tr></thead>
                <tbody>{positions.map((position) => {
                  // Use supplied P&L amounts, never reconstruct it from fills. Without
                  // unrealized P&L, only a closed position has a known total.
                  const totalPnL = position.total_pnl ?? (position.unrealized_pnl != null
                    ? position.realized_pnl + position.unrealized_pnl
                    : position.quantity === 0 ? position.realized_pnl : null)
                  return (
                    <tr key={`${position.simulation_id}/${position.tradingsymbol}`} onClick={() => showPositionTrades(position)}>
                      <td>{position.simulation_id}</td>
                      <td><button type="button" className="subtle-button" aria-label={`View trades for ${position.tradingsymbol}`}>{position.tradingsymbol}</button></td>
                      <td className="number-cell">{position.quantity}</td>
                      <td className="number-cell">{formatMoney(position.average_price)}</td>
                      <td className={`number-cell ${pnlClass(position.realized_pnl)}`}>{formatMoney(position.realized_pnl)}</td>
                      {hasUnrealizedPnL && <td className={`number-cell ${pnlClass(position.unrealized_pnl)}`}>{formatMoney(position.unrealized_pnl ?? null)}</td>}
                      <td className={`number-cell ${pnlClass(totalPnL)}`} title={totalPnL === null ? 'Unrealized P&L is unavailable.' : undefined}>{formatMoney(totalPnL)}</td>
                      <td>{position.quantity === 0 ? 'CLOSED' : 'OPEN'}</td>
                    </tr>
                  )
                })}</tbody>
              </table>
            </div>
          )}
        </section>
      )}
      {tab === 'trades' && (
        <section role="tabpanel" id="result-trades-panel" aria-labelledby="result-trades-tab">
          {positionFilter !== null && (
            <div className="results-toolbar" role="status">
              <span>Trades for <strong>{positionFilter.symbol}</strong> (simulation {positionFilter.simulationId})</span>
              <button type="button" className="secondary-button" onClick={() => setPositionFilter(null)}><RotateCcw size={15} aria-hidden="true" />View all trades</button>
            </div>
          )}
          {filteredTrades.length === 0 ? <p className="panel-message">{positionFilter === null ? 'No executed trades in this backtest.' : 'No executed trades for this position.'}</p> : (
            <div className="table-panel" tabIndex={0} role="region" aria-label="Executed trades">
              <table className="data-table trades-table">
                <thead><tr>{['Execution Date/Time', 'Trading Symbol', 'Quantity', 'Execution Price'].map((label) => <th scope="col" key={label}>{label}</th>)}</tr></thead>
                <tbody>{filteredTrades.map((trade) => (
                  <tr key={`${trade.simulation_id}/${trade.id}`}>
                    <td>{formatTimestamp(trade.timestamp)}</td><td>{trade.tradingsymbol}</td>
                    <td className="number-cell">{trade.quantity}</td>
                    <td className="number-cell">{formatMoney(trade.price)}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>
      )}
    </>
  )
}
