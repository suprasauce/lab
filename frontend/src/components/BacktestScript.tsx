import { useState } from 'react'
import Editor from '@monaco-editor/react'
import { Check, Copy } from 'lucide-react'
import { configureTheme } from '../editor'

export default function BacktestScript({ script }: { script?: string | null }) {
  const [copied, setCopied] = useState(false)
  const [copyError, setCopyError] = useState('')

  async function copyScript() {
    if (!script) return
    try {
      await navigator.clipboard.writeText(script)
      setCopied(true)
      setCopyError('')
    } catch {
      setCopyError('Unable to copy automatically. Select the script and copy it manually.')
    }
  }

  return (
    <>
      <div className="historical-script-header">
        <p className="page-subtitle">Read-only script saved with this historical backtest.</p>
        {script?.trim() && (
          <button type="button" className="secondary-button" onClick={() => void copyScript()}>
            {copied ? <Check size={15} aria-hidden="true" /> : <Copy size={15} aria-hidden="true" />}
            {copied ? 'Copied' : 'Copy Script'}
          </button>
        )}
      </div>
      <p className="script-copy-status" role="status">{copyError || (copied ? 'Script copied to clipboard.' : '')}</p>
      {script?.trim() ? (
        <div className="editor-panel historical-script-editor">
          <Editor height="100%" language="python" theme="lab-dark" value={script}
            beforeMount={configureTheme}
            loading={<span className="editor-loading">Loading script...</span>}
            options={{ readOnly: true, domReadOnly: true, automaticLayout: true,
              minimap: { enabled: false }, fontSize: 13, lineNumbers: 'on',
              scrollBeyondLastLine: false, padding: { top: 16 }, ariaLabel: 'Saved backtest script' }} />
        </div>
      ) : <p className="panel-message">No saved script is available for this backtest.</p>}
    </>
  )
}
