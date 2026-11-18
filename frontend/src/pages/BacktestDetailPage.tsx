import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeft, BarChart3, FileCode2 } from 'lucide-react'
import { getBacktest, type Backtest } from '../api/backtests'
import { ApiError } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import BacktestScript from '../components/BacktestScript'
import BacktestResults from '../components/BacktestResults'
import { formatMoney } from '../format'

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
      {error && <div className="error-message" role="alert"><span>{error}</span><button className="secondary-button" type="button" onClick={() => setRetry((value) => value + 1)}>Retry</button></div>}
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
              {value === 'results' ? <BarChart3 size={17} aria-hidden="true" /> : <FileCode2 size={17} aria-hidden="true" />}
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
              <BacktestResults key={id} positions={backtest.positions} trades={backtest.trades} />
            </>
          )}
        </section>
      )}
    </>
  )
}
