package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"kestrel/pkg/config"
)

func main() {
	cfg := config.Load()
	conn, err := pgx.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		fmt.Printf("Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	res, err := conn.Exec(context.Background(), "UPDATE delivery_jobs SET status = 'pending' WHERE status = 'in_flight'")
	if err != nil {
		fmt.Printf("Error reaping jobs: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Reaped %d stalled jobs!\n", res.RowsAffected())
}
