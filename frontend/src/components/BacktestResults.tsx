import { useState } from 'react'
import type { BacktestPosition, BacktestSummary, BacktestTrade } from '../api/backtests'
import { formatMoney, formatTimestamp } from '../format'

function pnlClass(value: number) {
  return value === 0 ? '' : value < 0 ? 'negative-value' : 'positive-value'
}

export default function BacktestResults({ summary, positions = [], trades = [] }: {
  summary: BacktestSummary
  positions: BacktestPosition[]
  trades: BacktestTrade[]
}) {
  const [tab, setTab] = useState<'positions' | 'trades'>('positions')

  return (
    <>
      <dl className="summary-panel">
        <div><dt>Total P&amp;L</dt><dd className={pnlClass(summary.total_pnl)}>{formatMoney(summary.total_pnl)}</dd></div>
        <div><dt>Total trades</dt><dd>{summary.total_trades}</dd></div>
      </dl>
      <div className="backtest-tabs" role="tablist" aria-label="Result tables">
        {(['positions', 'trades'] as const).map((value) => (
          <button key={value} type="button" role="tab" id={`result-${value}-tab`}
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
            {value === 'positions' ? `Positions (${positions.length})` : `Trades (${trades.length})`}
          </button>
        ))}
      </div>
      {tab === 'positions' ? (
        <section role="tabpanel" id="result-positions-panel" aria-labelledby="result-positions-tab">
          {positions.length === 0 ? <p className="panel-message">No positions were recorded.</p> : (
            <div className="table-panel" tabIndex={0} role="region" aria-label="Positions across all simulations">
              <table className="data-table positions-table">
                <thead><tr>{['Simulation ID', 'Trading Symbol', 'Quantity (lots)', 'Average Price', 'Realized P&L', 'Status'].map((label) => <th scope="col" key={label}>{label}</th>)}</tr></thead>
                <tbody>{positions.map((position) => (
                  <tr key={`${position.simulation_id}/${position.tradingsymbol}`}>
                    <td>{position.simulation_id}</td><td>{position.tradingsymbol}</td>
                    <td className="number-cell">{position.quantity}</td>
                    <td className="number-cell">{formatMoney(position.average_price)}</td>
                    <td className={`number-cell ${pnlClass(position.realized_pnl)}`}>{formatMoney(position.realized_pnl)}</td>
                    <td>{position.quantity === 0 ? 'CLOSED' : 'OPEN'}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>
      ) : (
        <section role="tabpanel" id="result-trades-panel" aria-labelledby="result-trades-tab">
          {trades.length === 0 ? <p className="panel-message">No trades were recorded.</p> : (
            <div className="table-panel" tabIndex={0} role="region" aria-label="Trades across all simulations">
              <table className="data-table trades-table">
                <thead><tr>{['Trade ID', 'Simulation ID', 'Trading Symbol', 'Execution Date/Time', 'Quantity (lots)', 'Execution Price'].map((label) => <th scope="col" key={label}>{label}</th>)}</tr></thead>
                <tbody>{trades.map((trade) => (
                  <tr key={`${trade.simulation_id}/${trade.id}`}>
                    <td>{trade.id}</td><td>{trade.simulation_id}</td><td>{trade.tradingsymbol}</td>
                    <td>{formatTimestamp(trade.timestamp)}</td>
                    <td className="number-cell">{trade.quantity}</td><td className="number-cell">{formatMoney(trade.price)}</td>
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
