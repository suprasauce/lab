import { useEffect, useRef, useState } from 'react'
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import Editor, { type OnMount } from '@monaco-editor/react'
import { configureTheme } from '../editor'
import { FileCode2, LoaderCircle, Play } from 'lucide-react'
import { createBacktest } from '../api/backtests'
import { createStrategy, listStrategies, updateStrategy } from '../api/strategies'
import type { CreateStrategyRequest } from '../types/strategy'

export const starterStrategy = `def initialize(ctx):
    ctx.configure(
        underlying="NIFTY",
    )
    for expiry in ctx.get_expiries(start="2026-01-01", end="2026-06-30"):
        ctx.simulate(expiry)


def on_candle(ctx):
    # Sell one call at 45 DTE.
    if ctx.dte <= 45 and ctx.time >= "09:30":
        if "entry_order_id" not in ctx.state:
            ctx.state["entry_order_id"] = ctx.place_order(
                contract=offset_selector("CALL", offset=2),
                side="SELL",
                quantity=75,
            )

    # Close the position at 21 DTE.
    if ctx.dte <= 21 and ctx.time >= "15:15":
        if "entry_order_id" in ctx.state and "exit_order_id" not in ctx.state:
            entry = ctx.order(ctx.state["entry_order_id"])
            if entry.status == "FILLED":
                ctx.state["exit_order_id"] = ctx.place_order(
                    contract=id_selector(entry.contract_id),
                    side="BUY",
                    quantity=75,
                )
`

export default function EditorPage() {
  const location = useLocation()
  return <StrategyEditor key={location.key} />
}

function StrategyEditor() {
  const [params] = useSearchParams()
  const location = useLocation()
  const requestedId = params.get('strategyId')
  const state = location.state as { template?: CreateStrategyRequest; saved?: CreateStrategyRequest } | null
  const initial = requestedId !== null ? state?.saved : state?.template
  const [script, onChange] = useState(initial?.script ?? (requestedId !== null ? '' : starterStrategy))
  const [name, setName] = useState(initial?.name ?? '')
  const [description, setDescription] = useState(initial?.description ?? '')
  const [strategyId, setStrategyId] = useState(requestedId)
  const [loading, setLoading] = useState(requestedId !== null)
  const [loadError, setLoadError] = useState('')
  const [saving, setSaving] = useState(false)
  const savePending = useRef(false)
  const active = useRef(true)
  const [success, setSuccess] = useState(state?.saved ? 'Strategy saved.' : '')

  useEffect(() => {
    active.current = true
    const controller = new AbortController()
    if (requestedId !== null) {
      listStrategies(controller.signal).then(({ strategies }) => {
        if (controller.signal.aborted) return
        const strategy = strategies.find((item) => item.id === requestedId)
        if (!strategy) throw new Error('Strategy not found for the current user.')
        setName(strategy.name)
        setDescription(strategy.description ?? '')
        onChange(strategy.script)
      }).catch((error: unknown) => {
        if (!controller.signal.aborted) setLoadError(error instanceof Error ? error.message : 'Unable to load strategy.')
      }).finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    }
    return () => { active.current = false; controller.abort() }
  }, [requestedId])

  async function saveStrategy() {
    if (savePending.current || loading || loadError) return
    setSuccess('')
    if (!name.trim() || !script.trim()) {
      setError('Strategy name and script are required.')
      return
    }
    savePending.current = true
    setSaving(true)
    setError('')
    const draft = { name: name.trim(), description, script }
    try {
      const result = strategyId !== null
        ? await updateStrategy(strategyId, draft)
        : await createStrategy(draft)
      if (!active.current) return
      // Remember the committed ID before navigation so another save cannot create a duplicate.
      setStrategyId(result.id)
      setSuccess('Strategy saved.')
      if (strategyId === null) navigate(`/editor?strategyId=${encodeURIComponent(result.id)}`, { replace: true, state: { saved: draft } })
    } catch (error) {
      if (active.current) setError(error instanceof Error ? error.message : 'Unable to save strategy.')
    } finally {
      savePending.current = false
      if (active.current) setSaving(false)
    }
  }

  const [position, setPosition] = useState({ lineNumber: 1, column: 1 })
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const submission = useRef<AbortController | null>(null)
  const navigate = useNavigate()

  useEffect(() => () => submission.current?.abort(), [])
  const handleMount: OnMount = (editor) => {
    editor.onDidChangeCursorPosition((event) => setPosition(event.position))
  }

  async function runBacktest() {
    if (submission.current) return
    const controller = new AbortController()
    submission.current = controller
    setSubmitting(true)
    setError('')
    try {
      const result = await createBacktest(script, controller.signal)
      if (!controller.signal.aborted) navigate(`/backtests/${encodeURIComponent(result.id)}`)
    } catch (error) {
      if (!controller.signal.aborted) setError(error instanceof Error ? error.message : 'Unable to submit the backtest.')
    } finally {
      submission.current = null
      if (!controller.signal.aborted) setSubmitting(false)
    }
  }

  return (
    <>
      <header className="page-header">
        <div><h1>Strategy Editor</h1><p className="page-subtitle">Turn your trading ideas into strategies.</p></div>
        <div className="header-actions">
          <label className="strategy-name">Strategy name<input value={name} onChange={(event) => { setName(event.target.value); setSuccess('') }} disabled={loading || saving || !!loadError} placeholder="Untitled strategy" /></label>
          <button type="button" className="secondary-button" onClick={saveStrategy} disabled={saving || loading || !!loadError} aria-busy={saving}>{saving ? 'Saving...' : strategyId !== null ? 'Save Changes' : 'Save Strategy'}</button>
        <button type="button" className="run-button" onClick={runBacktest} disabled={submitting || loading || !!loadError} aria-busy={submitting}>
          {submitting ? <LoaderCircle className="loading-icon" size={16} aria-hidden="true" /> : <Play size={16} aria-hidden="true" />}
          {submitting ? 'Submitting...' : 'Run Backtest'}
        </button>
        </div>
      </header>
      {loading && <p className="panel-message" role="status">Loading strategy...</p>}
      {loadError && <p className="error-message" role="alert">{loadError}</p>}
      {success && <p className="save-success" role="status">{success}</p>}
      {error && <p className="error-message" role="alert">{error}</p>}
      <label className="strategy-description">
        Description (optional)
        <textarea
          value={description}
          onChange={(event) => { setDescription(event.target.value); setSuccess('') }}
          disabled={loading || saving || !!loadError}
          rows={2}
          placeholder="Briefly describe what this strategy does..."
        />
      </label>
      <section className="editor-panel" aria-label="Strategy script editor">
        <div className="editor-toolbar"><div className="editor-tab"><FileCode2 size={16} aria-hidden="true" /><span>strategy.star</span><span className="tab-dot" aria-hidden="true" /></div><span className="language-badge">STARLARK</span></div>
        <div className="editor-surface">
          <Editor height="100%" language="python" path="strategy.star" theme="lab-dark" value={script}
            beforeMount={configureTheme} onMount={handleMount} onChange={(value) => { onChange(value ?? ''); setSuccess('') }}
            loading={<span className="editor-loading">Loading editor…</span>}
            options={{ readOnly: loading || saving || !!loadError, automaticLayout: true, ariaLabel: 'Strategy code', fontSize: 13, lineHeight: 23, fontFamily: "'Cascadia Code', 'SFMono-Regular', Consolas, monospace", minimap: { enabled: false }, scrollBeyondLastLine: false, padding: { top: 22, bottom: 22 }, lineNumbersMinChars: 3, folding: false, overviewRulerLanes: 0, hideCursorInOverviewRuler: true, renderLineHighlight: 'line', tabSize: 4, wordWrap: 'on', scrollbar: { verticalScrollbarSize: 7, horizontalScrollbarSize: 7 } }}
          />
        </div>
        <footer className="editor-status"><span><span className="status-dot" />Ready to edit</span><div className="flex items-center gap-5"><span>Python / Starlark</span><span>Ln {position.lineNumber}, Col {position.column}</span><span className="encoding-label">UTF-8</span></div></footer>
      </section>
    </>
  )
}
