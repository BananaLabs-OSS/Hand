package parties

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/bananalabs-oss/hand/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Handler struct {
	db *bun.DB
}

func NewHandler(db *bun.DB) *Handler {
	return &Handler{db: db}
}

func generateInviteCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func getAccountID(c *gin.Context) uuid.UUID {
	return uuid.MustParse(c.GetString("account_id"))
}

func (h *Handler) findMembership(ctx context.Context, accountID uuid.UUID) (*models.PartyMember, error) {
	member := new(models.PartyMember)
	err := h.db.NewSelect().
		Model(member).
		Where("account_id = ?", accountID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return member, nil
}

func (h *Handler) getPartyWithMembers(ctx context.Context, partyID uuid.UUID) (*models.Party, error) {
	party := new(models.Party)
	err := h.db.NewSelect().
		Model(party).
		Relation("Members").
		Where("p.id = ?", partyID).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return party, nil
}

// --- Player-facing endpoints ---

func (h *Handler) CreateParty(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	_, err := h.findMembership(ctx, accountID)
	if err == nil {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "already_in_party",
			Message: "You are already in a party. Leave first.",
		})
		return
	}

	now := time.Now().UTC()
	party := &models.Party{
		ID:         uuid.New(),
		OwnerID:    accountID,
		InviteCode: generateInviteCode(),
		MaxSize:    models.DefaultMaxSize,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	member := &models.PartyMember{
		PartyID:   party.ID,
		AccountID: accountID,
		Role:      models.RoleOwner,
		JoinedAt:  now,
	}

	err = h.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(party).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(member).Exec(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "create_failed",
			Message: "Failed to create party",
		})
		return
	}

	party.Members = []models.PartyMember{*member}
	c.JSON(http.StatusCreated, party)
}

func (h *Handler) GetMyParty(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	member, err := h.findMembership(ctx, accountID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_in_party",
			Message: "You are not in a party",
		})
		return
	}

	party, err := h.getPartyWithMembers(ctx, member.PartyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "fetch_failed",
			Message: "Failed to fetch party",
		})
		return
	}

	c.JSON(http.StatusOK, party)
}

func (h *Handler) JoinParty(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	var req struct {
		InviteCode string `json:"invite_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "invite_code is required",
		})
		return
	}

	_, err := h.findMembership(ctx, accountID)
	if err == nil {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "already_in_party",
			Message: "You are already in a party. Leave first.",
		})
		return
	}

	party := new(models.Party)
	err = h.db.NewSelect().
		Model(party).
		Relation("Members").
		Where("invite_code = ?", req.InviteCode).
		Scan(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "invalid_code",
			Message: "Invalid invite code",
		})
		return
	}

	if len(party.Members) >= party.MaxSize {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "party_full",
			Message: "Party is full",
		})
		return
	}

	member := &models.PartyMember{
		PartyID:   party.ID,
		AccountID: accountID,
		Role:      models.RoleMember,
		JoinedAt:  time.Now().UTC(),
	}

	_, err = h.db.NewInsert().Model(member).Exec(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "join_failed",
			Message: "Failed to join party",
		})
		return
	}

	party, _ = h.getPartyWithMembers(ctx, party.ID)
	c.JSON(http.StatusOK, party)
}

func (h *Handler) LeaveParty(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	member, err := h.findMembership(ctx, accountID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_in_party",
			Message: "You are not in a party",
		})
		return
	}

	if member.Role == models.RoleOwner {
		h.disbandParty(c, ctx, member.PartyID)
		return
	}

	_, err = h.db.NewDelete().
		Model((*models.PartyMember)(nil)).
		Where("party_id = ? AND account_id = ?", member.PartyID, accountID).
		Exec(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "leave_failed",
			Message: "Failed to leave party",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Left party"})
}

func (h *Handler) KickMember(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	var req struct {
		AccountID uuid.UUID `json:"account_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "account_id is required",
		})
		return
	}

	if req.AccountID == accountID {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "Cannot kick yourself. Use leave.",
		})
		return
	}

	member, err := h.findMembership(ctx, accountID)
	if err != nil || member.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "not_owner",
			Message: "Only the party owner can kick members",
		})
		return
	}

	target, err := h.findMembership(ctx, req.AccountID)
	if err != nil || target.PartyID != member.PartyID {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_in_party",
			Message: "That player is not in your party",
		})
		return
	}

	_, err = h.db.NewDelete().
		Model((*models.PartyMember)(nil)).
		Where("party_id = ? AND account_id = ?", member.PartyID, req.AccountID).
		Exec(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "kick_failed",
			Message: "Failed to kick member",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member kicked"})
}

func (h *Handler) TransferOwnership(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	var req struct {
		AccountID uuid.UUID `json:"account_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "account_id is required",
		})
		return
	}

	if req.AccountID == accountID {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_request",
			Message: "You are already the owner",
		})
		return
	}

	member, err := h.findMembership(ctx, accountID)
	if err != nil || member.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "not_owner",
			Message: "Only the party owner can transfer ownership",
		})
		return
	}

	target, err := h.findMembership(ctx, req.AccountID)
	if err != nil || target.PartyID != member.PartyID {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_in_party",
			Message: "That player is not in your party",
		})
		return
	}

	err = h.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewUpdate().
			Model((*models.PartyMember)(nil)).
			Set("role = ?", models.RoleMember).
			Where("party_id = ? AND account_id = ?", member.PartyID, accountID).
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.NewUpdate().
			Model((*models.PartyMember)(nil)).
			Set("role = ?", models.RoleOwner).
			Where("party_id = ? AND account_id = ?", member.PartyID, req.AccountID).
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.NewUpdate().
			Model((*models.Party)(nil)).
			Set("owner_id = ?", req.AccountID).
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", member.PartyID).
			Exec(ctx)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "transfer_failed",
			Message: "Failed to transfer ownership",
		})
		return
	}

	party, _ := h.getPartyWithMembers(ctx, member.PartyID)
	c.JSON(http.StatusOK, party)
}

func (h *Handler) DisbandParty(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	member, err := h.findMembership(ctx, accountID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_in_party",
			Message: "You are not in a party",
		})
		return
	}

	if member.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "not_owner",
			Message: "Only the party owner can disband",
		})
		return
	}

	h.disbandParty(c, ctx, member.PartyID)
}

func (h *Handler) RegenerateInvite(c *gin.Context) {
	ctx := c.Request.Context()
	accountID := getAccountID(c)

	member, err := h.findMembership(ctx, accountID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_in_party",
			Message: "You are not in a party",
		})
		return
	}

	if member.Role != models.RoleOwner {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Error:   "not_owner",
			Message: "Only the party owner can regenerate invites",
		})
		return
	}

	newCode := generateInviteCode()
	_, err = h.db.NewUpdate().
		Model((*models.Party)(nil)).
		Set("invite_code = ?", newCode).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", member.PartyID).
		Exec(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "regenerate_failed",
			Message: "Failed to regenerate invite code",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"invite_code": newCode})
}

// --- Internal endpoints (service-to-service) ---

func (h *Handler) GetPartyByID(c *gin.Context) {
	ctx := c.Request.Context()
	partyID, err := uuid.Parse(c.Param("partyId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid party ID",
		})
		return
	}

	party, err := h.getPartyWithMembers(ctx, partyID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_found",
			Message: "Party not found",
		})
		return
	}

	c.JSON(http.StatusOK, party)
}

func (h *Handler) GetPlayerParty(c *gin.Context) {
	ctx := c.Request.Context()
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "invalid_id",
			Message: "Invalid user ID",
		})
		return
	}

	member, err := h.findMembership(ctx, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "not_in_party",
			Message: "Player is not in a party",
		})
		return
	}

	party, err := h.getPartyWithMembers(ctx, member.PartyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "fetch_failed",
			Message: "Failed to fetch party",
		})
		return
	}

	c.JSON(http.StatusOK, party)
}

// --- Helpers ---

func (h *Handler) disbandParty(c *gin.Context, ctx context.Context, partyID uuid.UUID) {
	err := h.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		_, err := tx.NewDelete().
			Model((*models.PartyMember)(nil)).
			Where("party_id = ?", partyID).
			Exec(ctx)
		if err != nil {
			return err
		}

		_, err = tx.NewDelete().
			Model((*models.Party)(nil)).
			Where("id = ?", partyID).
			Exec(ctx)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "disband_failed",
			Message: "Failed to disband party",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Party disbanded"})
}
