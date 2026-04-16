// Hand — Pulp plugin port.
//
// Rewrite of the party-system microservice as a WASM plugin. The HTTP
// shell runs on Fiber's pulpgin router; data access uses Bun over the
// Fiber pulp/sql driver; JWT + service-token auth come from Fiber's
// ported Potassium middleware. Handler business logic is unchanged
// from the original standalone service.
//
// Build:
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o hand.wasm .
package main

import (
	"context"
	dsql "database/sql"
	"encoding/json"
	"fmt"

	"github.com/BananaLabs-OSS/Fiber/pulp"
	pulpgin "github.com/BananaLabs-OSS/Fiber/pulp/gin"
	"github.com/BananaLabs-OSS/Fiber/pulp/gin/middleware"
	_ "github.com/BananaLabs-OSS/Fiber/pulp/sql"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func main() {}

var (
	db      *bun.DB
	handler *Handler
)

func init() {
	pulp.OnInit(bootstrap)
}

// bootstrap opens the SQLite database via Pulp's storage.sqlite, runs
// schema migrations, and wires the router. Config comes from the
// manifest's [config] table.
func bootstrap(configBytes []byte) error {
	cfg, err := parseConfig(configBytes)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	raw, err := dsql.Open("pulp", "")
	if err != nil {
		return fmt.Errorf("open pulp sql driver: %w", err)
	}
	db = bun.NewDB(raw, sqlitedialect.New())

	if err := migrate(context.Background()); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	handler = &Handler{db: db}

	r := pulpgin.New()

	r.GET("/health", func(c *pulpgin.Context) {
		c.JSON(200, pulpgin.H{"status": "ok", "service": "hand"})
	})

	api := r.Group("/parties")
	api.Use(middleware.JWTAuth(middleware.JWTConfig{Secret: []byte(cfg.JWTSecret)}))
	api.POST("", handler.CreateParty)
	api.GET("/mine", handler.GetMyParty)
	api.POST("/join", handler.JoinParty)
	api.POST("/leave", handler.LeaveParty)
	api.POST("/kick", handler.KickMember)
	api.POST("/transfer", handler.TransferOwnership)
	api.DELETE("", handler.DisbandParty)
	api.POST("/invite", handler.RegenerateInvite)

	internal := r.Group("/internal/parties")
	internal.Use(middleware.ServiceAuth(cfg.ServiceToken))
	internal.GET("/:partyId", handler.GetPartyByID)
	internal.GET("/player/:userId", handler.GetPlayerParty)

	if err := r.Run(); err != nil {
		return fmt.Errorf("router: %w", err)
	}
	return nil
}

// migrate creates the parties / party_members tables and indexes if
// they are missing. Matches the shape potassium/database.Migrate
// produced for the original service.
func migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS parties (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			invite_code TEXT NOT NULL UNIQUE,
			max_size INTEGER NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS party_members (
			party_id TEXT NOT NULL,
			account_id TEXT NOT NULL,
			role TEXT NOT NULL,
			joined_at TIMESTAMP NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_party_members_unique ON party_members (party_id, account_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_party_members_account ON party_members (account_id)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("migrate exec: %w", err)
		}
	}
	return nil
}

type config struct {
	JWTSecret    string `json:"jwt_secret"`
	ServiceToken string `json:"service_token"`
}

// parseConfig reads the MessagePack-encoded manifest [config] table.
// We go through a JSON round-trip to keep the decoding code path the
// same shape handlers use elsewhere; configs are tiny so cost is
// trivial.
func parseConfig(data []byte) (config, error) {
	var cfg config
	if len(data) == 0 {
		return cfg, fmt.Errorf("missing [config] — manifest must set jwt_secret and service_token")
	}
	// Manifest config arrives as MessagePack; decode into a generic map
	// then re-marshal to JSON to let encoding/json bind it to our
	// struct. Avoids pulling the msgpack dependency into main.
	var raw map[string]any
	if err := decodeMsgpack(data, &raw); err != nil {
		return cfg, err
	}
	jbytes, err := json.Marshal(raw)
	if err != nil {
		return cfg, fmt.Errorf("re-marshal config: %w", err)
	}
	if err := json.Unmarshal(jbytes, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	if cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("jwt_secret missing from [config]")
	}
	if cfg.ServiceToken == "" {
		return cfg, fmt.Errorf("service_token missing from [config]")
	}
	return cfg, nil
}
