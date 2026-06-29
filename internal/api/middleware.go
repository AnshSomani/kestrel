package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// AuthMiddleware returns an Echo middleware that validates the X-API-Key header against the database.
func AuthMiddleware(pool *pgxpool.Pool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := c.Request().Header.Get("X-API-Key")
			if key == "" {
				return c.JSON(http.StatusUnauthorized, echo.Map{
					"error": "missing X-API-Key header",
				})
			}

			var userID string
			err := pool.QueryRow(c.Request().Context(), "SELECT user_id FROM api_keys WHERE key = $1", key).Scan(&userID)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, echo.Map{
					"error": "invalid API key",
				})
			}

			c.Set("user_id", userID)
			return next(c)
		}
	}
}

// JWTMiddleware returns an Echo middleware that validates the Bearer JWT token.
// This is used for human-facing dashboard endpoints.
func JWTMiddleware(jwtSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, echo.Map{"error": "missing Authorization header"})
			}

			// Expect "Bearer <token>"
			if len(authHeader) < 7 || authHeader[:7] != "Bearer " {
				return c.JSON(http.StatusUnauthorized, echo.Map{"error": "invalid Authorization header format"})
			}

			tokenString := authHeader[7:]
			claims, err := ValidateToken(tokenString, jwtSecret)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, echo.Map{"error": "invalid or expired access token"})
			}

			// Store claims in context for downstream handlers
			c.Set("user", claims)
			c.Set("user_id", claims.UserID)
			return next(c)
		}
	}
}

// RequestLogger returns an Echo middleware that logs each request using slog.
func RequestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			// Process request
			err := next(c)

			duration := time.Since(start)
			status := c.Response().Status

			if err != nil {
				c.Error(err)
				status = c.Response().Status
			}

			// Do not log health checks to avoid noise
			if c.Request().URL.Path == "/health" || c.Request().URL.Path == "/metrics" {
				return err
			}

			logAttrs := []any{
				slog.String("method", c.Request().Method),
				slog.String("path", c.Request().URL.Path),
				slog.Int("status", status),
				slog.Duration("latency", duration),
				slog.String("ip", c.RealIP()),
			}

			if status >= 500 {
				logger.Error("server error", logAttrs...)
			} else if status >= 400 {
				logger.Warn("client error", logAttrs...)
			} else {
				logger.Info("request", logAttrs...)
			}

			return err
		}
	}
}

// MultiAuthMiddleware returns an Echo middleware that validates EITHER the X-API-Key OR the Bearer JWT token.
func MultiAuthMiddleware(pool *pgxpool.Pool, jwtSecret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 1. Try API Key
			key := c.Request().Header.Get("X-API-Key")
			if key != "" {
				var userID string
				err := pool.QueryRow(c.Request().Context(), "SELECT user_id FROM api_keys WHERE key = $1", key).Scan(&userID)
				if err == nil {
					c.Set("user_id", userID)
					return next(c)
				}
			}

			// 2. Try JWT
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader != "" {
				if len(authHeader) >= 7 && authHeader[:7] == "Bearer " {
					tokenString := authHeader[7:]
					claims, err := ValidateToken(tokenString, jwtSecret)
					if err == nil {
						c.Set("user", claims)
						c.Set("user_id", claims.UserID)
						return next(c)
					}
				}
			}

			return c.JSON(http.StatusUnauthorized, echo.Map{"error": "missing or invalid authentication"})
		}
	}
}

// AdminOnlyMiddleware returns an Echo middleware that requires the authenticated user to be an admin.
// It must be used AFTER JWTMiddleware or MultiAuthMiddleware which set the "user" context key.
func AdminOnlyMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims, ok := c.Get("user").(*JWTClaims)
			if !ok {
				return c.JSON(http.StatusUnauthorized, echo.Map{"error": "unauthorized"})
			}
			if claims.Role != "admin" {
				return c.JSON(http.StatusForbidden, echo.Map{"error": "forbidden: requires admin role"})
			}
			return next(c)
		}
	}
}

// CORSMiddleware returns an Echo middleware that sets permissive CORS headers
func CORSMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			origin := c.Request().Header.Get("Origin")
			if origin == "" {
				origin = "*"
			}
			c.Response().Header().Set("Access-Control-Allow-Origin", origin)
			c.Response().Header().Set("Access-Control-Allow-Credentials", "true")
			c.Response().Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-API-Key")
			c.Response().Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

			if c.Request().Method == "OPTIONS" {
				return c.NoContent(http.StatusNoContent)
			}

			return next(c)
		}
	}
}
