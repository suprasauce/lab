import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { getBacktest, type Backtest } from '../api/backtests'
import { ApiError } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import BacktestScript from '../components/BacktestScript'
import { formatExpiry, formatMoney, formatTimestamp } from '../format'

export default function BacktestDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [backtest, setBacktest] = useState<Backtest | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  const [tab, setTab] = useState<'results' | 'script'>('results')

  useEffect(() => { setTab('results') }, [id])

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    let pending = false
    setBacktest(null)
    setLoading(true)
    setError('')

    async function load() {
      if (!id) return
      try {
        const result = await getBacktest(id, controller.signal)
        if (controller.signal.aborted) return
        setBacktest(result)
        setError('')
        pending = result.status === 'PENDING'
        // Schedule after the response so slow requests never overlap.
        if (pending) timer = setTimeout(load, 3000)
      } catch (error) {
        if (controller.signal.aborted) return
        setError(error instanceof Error ? error.message : 'Unable to load this backtest.')
        // Keep checking a known pending job after transient failures, not 4xx errors.
        if (pending && (!(error instanceof ApiError) || error.status >= 500)) {
          timer = setTimeout(load, 3000)
        }
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }

    void load()
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [id, retry])

  return (
    <>
      <Link to="/backtests" className="back-link"><ArrowLeft size={15} aria-hidden="true" />My Backtests</Link>
      <header className="page-header">
        <div><h1>Backtest #{id}</h1><p className="page-subtitle">Execution status and strategy results.</p></div>
        {backtest && <StatusBadge status={backtest.status} />}
      </header>
      {error && <div className="error-message" role="alert"><span>{error}</span><button className="text-link" type="button" onClick={() => setRetry((value) => value + 1)}>Retry</button></div>}
      {loading && <p className="panel-message" role="status">Loading backtest...</p>}
      {!loading && backtest && (
        <div className="backtest-tabs" role="tablist" aria-label="Backtest details">
          {(['results', 'script'] as const).map((value) => (
            <button key={value} type="button" role="tab" id={`backtest-${value}-tab`}
              aria-selected={tab === value} aria-controls={`backtest-${value}-panel`}
              tabIndex={tab === value ? 0 : -1}
              onClick={() => setTab(value)}
              onKeyDown={(event) => {
                if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
                event.preventDefault()
                const next = event.key === 'Home' ? 'results' : event.key === 'End' ? 'script' : value === 'results' ? 'script' : 'results'
                setTab(next)
                document.getElementById(`backtest-${next}-tab`)?.focus()
              }}>
              {value === 'results' ? 'Results' : 'Script'}
            </button>
          ))}
        </div>
      )}
      {!loading && backtest && tab === 'script' && (
        <section role="tabpanel" id="backtest-script-panel" aria-labelledby="backtest-script-tab">
          <BacktestScript key={id} script={backtest.script} />
        </section>
      )}
      {!loading && backtest && tab === 'results' && (
        <section role="tabpanel" id="backtest-results-panel" aria-labelledby="backtest-results-tab">
          {!loading && backtest?.status === 'PENDING' && <div className="panel-message" role="status"><h2>Your backtest is being processed</h2><p>This page checks for updates every 3 seconds. You can safely navigate away.</p></div>}
          {!loading && backtest?.status === 'FAILED' && <div className="panel-message failure-panel"><h2>Backtest failed</h2><p>{backtest.error || 'The backtest could not be completed. No failure details were provided.'}</p></div>}
          {!loading && backtest?.status === 'COMPLETED' && (
            <>
              <dl className="summary-panel">
                <div><dt>Total P&amp;L</dt><dd className={backtest.summary.total_pnl < 0 ? 'negative-value' : 'positive-value'}>{formatMoney(backtest.summary.total_pnl)}</dd></div>
                <div><dt>Total trades</dt><dd>{backtest.summary.total_trades}</dd></div>
              </dl>
              <h2 className="section-title">Trades</h2>
              {backtest.trades.length === 0 ? <p className="panel-message">This backtest completed without any trades.</p> : (
                <div className="table-panel" tabIndex={0} role="region" aria-label="Backtest trades">
                  <table className="data-table trades-table">
                    <thead><tr>{['Expiry', 'Underlying', 'Strike', 'Option type', 'Side', 'Entry timestamp', 'Exit timestamp', 'Entry price', 'Exit price', 'Quantity', 'P&L'].map((label) => <th scope="col" key={label}>{label}</th>)}</tr></thead>
                    <tbody>{backtest.trades.map((trade, index) => (
                      <tr key={index}>
                        <td>{formatExpiry(trade.expiry)}</td><td>{trade.underlying}</td><td className="number-cell">{formatMoney(trade.strike)}</td>
                        <td>{trade.option_type}</td><td>{trade.side}</td><td>{formatTimestamp(trade.entry_timestamp)}</td><td>{formatTimestamp(trade.exit_timestamp)}</td>
                        <td className="number-cell">{formatMoney(trade.entry_price)}</td><td className="number-cell">{formatMoney(trade.exit_price)}</td><td className="number-cell">{trade.quantity}</td>
                        <td className={`number-cell ${trade.pnl === null ? '' : trade.pnl < 0 ? 'negative-value' : 'positive-value'}`}>{formatMoney(trade.pnl)}</td>
                      </tr>
                    ))}</tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </section>
      )}
    </>
  )
}
