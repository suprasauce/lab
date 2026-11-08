import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import Editor, { loader, type BeforeMount, type OnMount } from '@monaco-editor/react'
import * as monaco from 'monaco-editor/editor/editor.api'
import 'monaco-editor/languages/definitions/python/register.js'
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'
import { FileCode2, LoaderCircle, Play } from 'lucide-react'
import { createBacktest } from '../api/backtests'

self.MonacoEnvironment = { getWorker: () => new EditorWorker() }
loader.config({ monaco })

export const starterStrategy = `def initialize(ctx):
    ctx.configure(
        underlying="NIFTY",
        start_date="2026-01-01",
        end_date="2026-06-30",
    )


def on_candle(ctx):
    # Sell one call at 45 DTE.
    if ctx.dte <= 45 and ctx.time >= "09:30":
        if "entry_order_id" not in ctx.state:
            ctx.state["entry_order_id"] = ctx.place_order(
                contract=atm_option("CALL", offset=2),
                side="SELL",
                quantity=75,
            )

    # Close the position at 21 DTE.
    if ctx.dte <= 21 and ctx.time >= "15:15":
        if "entry_order_id" in ctx.state and "exit_order_id" not in ctx.state:
            entry = ctx.order(ctx.state["entry_order_id"])
            if entry.status == "FILLED":
                ctx.state["exit_order_id"] = ctx.place_order(
                    contract=entry.contract_id,
                    side="BUY",
                    quantity=75,
                )
`

const configureTheme: BeforeMount = (editor) => {
  editor.editor.defineTheme('lab-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: 'comment', foreground: '788092' },
      { token: 'keyword', foreground: 'BCA0F9' },
      { token: 'string', foreground: 'D8AC86' },
      { token: 'number', foreground: '9ACDB3' },
    ],
    colors: {
      'editor.background': '#12151D',
      'editor.foreground': '#D7DCE6',
      'editorLineNumber.foreground': '#596174',
      'editorLineNumber.activeForeground': '#BCC3D3',
      'editor.lineHighlightBackground': '#191D28',
      'editor.selectionBackground': '#7660CB44',
      'editorCursor.foreground': '#B6A0FF',
      'editorIndentGuide.background1': '#222735',
      'editorWidget.background': '#1C202B',
      'editorWidget.border': '#303646',
      'focusBorder': '#8267DD',
    },
  })
}

export default function EditorPage({ script, onChange }: { script: string; onChange: (value: string) => void }) {
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
        <button type="button" className="run-button" onClick={runBacktest} disabled={submitting} aria-busy={submitting}>
          {submitting ? <LoaderCircle className="loading-icon" size={16} aria-hidden="true" /> : <Play size={16} aria-hidden="true" />}
          {submitting ? 'Submitting...' : 'Run Backtest'}
        </button>
      </header>
      {error && <p className="error-message" role="alert">{error}</p>}
      <section className="editor-panel" aria-label="Strategy script editor">
        <div className="editor-toolbar"><div className="editor-tab"><FileCode2 size={16} aria-hidden="true" /><span>strategy.star</span><span className="tab-dot" aria-hidden="true" /></div><span className="language-badge">STARLARK</span></div>
        <div className="editor-surface">
          <Editor height="100%" language="python" path="strategy.star" theme="lab-dark" value={script}
            beforeMount={configureTheme} onMount={handleMount} onChange={(value) => onChange(value ?? '')}
            loading={<span className="editor-loading">Loading editor…</span>}
            options={{ automaticLayout: true, ariaLabel: 'Strategy code', fontSize: 13, lineHeight: 23, fontFamily: "'Cascadia Code', 'SFMono-Regular', Consolas, monospace", minimap: { enabled: false }, scrollBeyondLastLine: false, padding: { top: 22, bottom: 22 }, lineNumbersMinChars: 3, folding: false, overviewRulerLanes: 0, hideCursorInOverviewRuler: true, renderLineHighlight: 'line', tabSize: 4, wordWrap: 'on', scrollbar: { verticalScrollbarSize: 7, horizontalScrollbarSize: 7 } }}
          />
        </div>
        <footer className="editor-status"><span><span className="status-dot" />Ready to edit</span><div className="flex items-center gap-5"><span>Python / Starlark</span><span>Ln {position.lineNumber}, Col {position.column}</span><span className="encoding-label">UTF-8</span></div></footer>
      </section>
    </>
  )
}
