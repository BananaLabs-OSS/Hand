-- Hand is the compatibility application. The reusable, game-agnostic party
-- owner remains independently callable by any other Pulp application's Lua.
local PARTY = "party-state"

local function require_wire(payload)
  if type(payload) ~= "table" then error("hand: payload must be a table") end
  local raw = payload.request_msgpack
  if type(raw) ~= "string" or raw == "" then
    error("hand: request_msgpack must contain MessagePack bytes")
  end
  return raw
end

local function forward(provider)
  return function(payload)
    return {
      response_msgpack = pulp.call_raw(PARTY, provider, require_wire(payload)),
    }
  end
end

pulp.on("hand.party.create.v1", forward("party.v1.create"))
pulp.on("hand.party.get.v1", forward("party.v1.get"))
pulp.on("hand.party.get-for-player.v1", forward("party.v1.get-for-player"))
pulp.on("hand.party.join.v1", forward("party.v1.join"))
pulp.on("hand.party.leave.v1", forward("party.v1.leave"))
pulp.on("hand.party.kick.v1", forward("party.v1.kick"))
pulp.on("hand.party.transfer.v1", forward("party.v1.transfer"))
pulp.on("hand.party.disband.v1", forward("party.v1.disband"))
pulp.on("hand.party.invite.rotate.v1", forward("party.v1.invite.rotate"))
