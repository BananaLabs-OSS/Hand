package main

import (
	"fmt"

	"github.com/BananaLabs-OSS/Fiber/pulp"
	"github.com/BananaLabs-OSS/Fiber/pulp/cellconfig"
	pulpgin "github.com/BananaLabs-OSS/Fiber/pulp/gin"
	"github.com/BananaLabs-OSS/Fiber/pulp/gin/middleware"
	"github.com/BananaLabs-OSS/Fiber/pulp/workflow"
)

func main() {}
func init() { pulp.OnInit(bootstrap) }

type config struct {
	JWTSecret    string `json:"jwt_secret"`
	ServiceToken string `json:"service_token"`
}

func parseConfig(data []byte) (config, error) {
	var c config
	if len(data) == 0 {
		return c, fmt.Errorf("missing [config] — manifest must set jwt_secret and service_token")
	}
	if err := cellconfig.Decode(data, &c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	if c.JWTSecret == "" {
		return c, fmt.Errorf("jwt_secret missing from [config]")
	}
	if c.ServiceToken == "" {
		return c, fmt.Errorf("service_token missing from [config]")
	}
	return c, nil
}
func bootstrap(configBytes []byte) error {
	cfg, err := parseConfig(configBytes)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	h := &Handler{client: workflow.NewClient("lua-orchestrator")}
	r := pulpgin.New()
	r.GET("/health", func(c *pulpgin.Context) { c.JSON(200, pulpgin.H{"status": "ok", "service": "hand"}) })
	api := r.Group("/parties")
	api.Use(middleware.JWTAuth(middleware.JWTConfig{Secret: []byte(cfg.JWTSecret)}))
	api.POST("", h.CreateParty)
	api.GET("/mine", h.GetMyParty)
	api.POST("/join", h.JoinParty)
	api.POST("/leave", h.LeaveParty)
	api.POST("/kick", h.KickMember)
	api.POST("/transfer", h.TransferOwnership)
	api.DELETE("", h.DisbandParty)
	api.POST("/invite", h.RegenerateInvite)
	internal := r.Group("/internal/parties")
	internal.Use(middleware.ServiceAuth(cfg.ServiceToken))
	internal.GET("/:partyId", h.GetPartyByID)
	internal.GET("/player/:userId", h.GetPlayerParty)
	if err := r.Run(); err != nil {
		return fmt.Errorf("router: %w", err)
	}
	return nil
}
