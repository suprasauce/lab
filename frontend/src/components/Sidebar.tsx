import { BarChart3, ChartNoAxesColumnIncreasing, Code2, Copy, Folder, UserRound } from 'lucide-react'
import { NavLink } from 'react-router-dom'

const navigation = [
  { to: '/editor', label: 'Editor', icon: Code2 },
  { to: '/backtests', label: 'My Backtests', icon: BarChart3 },
  { to: '/strategies', label: 'My Strategies', icon: Folder },
  { to: '/templates', label: 'Templates', icon: Copy },
]

export default function Sidebar() {
  return (
    <aside className="sidebar">
      <div className="brand"><ChartNoAxesColumnIncreasing size={29} strokeWidth={3} /><span>Lab</span></div>
      <nav aria-label="Main navigation" className="flex flex-col gap-2">
        {navigation.map(({ to, label, icon: Icon }) => (
          <NavLink key={to} to={to} title={label} className={({ isActive }) => `nav-link ${isActive ? 'nav-link-active' : ''}`}>
            <Icon size={21} strokeWidth={1.8} aria-hidden="true" />
            <span className="nav-label">{label}</span>
          </NavLink>
        ))}
      </nav>
      <div className="sidebar-footer">
        <div className="profile-avatar" role="img" aria-label="Profile"><UserRound size={21} /></div>
        <span className="profile-label">Your workspace</span>
      </div>
    </aside>
  )
}
