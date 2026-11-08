import { Outlet } from 'react-router-dom'
import Sidebar from './Sidebar'

export default function AppLayout() {
  return (
    <div className="app-shell">
      <a href="#main-content" className="skip-link">Skip to content</a>
      <Sidebar />
      <main id="main-content" className="main-pane" tabIndex={-1}>
        <Outlet />
      </main>
    </div>
  )
}
