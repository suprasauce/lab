import type { BacktestStatus } from '../api/backtests'

export default function StatusBadge({ status }: { status: BacktestStatus }) {
  return <span className={`status-badge status-${status.toLowerCase()}`}><span aria-hidden="true" />{status}</span>
}
