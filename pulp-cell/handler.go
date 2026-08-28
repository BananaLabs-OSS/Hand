package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	pulpgin "github.com/BananaLabs-OSS/Fiber/pulp/gin"
	"github.com/BananaLabs-OSS/Fiber/pulp/gin/middleware"
	"github.com/BananaLabs-OSS/Fiber/pulp/workflow"
	"github.com/SirNiklas9/pulp-engines/party-state-sqlite-cell/partyowner"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

type Handler struct{ client *workflow.Client }
type partyReply struct {
	ResponseMsgpack []byte `msgpack:"response_msgpack"`
}

func generateInviteCode() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
func getAccountID(c *pulpgin.Context) (uuid.UUID, bool) {
	id, e := uuid.Parse(c.GetString("account_id"))
	if e != nil {
		c.JSON(401, middleware.ErrorResponse{Error: "invalid_account", Message: "Invalid account"})
		return uuid.Nil, false
	}
	return id, true
}
func partyCommandID(c *pulpgin.Context, op string) string {
	return fmt.Sprintf("%s:http-%d", op, c.Request().ID)
}
func (h *Handler) call(event string, request, output any) *partyowner.Error {
	wire, err := msgpack.Marshal(request)
	if err != nil {
		return &partyowner.Error{Code: "unavailable", Message: err.Error()}
	}
	result, err := h.client.Dispatch(workflow.DispatchRequest{Event: event, Payload: map[string]any{"request_msgpack": wire}})
	if err != nil {
		return &partyowner.Error{Code: "unavailable", Message: err.Error()}
	}
	reply, err := workflow.DecodeValue[partyReply](result)
	if err != nil {
		return &partyowner.Error{Code: "unavailable", Message: err.Error()}
	}
	if err = msgpack.Unmarshal(reply.ResponseMsgpack, output); err != nil {
		return &partyowner.Error{Code: "unavailable", Message: err.Error()}
	}
	return nil
}
func partyFailure(c *pulpgin.Context, e *partyowner.Error) {
	status := 409
	if e.Code == "invalid_request" {
		status = 400
	}
	if e.Code == "not_owner" {
		status = 403
	}
	if e.Code == "not_found" || e.Code == "not_in_party" || e.Code == "invalid_code" {
		status = 404
	}
	if e.Code == "unavailable" {
		status = 500
	}
	c.JSON(status, middleware.ErrorResponse{Error: e.Code, Message: e.Message})
}
func partyResult[T any](c *pulpgin.Context, h *Handler, event string, request any, status int) {
	var result partyowner.Result[T]
	if e := h.call(event, request, &result); e != nil {
		partyFailure(c, e)
		return
	}
	if result.Error != nil {
		partyFailure(c, result.Error)
		return
	}
	c.JSON(status, result.Value)
}

func (h *Handler) CreateParty(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	partyResult[partyowner.Party](c, h, "hand.party.create.v1", partyowner.CreateRequest{RequestID: partyCommandID(c, "create"), PartyID: uuid.New(), OwnerID: a, InviteCode: generateInviteCode(), MaxSize: partyowner.DefaultMaxSize, NowUnixMS: time.Now().UTC().UnixMilli()}, 201)
}
func (h *Handler) GetMyParty(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	partyResult[partyowner.Party](c, h, "hand.party.get-for-player.v1", partyowner.GetForPlayerRequest{AccountID: a}, 200)
}
func (h *Handler) JoinParty(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	var in struct {
		InviteCode string `json:"invite_code" binding:"required"`
	}
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(400, middleware.ErrorResponse{Error: "invalid_request", Message: "invite_code is required"})
		return
	}
	partyResult[partyowner.Party](c, h, "hand.party.join.v1", partyowner.JoinRequest{RequestID: partyCommandID(c, "join"), AccountID: a, InviteCode: in.InviteCode, NowUnixMS: time.Now().UTC().UnixMilli()}, 200)
}
func (h *Handler) LeaveParty(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	partyResult[partyowner.Ack](c, h, "hand.party.leave.v1", partyowner.PlayerCommand{RequestID: partyCommandID(c, "leave"), AccountID: a}, 200)
}
func targetInput(c *pulpgin.Context) (uuid.UUID, bool) {
	var in struct {
		AccountID uuid.UUID `json:"account_id" binding:"required"`
	}
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(400, middleware.ErrorResponse{Error: "invalid_request", Message: "account_id is required"})
		return uuid.Nil, false
	}
	return in.AccountID, true
}
func (h *Handler) KickMember(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	target, ok := targetInput(c)
	if !ok {
		return
	}
	partyResult[partyowner.Ack](c, h, "hand.party.kick.v1", partyowner.OwnerTargetCommand{RequestID: partyCommandID(c, "kick"), OwnerID: a, TargetID: target}, 200)
}
func (h *Handler) TransferOwnership(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	target, ok := targetInput(c)
	if !ok {
		return
	}
	partyResult[partyowner.Party](c, h, "hand.party.transfer.v1", partyowner.OwnerTargetCommand{RequestID: partyCommandID(c, "transfer"), OwnerID: a, TargetID: target, NowUnixMS: time.Now().UTC().UnixMilli()}, 200)
}
func (h *Handler) DisbandParty(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	partyResult[partyowner.Ack](c, h, "hand.party.disband.v1", partyowner.OwnerCommand{RequestID: partyCommandID(c, "disband"), OwnerID: a}, 200)
}
func (h *Handler) RegenerateInvite(c *pulpgin.Context) {
	a, ok := getAccountID(c)
	if !ok {
		return
	}
	partyResult[partyowner.Invite](c, h, "hand.party.invite.rotate.v1", partyowner.RotateInviteRequest{RequestID: partyCommandID(c, "invite"), OwnerID: a, InviteCode: generateInviteCode(), NowUnixMS: time.Now().UTC().UnixMilli()}, 200)
}
func (h *Handler) GetPartyByID(c *pulpgin.Context) {
	id, e := uuid.Parse(c.Param("partyId"))
	if e != nil {
		c.JSON(400, middleware.ErrorResponse{Error: "invalid_id", Message: "Invalid party ID"})
		return
	}
	partyResult[partyowner.Party](c, h, "hand.party.get.v1", partyowner.GetRequest{PartyID: id}, http.StatusOK)
}
func (h *Handler) GetPlayerParty(c *pulpgin.Context) {
	id, e := uuid.Parse(c.Param("userId"))
	if e != nil {
		c.JSON(400, middleware.ErrorResponse{Error: "invalid_id", Message: "Invalid user ID"})
		return
	}
	partyResult[partyowner.Party](c, h, "hand.party.get-for-player.v1", partyowner.GetForPlayerRequest{AccountID: id}, 200)
}
