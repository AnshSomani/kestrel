package api

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"kestrel/internal/metrics"
	"kestrel/pkg/config"
)

// SetupRouter creates and configures the Gin router with all routes,
// middleware, and Prometheus metrics exposure.
func SetupRouter(h *Handler, cfg *config.Config, m *metrics.Metrics, logger *slog.Logger) *echo.Echo {
	
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
		e.Use(CORSMiddleware())
	e.Use(RequestLogger(logger))

	// Public endpoints (no authentication required)
	e.GET("/health", h.Health)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Auth endpoints
	auth := e.Group("/api/auth")
	{
		auth.POST("/signup", h.Signup)
		auth.POST("/login", h.Login)
		auth.POST("/refresh", h.Refresh)
		auth.POST("/logout", h.Logout)
	}

	// Server-to-Server API endpoints (API Key required)
	api := e.Group("/api")
	api.Use(AuthMiddleware(h.pool))
	{
		// Events
		api.POST("/events", h.CreateEvent)
		api.GET("/events/:id/deliveries", h.GetDeliveryJobs)
	}

	// Shared endpoints (API Key OR JWT allowed)
	shared := e.Group("/api")
	shared.Use(MultiAuthMiddleware(h.pool, cfg.JWTSecret))
	{
		// Subscriptions
		shared.POST("/subscriptions", h.CreateSubscription)
		shared.PUT("/subscriptions/:id", h.UpdateSubscription)
		shared.GET("/subscriptions", h.ListSubscriptions)
	}

	// Dashboard UI endpoints (JWT required)
	dashboard := e.Group("/api")
	dashboard.Use(JWTMiddleware(cfg.JWTSecret))
	{
		dashboard.GET("/stats", h.Stats)
		dashboard.GET("/events", h.ListEvents)
		dashboard.GET("/events/:id", h.GetEvent)
		dashboard.GET("/keys", h.ListAPIKeys)
		dashboard.POST("/keys", h.CreateAPIKey)
		dashboard.DELETE("/keys/:id", h.DeleteAPIKey)

		// Admin-only dashboard endpoints
		admin := dashboard.Group("")
		admin.Use(AdminOnlyMiddleware())
		{
			admin.GET("/users", h.ListUsers)
		}
	}

	return e
}
