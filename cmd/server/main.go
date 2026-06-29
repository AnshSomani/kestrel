package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kestrel/internal/api"
	"kestrel/internal/circuitbreaker"
	"kestrel/internal/metrics"
	"kestrel/internal/queue"
	"kestrel/internal/ratelimit"
	"kestrel/internal/store"
	"kestrel/internal/worker"
	"kestrel/pkg/config"
)

func main() {
	// 1. Load configuration from environment variables
	cfg := config.Load()

	// 2. Setup structured JSON logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("kestrel starting",
		"server_port", cfg.ServerPort,
		"worker_pool_size", cfg.WorkerPoolSize,
		"max_concurrent", cfg.MaxConcurrent,
	)

	// 3. Connect to PostgreSQL
	ctx := context.Background()
	pg, err := store.NewPostgresStore(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err)
		os.Exit(1)
	}
	defer pg.Close()
	logger.Info("connected to postgres")

	// 4. Run database migrations
	if err := pg.RunMigrations(ctx); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations completed")

	// 4b. Seed admin user for dashboard
	// Initialize a dummy handler just to use the CreateAdminUser method, 
	// or we can just run the query directly, but let's instantiate the handler later.
	// Actually, we'll create the handler later. We can just execute the raw SQL here, or move Handler creation up.
	// Since Handler needs the Queue (which is initialized below), let's wait until Handler is initialized.

	// 5. Connect to Redis
	redisStore, err := store.NewRedisStore(cfg.RedisURL)
	if err != nil {
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	defer redisStore.Close()
	logger.Info("connected to redis")

	// 6. Initialize Prometheus metrics
	m := metrics.NewMetrics()

	// 7. Initialize core components
	q := queue.NewPostgresQueue(pg.Pool())
	cb := circuitbreaker.New(
		redisStore.Client(),
		cfg.CBFailThreshold,
		cfg.CBWindowSize,
		cfg.CBResetTimeout,
		m,
	)
	rl := ratelimit.NewSlidingWindowLimiter(redisStore.Client(), m)
	deliverer := worker.NewDeliverer(cfg.DeliveryTimeout, cfg.DryRun)

	// 8. Start worker pool
	pool := worker.NewPool(
		worker.PoolConfig{
			Size:          cfg.WorkerPoolSize,
			MaxConcurrent: cfg.MaxConcurrent,
			PollInterval:  cfg.PollInterval,
			BatchSize:     cfg.DequeueBatch,
			NumPollers:    cfg.NumPollers,
		},
		q,
		deliverer,
		cb,
		rl,
		worker.RetryConfig{
			MaxAttempts: cfg.RetryMaxAttempts,
			BaseDelay:   cfg.RetryBaseDelay,
			MaxDelay:    cfg.RetryMaxDelay,
		},
		worker.RateLimitConfig{
			Rate:   cfg.RateLimitRate,
			Window: cfg.RateLimitWindow,
		},
		m,
		logger,
	)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	pool.Start(cancelCtx)
	logger.Info("worker pool started",
		"workers", cfg.WorkerPoolSize,
		"max_concurrent", cfg.MaxConcurrent,
	)

	// Start cleanup worker
	cleanup := worker.NewCleanupWorker(pg.Pool(), logger)
	cleanup.Start()
	logger.Info("cleanup worker started")

	// 9. Setup HTTP server
	handler := api.NewHandler(pg.Pool(), q, m, logger, cfg)
	
	if err := handler.CreateAdminUser(ctx, "admin@kestrel.local", "password"); err != nil {
		logger.Info("admin user already exists or failed to create")
	} else {
		logger.Info("seeded default admin user: admin@kestrel.local / password")
	}

	router := api.SetupRouter(handler, cfg, m, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 10. Graceful shutdown handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("kestrel server starting", "port", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server listen error", "error", err)
			os.Exit(1)
		}
	}()

	// Block until shutdown signal
	sig := <-quit
	logger.Info("shutdown signal received", "signal", sig.String())

	// Stop cleanup worker
	cleanup.Stop()
	logger.Info("cleanup worker stopped")

	// Cancel worker context first to stop polling for new jobs
	cancel()

	// Shutdown worker pool — drain in-flight deliveries
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := pool.Shutdown(shutdownCtx); err != nil {
		logger.Error("worker pool shutdown error", "error", err)
	} else {
		logger.Info("worker pool stopped")
	}

	// Shutdown HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	} else {
		logger.Info("http server stopped")
	}

	logger.Info("kestrel stopped")
}
