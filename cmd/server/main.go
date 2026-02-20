package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bananalabs-oss/hand/internal/database"
	"github.com/bananalabs-oss/hand/internal/router"
)

func main() {
	log.Printf("Starting Hand")

	jwtSecret := requireEnv("JWT_SECRET")
	serviceToken := requireEnv("SERVICE_TOKEN")
	databaseURL := envOrDefault("DATABASE_URL", "sqlite://hand.db")
	host := envOrDefault("HOST", "0.0.0.0")
	port := envOrDefault("PORT", "8003")

	log.Printf("Hand Configuration:")
	log.Printf("  Host:     %s", host)
	log.Printf("  Port:     %s", port)
	log.Printf("  Database: %s", databaseURL)

	ctx := context.Background()

	db, err := database.Connect(databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	r := router.Setup(db, jwtSecret, serviceToken)

	addr := fmt.Sprintf("%s:%s", host, port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("Hand listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("Shutting down Hand...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Printf("Hand stopped")
}

func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("%s is required", key)
	}
	return val
}

func envOrDefault(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
