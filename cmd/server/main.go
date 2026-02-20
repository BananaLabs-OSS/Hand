package main

import (
	"context"
	"fmt"
	"log"

	"github.com/bananalabs-oss/hand/internal/models"
	"github.com/bananalabs-oss/hand/internal/router"
	"github.com/bananalabs-oss/potassium/config"
	"github.com/bananalabs-oss/potassium/database"
	"github.com/bananalabs-oss/potassium/server"
)

func main() {
	log.Printf("Starting Hand")

	jwtSecret := config.RequireEnv("JWT_SECRET")
	serviceToken := config.RequireEnv("SERVICE_TOKEN")
	databaseURL := config.EnvOrDefault("DATABASE_URL", "sqlite://hand.db")
	host := config.EnvOrDefault("HOST", "0.0.0.0")
	port := config.EnvOrDefault("PORT", "8003")

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

	if err := database.Migrate(ctx, db, []interface{}{
		(*models.Party)(nil),
		(*models.PartyMember)(nil),
	}, []database.Index{
		{Name: "idx_party_members_unique", Query: "CREATE UNIQUE INDEX IF NOT EXISTS idx_party_members_unique ON party_members (party_id, account_id)"},
		{Name: "idx_party_members_account", Query: "CREATE UNIQUE INDEX IF NOT EXISTS idx_party_members_account ON party_members (account_id)"},
	}); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	r := router.Setup(db, jwtSecret, serviceToken)

	addr := fmt.Sprintf("%s:%s", host, port)
	server.ListenAndShutdown(addr, r, "Hand")
}
