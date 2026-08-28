package main

import "github.com/SirNiklas9/pulp-engines/party-state-sqlite-cell/partyowner"

const (
	RoleOwner      = partyowner.RoleOwner
	RoleMember     = partyowner.RoleMember
	DefaultMaxSize = partyowner.DefaultMaxSize
)

type Party = partyowner.Party
type PartyMember = partyowner.PartyMember
