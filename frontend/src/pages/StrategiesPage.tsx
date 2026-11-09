import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { RefreshCw } from 'lucide-react'
import { listStrategies } from '../api/strategies'
import type { Strategy } from '../types/strategy'
import { formatTimestamp } from '../format'

export default function StrategiesPage() {
  const [strategies, setStrategies] = useState<Strategy[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const navigate = useNavigate()

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    listStrategies(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setStrategies(result.strategies)
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setError(error instanceof Error ? error.message : 'Unable to load strategies.')
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [refresh])

  return (
    <>
      <header className="page-header">
        <div><h1>My Strategies</h1><p className="page-subtitle">Your saved strategies, most recently updated first.</p></div>
        <div className="header-actions"><Link to="/editor" className="run-button">New Strategy</Link><button className="secondary-button" type="button" onClick={() => setRefresh((value) => value + 1)} disabled={loading}>
          <RefreshCw size={15} className={loading ? 'loading-icon' : undefined} aria-hidden="true" />Refresh
        </button></div>
      </header>
      {error && <p className="error-message" role="alert">{error}</p>}
      {loading ? <p className="panel-message" role="status">Loading strategies...</p> : !error && (
        strategies.length === 0 ? <div className="panel-message">No strategies yet. Create a strategy in the <Link to="/editor" className="text-link">Editor</Link> to get started.</div> : (
          <div className="table-panel" tabIndex={0} role="region" aria-label="Your strategies">
            <table className="data-table backtests-table">
              <thead><tr><th scope="col">Strategy Name</th><th scope="col">Last Updated</th><th scope="col">Action</th></tr></thead>
              <tbody>{strategies.map((strategy) => (
                <tr key={strategy.id} onClick={() => navigate(`/editor?strategyId=${encodeURIComponent(strategy.id)}`)}>
                  <td className="strategy-info">
                    {strategy.name}
                    {strategy.description && <p className="strategy-description-preview" title={strategy.description}>{strategy.description}</p>}
                  </td>
                  <td>{formatTimestamp(strategy.updated_at)}</td>
                  <td><Link to={`/editor?strategyId=${encodeURIComponent(strategy.id)}`} className="text-link" onClick={(event) => event.stopPropagation()}>Open in Editor</Link></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )
      )}
    </>
  )
}
