import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { RefreshCw } from 'lucide-react'
import { listBacktests, type BacktestListItem } from '../api/backtests'
import StatusBadge from '../components/StatusBadge'
import { formatTimestamp } from '../format'

export default function BacktestsPage() {
  const [backtests, setBacktests] = useState<BacktestListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const navigate = useNavigate()

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    listBacktests(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setBacktests(result.backtests)
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setError(error instanceof Error ? error.message : 'Unable to load backtests.')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [refresh])

  return (
    <>
      <header className="page-header">
        <div><h1>My Backtests</h1><p className="page-subtitle">Your strategy runs, newest first.</p></div>
        <button className="secondary-button" type="button" onClick={() => setRefresh((value) => value + 1)} disabled={loading}>
          <RefreshCw size={15} className={loading ? 'loading-icon' : undefined} aria-hidden="true" />Refresh
        </button>
      </header>
      {error && <p className="error-message" role="alert">{error}</p>}
      {loading ? <p className="panel-message" role="status">Loading backtests...</p> : !error && (
        backtests.length === 0 ? <div className="panel-message">No backtests yet. Run a strategy from the <Link to="/editor" className="text-link">Editor</Link> to get started.</div> : (
          <div className="table-panel" tabIndex={0} role="region" aria-label="Your backtests">
            <table className="data-table backtests-table">
              <thead><tr><th scope="col">Backtest ID</th><th scope="col">Status</th><th scope="col">Created At</th></tr></thead>
              <tbody>{backtests.map((backtest) => (
                <tr key={backtest.id} onClick={() => navigate(`/backtests/${encodeURIComponent(backtest.id)}`)}>
                  <td><Link to={`/backtests/${encodeURIComponent(backtest.id)}`} className="backtest-link" onClick={(event) => event.stopPropagation()}>#{backtest.id}</Link></td>
                  <td><StatusBadge status={backtest.status} /></td>
                  <td>{formatTimestamp(backtest.created_at)}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )
      )}
    </>
  )
}
