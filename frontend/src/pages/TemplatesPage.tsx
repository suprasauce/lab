import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { listTemplates } from '../api/strategies'
import type { Strategy } from '../types/strategy'

export default function TemplatesPage() {
  const [templates, setTemplates] = useState<Strategy[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const navigate = useNavigate()
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    listTemplates(controller.signal).then((result) => {
      if (!controller.signal.aborted) setTemplates(result.templates)
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setError(error instanceof Error ? error.message : 'Unable to load templates.')
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [refresh])
  return <>
    <header className="page-header"><div><h1>Templates</h1><p className="page-subtitle">Start a new strategy from a platform template.</p></div>
      <button type="button" className="secondary-button" disabled={loading} onClick={() => setRefresh((value) => value + 1)}>Refresh</button>
    </header>
    {error && <p className="error-message" role="alert">{error}</p>}
    {loading ? <p className="panel-message" role="status">Loading templates...</p> : !error && (
      templates.length === 0 ? <p className="panel-message">No templates are available yet.</p> :
      <div className="template-grid">{templates.map((template) => <article className="template-card" key={template.id}>
        <h2>{template.name}</h2>
        <button className="secondary-button" type="button" onClick={() => navigate('/editor', { state: { template: { name: template.name, script: template.script } } })}>Use Template</button>
      </article>)}</div>
    )}
  </>
}
