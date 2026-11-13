import { loader, type BeforeMount } from '@monaco-editor/react'
import * as monaco from 'monaco-editor/editor/editor.api'
import 'monaco-editor/languages/definitions/python/register.js'
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'

self.MonacoEnvironment = { getWorker: () => new EditorWorker() }
loader.config({ monaco })

export const configureTheme: BeforeMount = (editor) => {
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

