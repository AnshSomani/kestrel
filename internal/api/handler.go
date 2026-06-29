package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"

	"kestrel/internal/metrics"
	"kestrel/internal/queue"
	"kestrel/pkg/config"
)

type Handler struct {
	pool    *pgxpool.Pool
	queue   *queue.PostgresQueue
	metrics *metrics.Metrics
	logger  *slog.Logger
	config  *config.Config
}

func NewHandler(pool *pgxpool.Pool, q *queue.PostgresQueue, m *metrics.Metrics, logger *slog.Logger, cfg *config.Config) *Handler {
	return &Handler{
		pool:    pool,
		queue:   q,
		metrics: m,
		logger:  logger,
		config:  cfg,
	}
}

type createEventRequest struct {
	Type           string          `json:"type" binding:"required"`
	Payload        json.RawMessage `json:"payload" binding:"required"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
}

type eventResponse struct {
	ID                string          `json:"id"`
	Type              string          `json:"type"`
	Payload           json.RawMessage `json:"payload"`
	IdempotencyKey    *string         `json:"idempotency_key,omitempty"`
	DeliveriesCreated int             `json:"deliveries_created,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

type createSubscriptionRequest struct {
	EndpointURL string   `json:"endpoint_url" binding:"required"`
	Secret      string   `json:"secret" binding:"required"`
	EventTypes  []string `json:"event_types" binding:"required,min=1"`
}

type subscriptionResponse struct {
	ID          string    `json:"id"`
	EndpointURL string    `json:"endpoint_url"`
	Secret      string    `json:"secret"`
	EventTypes  []string  `json:"event_types"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

type eventsListResponse struct {
	Events     []eventResponse `json:"events"`
	NextCursor *string         `json:"next_cursor,omitempty"`
}

type healthResponse struct {
	Status     string `json:"status"`
	Postgres   string `json:"postgres"`
	QueueDepth int64  `json:"queue_depth"`
}

type statsResponse struct {
	TotalEvents         int64                `json:"total_events"`
	Deliveries          map[string]int64     `json:"deliveries"`
	QueueDepth          int64                `json:"queue_depth"`
	ActiveSubscriptions int64                `json:"active_subscriptions"`
	RecentDeliveries    []recentDeliveryItem `json:"recent_deliveries"`
}

type recentDeliveryItem struct {
	ID             string    `json:"id"`
	EventType      string    `json:"event_type"`
	EndpointURL    string    `json:"endpoint_url"`
	Status         string    `json:"status"`
	AttemptCount   int       `json:"attempt_count"`
	LastStatusCode *int      `json:"last_status_code,omitempty"`
	LastError      *string   `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
}

func (h *Handler) CreateEvent(c echo.Context) error {
	var req createEventRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request: type and payload are required"})
	}

	if !json.Valid(req.Payload) {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "payload must be valid JSON"})
	}

	ctx := c.Request().Context()
	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == nil {
		key := generateRandomKey()
		idempotencyKey = &key
	}

	userIDStr, ok := c.Get("user_id").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{"error": "unauthorized"})
	}
	tenantID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "invalid user ID"})
	}

	var eventID uuid.UUID
	var eventType string
	var payload json.RawMessage
	var createdAt time.Time
	var wasInserted bool

	err = h.pool.QueryRow(ctx,
		`INSERT INTO events (type, payload, idempotency_key, user_id)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING id, type, payload, created_at`,
		req.Type, req.Payload, *idempotencyKey, tenantID,
	).Scan(&eventID, &eventType, &payload, &createdAt)

	if err != nil {
		row := h.pool.QueryRow(ctx,
			`SELECT id, type, payload, created_at FROM events WHERE idempotency_key = $1 AND user_id = $2`,
			*idempotencyKey, tenantID,
		)
		if scanErr := row.Scan(&eventID, &eventType, &payload, &createdAt); scanErr != nil {
			h.logger.Error("failed to fetch existing event", "error", scanErr)
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
		}
		wasInserted = false
	} else {
		wasInserted = true
	}

	if !wasInserted {
		return c.JSON(http.StatusOK, eventResponse{
			ID:             eventID.String(),
			Type:           eventType,
			Payload:        payload,
			IdempotencyKey: idempotencyKey,
			CreatedAt:      createdAt,
		})
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id FROM subscriptions WHERE is_active = true AND user_id = $1 AND $2 = ANY(event_types)`,
		tenantID, req.Type,
	)
	if err != nil {
		h.logger.Error("failed to query subscriptions", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
	}
	defer rows.Close()

	var subscriptionIDs []uuid.UUID
	for rows.Next() {
		var subID uuid.UUID
		if err := rows.Scan(&subID); err != nil {
			h.logger.Error("failed to scan subscription ID", "error", err)
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
		}
		subscriptionIDs = append(subscriptionIDs, subID)
	}
	if err := rows.Err(); err != nil {
		h.logger.Error("rows iteration error", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
	}

	deliveriesCreated := 0
	if len(subscriptionIDs) > 0 {
		if err := h.queue.EnqueueBatch(ctx, eventID, subscriptionIDs, tenantID); err != nil {
			h.logger.Error("failed to enqueue delivery jobs", "error", err, "event_id", eventID)
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to create delivery jobs"})
		}
		deliveriesCreated = len(subscriptionIDs)
	}

	h.metrics.EventsIngested.Inc()

	return c.JSON(http.StatusCreated, eventResponse{
		ID:                eventID.String(),
		Type:              eventType,
		Payload:           payload,
		IdempotencyKey:    idempotencyKey,
		DeliveriesCreated: deliveriesCreated,
		CreatedAt:         createdAt,
	})
}

func (h *Handler) ListEvents(c echo.Context) error {
	ctx := c.Request().Context()
	limitStr := c.QueryParam("limit")
	if limitStr == "" {
		limitStr = "20"
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	cursor := c.QueryParam("cursor")
	eventType := c.QueryParam("type")
	var events []eventResponse

	userIDStr, _ := c.Get("user_id").(string)
	tenantID, _ := uuid.Parse(userIDStr)

	if cursor != "" {
		cursorUUID, err := uuid.Parse(cursor)
		if err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid cursor: must be a valid UUID"})
		}

		if eventType != "" {
			rows, err := h.pool.Query(ctx,
				`SELECT e.id, e.type, e.payload, e.idempotency_key, e.created_at
				 FROM events e
				 WHERE e.user_id = $1 AND (e.created_at, e.id) < (SELECT created_at, id FROM events WHERE id = $2)
				   AND e.type = $3
				 ORDER BY e.created_at DESC, e.id DESC
				 LIMIT $4`,
				tenantID, cursorUUID, eventType, limit,
			)
			if err != nil {
				h.logger.Error("failed to query events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
			defer rows.Close()
			events, err = scanEvents(rows)
			if err != nil {
				h.logger.Error("failed to scan events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
		} else {
			rows, err := h.pool.Query(ctx,
				`SELECT e.id, e.type, e.payload, e.idempotency_key, e.created_at
				 FROM events e
				 WHERE e.user_id = $1 AND (e.created_at, e.id) < (SELECT created_at, id FROM events WHERE id = $2)
				 ORDER BY e.created_at DESC, e.id DESC
				 LIMIT $3`,
				tenantID, cursorUUID, limit,
			)
			if err != nil {
				h.logger.Error("failed to query events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
			defer rows.Close()
			events, err = scanEvents(rows)
			if err != nil {
				h.logger.Error("failed to scan events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
		}
	} else {
		if eventType != "" {
			rows, err := h.pool.Query(ctx,
				`SELECT id, type, payload, idempotency_key, created_at
				 FROM events
				 WHERE user_id = $1 AND type = $2
				 ORDER BY created_at DESC, id DESC
				 LIMIT $3`,
				tenantID, eventType, limit,
			)
			if err != nil {
				h.logger.Error("failed to query events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
			defer rows.Close()
			events, err = scanEvents(rows)
			if err != nil {
				h.logger.Error("failed to scan events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
		} else {
			rows, err := h.pool.Query(ctx,
				`SELECT id, type, payload, idempotency_key, created_at
				 FROM events
				 WHERE user_id = $1
				 ORDER BY created_at DESC, id DESC
				 LIMIT $2`,
				tenantID, limit,
			)
			if err != nil {
				h.logger.Error("failed to query events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
			defer rows.Close()
			events, err = scanEvents(rows)
			if err != nil {
				h.logger.Error("failed to scan events", "error", err)
				return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
			}
		}
	}

	resp := eventsListResponse{Events: events}
	if len(events) == limit {
		lastID := events[len(events)-1].ID
		resp.NextCursor = &lastID
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) GetEvent(c echo.Context) error {
	ctx := c.Request().Context()
	idStr := c.Param("id")
	eventID, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid event ID"})
	}

	userIDStr, _ := c.Get("user_id").(string)
	tenantID, _ := uuid.Parse(userIDStr)

	var ev eventResponse
	err = h.pool.QueryRow(ctx,
		`SELECT id, type, payload, idempotency_key, created_at
		 FROM events WHERE id = $1 AND user_id = $2`,
		eventID, tenantID,
	).Scan(&ev.ID, &ev.Type, &ev.Payload, &ev.IdempotencyKey, &ev.CreatedAt)

	if err != nil {
		return c.JSON(http.StatusNotFound, echo.Map{"error": "event not found"})
	}
	return c.JSON(http.StatusOK, ev)
}

func (h *Handler) CreateSubscription(c echo.Context) error {
	var req createSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid request"})
	}
	ctx := c.Request().Context()
	userIDStr, ok := c.Get("user_id").(string)
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{"error": "unauthorized"})
	}
	tenantID, _ := uuid.Parse(userIDStr)

	var sub subscriptionResponse
	err := h.pool.QueryRow(ctx,
		`INSERT INTO subscriptions (endpoint_url, secret, event_types, is_active, user_id)
		 VALUES ($1, $2, $3, true, $4)
		 RETURNING id, endpoint_url, secret, event_types, is_active, created_at`,
		req.EndpointURL, req.Secret, req.EventTypes, tenantID,
	).Scan(&sub.ID, &sub.EndpointURL, &sub.Secret, &sub.EventTypes, &sub.IsActive, &sub.CreatedAt)

	if err != nil {
		h.logger.Error("failed to create subscription", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "failed to create subscription"})
	}
	return c.JSON(http.StatusCreated, sub)
}

func (h *Handler) ListSubscriptions(c echo.Context) error {
	ctx := c.Request().Context()
	userIDStr, _ := c.Get("user_id").(string)
	tenantID, _ := uuid.Parse(userIDStr)

	rows, err := h.pool.Query(ctx,
		`SELECT id, endpoint_url, secret, event_types, is_active, created_at
		 FROM subscriptions
		 WHERE is_active = true AND user_id = $1
		 ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		h.logger.Error("failed to query subscriptions", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
	}
	defer rows.Close()

	var subscriptions []subscriptionResponse
	for rows.Next() {
		var sub subscriptionResponse
		if err := rows.Scan(&sub.ID, &sub.EndpointURL, &sub.Secret, &sub.EventTypes, &sub.IsActive, &sub.CreatedAt); err != nil {
			h.logger.Error("failed to scan subscription", "error", err)
			return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
		}
		subscriptions = append(subscriptions, sub)
	}
	if err := rows.Err(); err != nil {
		h.logger.Error("rows iteration error", "error", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
	}

	if subscriptions == nil {
		subscriptions = []subscriptionResponse{}
	}
	return c.JSON(http.StatusOK, echo.Map{"subscriptions": subscriptions})
}

func (h *Handler) GetDeliveryJobs(c echo.Context) error {
	ctx := c.Request().Context()
	idStr := c.Param("id")
	eventID, err := uuid.Parse(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"error": "invalid event ID"})
	}

	userIDStr, _ := c.Get("user_id").(string)
	tenantID, _ := uuid.Parse(userIDStr)

	jobs, err := h.queue.GetJobsByEvent(ctx, eventID, tenantID)
	if err != nil {
		h.logger.Error("failed to get delivery jobs", "error", err, "event_id", eventID)
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": "internal server error"})
	}

	if jobs == nil {
		jobs = []*queue.Job{}
	}
	return c.JSON(http.StatusOK, echo.Map{"deliveries": jobs})
}

func (h *Handler) Health(c echo.Context) error {
	ctx := c.Request().Context()
	pgStatus := "up"
	if err := h.pool.Ping(ctx); err != nil {
		pgStatus = "down"
		h.logger.Warn("health check: postgres is down", "error", err)
	}

	// For health checks, we can get the total queue depth by summing all tenants, or just return an aggregate if we want.
	// But `GetQueueDepth` now requires a tenantID. Health checks don't have a tenant.
	// Let's just run a direct query.
	var queueDepth int64
	if err := h.pool.QueryRow(ctx, "SELECT SUM(value) FROM system_stats WHERE key = 'delivery_pending'").Scan(&queueDepth); err != nil {
		h.logger.Warn("health check: failed to get total queue depth", "error", err)
		queueDepth = -1
	}

	status := "ok"
	statusCode := http.StatusOK
	if pgStatus != "up" {
		status = "degraded"
		statusCode = http.StatusServiceUnavailable
	}
	return c.JSON(statusCode, healthResponse{
		Status:     status,
		Postgres:   pgStatus,
		QueueDepth: queueDepth,
	})
}

func (h *Handler) Stats(c echo.Context) error {
	ctx := c.Request().Context()
	userIDStr, _ := c.Get("user_id").(string)
	tenantID, _ := uuid.Parse(userIDStr)

	var totalEvents int64
	if err := h.pool.QueryRow(ctx, "SELECT value FROM system_stats WHERE key = 'total_events' AND user_id = $1", tenantID).Scan(&totalEvents); err != nil {
		totalEvents = 0
	}

	deliveries := map[string]int64{
		"pending":   0,
		"in_flight": 0,
		"delivered": 0,
		"failed":    0,
		"dead":      0,
	}
	for status := range deliveries {
		var count int64
		if err := h.pool.QueryRow(ctx, "SELECT value FROM system_stats WHERE key = $1 AND user_id = $2", "delivery_"+status, tenantID).Scan(&count); err == nil {
			deliveries[status] = count
		}
	}

	queueDepth, _ := h.queue.GetQueueDepth(ctx, tenantID)
	var activeSubs int64
	if err := h.pool.QueryRow(ctx, "SELECT COUNT(*) FROM subscriptions WHERE is_active = true AND user_id = $1", tenantID).Scan(&activeSubs); err != nil {
		activeSubs = 0
	}

	var recent []recentDeliveryItem
	recentRows, err := h.pool.Query(ctx, `
		SELECT dj.id, e.type, s.endpoint_url, dj.status, dj.attempt_count,
		       dj.last_status_code, dj.last_error, dj.created_at, dj.delivered_at
		FROM delivery_jobs dj
		JOIN events e ON e.id = dj.event_id
		JOIN subscriptions s ON s.id = dj.subscription_id
		WHERE dj.user_id = $1
		ORDER BY dj.created_at DESC
		LIMIT 50
	`, tenantID)
	if err == nil {
		for recentRows.Next() {
			var item recentDeliveryItem
			if err := recentRows.Scan(
				&item.ID, &item.EventType, &item.EndpointURL, &item.Status,
				&item.AttemptCount, &item.LastStatusCode, &item.LastError,
				&item.CreatedAt, &item.DeliveredAt,
			); err == nil {
				recent = append(recent, item)
			}
		}
		recentRows.Close()
	}
	if recent == nil {
		recent = []recentDeliveryItem{}
	}

	return c.JSON(http.StatusOK, statsResponse{
		TotalEvents:         totalEvents,
		Deliveries:          deliveries,
		QueueDepth:          queueDepth,
		ActiveSubscriptions: activeSubs,
		RecentDeliveries:    recent,
	})
}

func scanEvents(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]eventResponse, error) {
	var events []eventResponse
	for rows.Next() {
		var ev eventResponse
		if err := rows.Scan(&ev.ID, &ev.Type, &ev.Payload, &ev.IdempotencyKey, &ev.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if events == nil {
		events = []eventResponse{}
	}
	return events, nil
}

func generateRandomKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
