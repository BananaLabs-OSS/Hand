package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	RoleOwner  = "owner"
	RoleMember = "member"

	DefaultMaxSize = 8
)

type Party struct {
	bun.BaseModel `bun:"table:parties,alias:p"`

	ID         uuid.UUID `bun:"id,pk,type:text"            json:"id"`
	OwnerID    uuid.UUID `bun:"owner_id,notnull,type:text"  json:"owner_id"`
	InviteCode string    `bun:"invite_code,notnull,unique"  json:"invite_code"`
	MaxSize    int       `bun:"max_size,notnull"            json:"max_size"`
	CreatedAt  time.Time `bun:"created_at,nullzero,notnull" json:"created_at"`
	UpdatedAt  time.Time `bun:"updated_at,nullzero,notnull" json:"updated_at"`

	Members []PartyMember `bun:"rel:has-many,join:id=party_id" json:"members,omitempty"`
}

type PartyMember struct {
	bun.BaseModel `bun:"table:party_members,alias:pm"`

	PartyID   uuid.UUID `bun:"party_id,notnull,type:text"   json:"party_id"`
	AccountID uuid.UUID `bun:"account_id,notnull,type:text"  json:"account_id"`
	Role      string    `bun:"role,notnull"                  json:"role"`
	JoinedAt  time.Time `bun:"joined_at,nullzero,notnull"    json:"joined_at"`
}