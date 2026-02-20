package router

import (
	"net/http"

	"github.com/bananalabs-oss/hand/internal/parties"
	potassium "github.com/bananalabs-oss/potassium/middleware"
	"github.com/gin-gonic/gin"
	"github.com/uptrace/bun"
)

func Setup(db *bun.DB, jwtSecret, serviceToken string) *gin.Engine {
	r := gin.Default()

	h := parties.NewHandler(db)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "hand"})
	})

	// Player-facing endpoints (JWT auth via Potassium)
	api := r.Group("/parties")
	api.Use(potassium.JWTAuth(potassium.JWTConfig{
		Secret: []byte(jwtSecret),
	}))
	{
		api.POST("", h.CreateParty)
		api.GET("/mine", h.GetMyParty)
		api.POST("/join", h.JoinParty)
		api.POST("/leave", h.LeaveParty)
		api.POST("/kick", h.KickMember)
		api.POST("/transfer", h.TransferOwnership)
		api.DELETE("", h.DisbandParty)
		api.POST("/invite", h.RegenerateInvite)
	}

	// Internal endpoints (service token auth via Potassium)
	internal := r.Group("/internal/parties")
	internal.Use(potassium.ServiceAuth(serviceToken))
	{
		internal.GET("/:partyId", h.GetPartyByID)
		internal.GET("/player/:userId", h.GetPlayerParty)
	}

	return r
}
