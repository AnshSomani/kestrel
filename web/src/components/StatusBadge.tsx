import './StatusBadge.css'

const statusConfig: Record<string, { color: string; label: string }> = {
  delivered: { color: 'green', label: 'Delivered' },
  pending: { color: 'yellow', label: 'Pending' },
  in_flight: { color: 'cyan', label: 'In Flight' },
  failed: { color: 'red', label: 'Failed' },
  dead: { color: 'red', label: 'Dead' },
  rate_limited: { color: 'yellow', label: 'Rate Limited' },
  circuit_open: { color: 'red', label: 'Circuit Open' },
}

interface StatusBadgeProps {
  status: string
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  const config = statusConfig[status] || { color: 'purple', label: status }
  return (
    <span className={`status-badge status-badge--${config.color}`}>
      {config.label}
    </span>
  )
}
