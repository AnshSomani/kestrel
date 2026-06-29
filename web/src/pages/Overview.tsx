import { PieChart, Pie, Cell, BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import MetricCard from '../components/MetricCard'
import DeliveryFeed from '../components/DeliveryFeed'
import { useStats, formatNumber } from '../hooks/useApi'
import './Overview.css'

const CHART_COLORS: Record<string, string> = {
  delivered: '#10b981',
  pending: '#f59e0b',
  in_flight: '#06b6d4',
  failed: '#ef4444',
  dead: '#8b5cf6',
}

export default function Overview() {
  const { data, loading } = useStats(3000)

  if (loading || !data) {
    return (
      <div className="overview-loading">
        <div className="loading-spinner" />
        <p>Connecting to Kestrel...</p>
      </div>
    )
  }

  const totalDeliveries = Object.values(data.deliveries).reduce((a, b) => a + b, 0)
  const deliveryRate = totalDeliveries > 0
    ? ((data.deliveries.delivered || 0) / totalDeliveries * 100).toFixed(1)
    : '0.0'

  const pieData = Object.entries(data.deliveries)
    .filter(([, v]) => v > 0)
    .map(([status, count]) => ({
      name: status.replace('_', ' '),
      value: count,
      color: CHART_COLORS[status] || '#64748b',
    }))

  const barData = Object.entries(data.deliveries)
    .filter(([, v]) => v > 0)
    .map(([status, count]) => ({
      status: status.replace('_', ' '),
      count,
      fill: CHART_COLORS[status] || '#64748b',
    }))

  return (
    <div className="overview">
      <div className="overview-header">
        <h1>Dashboard</h1>
        <span className="overview-subtitle">Real-time webhook delivery metrics</span>
      </div>

      <div className="metrics-grid">
        <MetricCard
          label="Total Events"
          value={formatNumber(data.total_events)}
          icon="⚡"
          color="purple"
          subtitle="Ingested events"
        />
        <MetricCard
          label="Delivery Rate"
          value={`${deliveryRate}%`}
          icon="✅"
          color="green"
          subtitle={`${formatNumber(data.deliveries.delivered || 0)} delivered`}
        />
        <MetricCard
          label="Queue Depth"
          value={formatNumber(data.queue_depth)}
          icon="📋"
          color="cyan"
          pulse={data.queue_depth > 0}
          subtitle="Pending jobs"
        />
        <MetricCard
          label="Subscribers"
          value={formatNumber(data.active_subscriptions)}
          icon="🔗"
          color="yellow"
          subtitle="Active endpoints"
        />
      </div>

      <div className="charts-grid">
        <div className="glass-card chart-card">
          <h3 className="chart-title">Delivery Volume</h3>
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={barData} barSize={32}>
              <XAxis
                dataKey="status"
                tick={{ fill: '#94a3b8', fontSize: 11 }}
                axisLine={false}
                tickLine={false}
              />
              <YAxis
                tick={{ fill: '#94a3b8', fontSize: 11 }}
                axisLine={false}
                tickLine={false}
              />
              <Tooltip
                cursor={false}
                content={({ active, payload }) => {
                  if (active && payload && payload.length) {
                    return (
                      <div style={{
                        background: '#1a1a25',
                        border: '1px solid rgba(255,255,255,0.1)',
                        borderRadius: '8px',
                        padding: '8px 12px',
                        color: '#f1f5f9',
                        fontSize: '12px'
                      }}>
                        <span style={{ color: payload[0].payload.fill, marginRight: '8px' }}>
                          {payload[0].payload.status}
                        </span>
                        <span style={{ color: '#ffffff', fontWeight: 500 }}>
                          {payload[0].value}
                        </span>
                      </div>
                    );
                  }
                  return null;
                }}
              />
              <Bar dataKey="count" radius={[6, 6, 0, 0]}>
                {barData.map((entry, i) => (
                  <Cell key={i} fill={entry.fill} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="glass-card chart-card">
          <h3 className="chart-title">Status Distribution</h3>
          {pieData.length > 0 ? (
            <ResponsiveContainer width="100%" height={260}>
              <PieChart>
                <Pie
                  data={pieData}
                  cx="50%"
                  cy="50%"
                  innerRadius={60}
                  outerRadius={100}
                  paddingAngle={3}
                  dataKey="value"
                  stroke="none"
                >
                  {pieData.map((entry, i) => (
                    <Cell key={i} fill={entry.color} />
                  ))}
                </Pie>
                <Tooltip
                  content={({ active, payload }) => {
                    if (active && payload && payload.length) {
                      return (
                        <div style={{
                          background: '#1a1a25',
                          border: '1px solid rgba(255,255,255,0.1)',
                          borderRadius: '8px',
                          padding: '8px 12px',
                          color: '#f1f5f9',
                          fontSize: '12px'
                        }}>
                          <span style={{ color: payload[0].payload.color, marginRight: '8px' }}>
                            {payload[0].payload.name}
                          </span>
                          <span style={{ color: '#ffffff', fontWeight: 500 }}>
                            {payload[0].value}
                          </span>
                        </div>
                      );
                    }
                    return null;
                  }}
                />
              </PieChart>
            </ResponsiveContainer>
          ) : (
            <div className="chart-empty">No delivery data yet</div>
          )}
          <div className="chart-legend">
            {pieData.map((entry) => (
              <div key={entry.name} className="legend-item">
                <span className="legend-dot" style={{ background: entry.color }} />
                <span className="legend-label">{entry.name}</span>
                <span className="legend-value mono">{formatNumber(entry.value)}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      <DeliveryFeed deliveries={data.recent_deliveries} />
    </div>
  )
}
