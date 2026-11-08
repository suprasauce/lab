import { useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import AppLayout from './components/AppLayout'
import EditorPage, { starterStrategy } from './pages/EditorPage'
import BacktestsPage from './pages/BacktestsPage'
import BacktestDetailPage from './pages/BacktestDetailPage'
import StrategiesPage from './pages/StrategiesPage'
import TemplatesPage from './pages/TemplatesPage'

export default function App() {
  const [script, setScript] = useState(starterStrategy)

  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<Navigate to="/editor" replace />} />
        <Route path="editor" element={<EditorPage script={script} onChange={setScript} />} />
        <Route path="backtests" element={<BacktestsPage />} />
        <Route path="backtests/:id" element={<BacktestDetailPage />} />
        <Route path="strategies" element={<StrategiesPage />} />
        <Route path="templates" element={<TemplatesPage />} />
        <Route path="*" element={<Navigate to="/editor" replace />} />
      </Route>
    </Routes>
  )
}
