import type { DeliveryItem } from '../hooks/useApi'
import { timeAgo, truncate } from '../hooks/useApi'
import StatusBadge from './StatusBadge'
import './DeliveryFeed.css'

interface DeliveryFeedProps {
  deliveries: DeliveryItem[]
}

export default function DeliveryFeed({ deliveries }: DeliveryFeedProps) {
  if (deliveries.length === 0) {
    return (
      <div className="delivery-feed glass-card">
        <h3 className="feed-title">Live Delivery Feed</h3>
        <div className="feed-empty">
          <span className="feed-empty-icon">📭</span>
          <p>No deliveries yet. Fire your first webhook!</p>
        </div>
      </div>
    )
  }

  return (
    <div className="delivery-feed glass-card">
      <h3 className="feed-title">
        Live Delivery Feed
        <span className="feed-count">{deliveries.length}</span>
      </h3>
      <div className="feed-list">
        {deliveries.map((d, i) => (
          <div
            key={d.id}
            className="feed-item animate-fade-in"
            style={{ animationDelay: `${i * 30}ms` }}
          >
            <div className="feed-item-left">
              <span className="feed-event-type mono">{d.event_type}</span>
              <span className="feed-endpoint" title={d.endpoint_url}>
                {truncate(d.endpoint_url, 40)}
              </span>
            </div>
            <div className="feed-item-right">
              <StatusBadge status={d.status} />
              <span className="feed-time" title={d.created_at}>
                {timeAgo(d.created_at)}
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
