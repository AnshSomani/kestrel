import './MetricCard.css'

interface MetricCardProps {
  label: string
  value: string | number
  subtitle?: string
  icon: string
  color?: 'purple' | 'cyan' | 'green' | 'yellow' | 'red'
  pulse?: boolean
}

export default function MetricCard({
  label,
  value,
  subtitle,
  icon,
  color = 'purple',
  pulse = false,
}: MetricCardProps) {
  return (
    <div className={`metric-card metric-card--${color}`}>
      <div className="metric-card-header">
        <span className="metric-card-label">{label}</span>
        <span className="metric-card-icon">{icon}</span>
      </div>
      <div className="metric-card-value">
        {value}
        {pulse && <span className="metric-card-pulse" />}
      </div>
      {subtitle && <div className="metric-card-subtitle">{subtitle}</div>}
    </div>
  )
}
