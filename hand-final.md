# Hand — Pulp-Cell Final Audit

Date: 2026-04-19
Scope: Exhaustive parity audit of `pulp-cell/` against native `cmd/server/` + `internal/`.

## Verdict

**0 gaps.** Pulp-cell is byte-level behaviorally equivalent to the native Hand service for every endpoint, every error path, every transaction boundary, every response shape. Both targets compile clean.

- Native build: `go build ./...` clean
- Cell build: `GOOS=wasip1 GOARCH=wasm go build -o hand.wasm .` clean

## Hunt Pattern Results

### 1. Party creation transaction
**Match.** Both insert `parties` row, then `party_members` row, inside `db.RunInTx` with `&sql.TxOptions{}` / `&dsql.TxOptions{}`. Order: party → member → return nil (commit).

- Native: `internal/parties/handler.go:94-102`
- Cell: `pulp-cell/handler.go:89-97`

### 2. Join race — tx recheck
**Match.** Both `SELECT COUNT(*)` inside the transaction before insert; return `fmt.Errorf("party_full")` if `count >= MaxSize`. Outer handler unwraps and emits 409.

- Native: `handler.go:194-218`
- Cell: `handler.go:188-212`

### 3. Transfer ownership
**Match.** Three-step sequence inside single tx, identical order:
1. Demote current owner (`role = member`)
2. Promote target (`role = owner`)
3. Update `parties.owner_id` + `updated_at`

- Native: `handler.go:361-387`
- Cell: `handler.go:355-381`

### 4. Disband
**Match.** FK-safe order: delete all `party_members` for party_id, then delete `parties` row, both in single tx.

- Native: `handler.go:522-537`
- Cell: `handler.go:517-531`

### 5. Invite regeneration
**Match.** 4-byte `crypto/rand` → 8-char hex. No TTL (neither has one). Uniqueness enforced by schema's `UNIQUE` constraint on `parties.invite_code`. Collision is possible in both and would surface as 500 `regenerate_failed` — intentional behavioral parity.

- Native: `handler.go:424-461`
- Cell: `handler.go:418-455`

### 6. Endpoint response body JSON shape
**All 10 endpoints match.**

| Endpoint | Success | Failure |
|---|---|---|
| `POST /parties` | 201 Party | ErrorResponse |
| `GET /parties/mine` | 200 Party | ErrorResponse |
| `POST /parties/join` | 200 Party | ErrorResponse |
| `POST /parties/leave` | 200 `{"message":"Left party"}` | ErrorResponse |
| `POST /parties/kick` | 200 `{"message":"Member kicked"}` | ErrorResponse |
| `POST /parties/transfer` | 200 Party | ErrorResponse |
| `DELETE /parties` | 200 `{"message":"Party disbanded"}` | ErrorResponse |
| `POST /parties/invite` | 200 `{"invite_code":"..."}` | ErrorResponse |
| `GET /internal/parties/:partyId` | 200 Party | ErrorResponse |
| `GET /internal/parties/player/:userId` | 200 Party | ErrorResponse |

`gin.H` and `pulpgin.H` are both `map[string]any` → identical JSON wire output. `middleware.ErrorResponse` is byte-identical in both packages (`{"error":"...","message":"..."}` with omitempty on message).

### 7. Auth middleware error responses
**Byte-identical in both packages.**

JWT failures (401):
- `{"error":"missing authorization header"}`
- `{"error":"invalid authorization format, expected: Bearer <token>"}`
- `{"error":"invalid token: <wrapped>"}`
- `{"error":"invalid token claims"}`

Service failures (401):
- `{"error":"missing service token"}`
- `{"error":"invalid service token"}`

Both middlewares parse `Authorization: Bearer <tok>` case-insensitively, both stash `account_id` and `session_id` via `c.Set`, both use `AbortWithStatusJSON`.

### 8. Time-travel fields
**UTC everywhere, same source.**

All `time.Now().UTC()` call sites:
- Native: lines 77 (create — reused for both rows at 83/91), 190 (join), 383 (transfer), 449 (regenerate invite)
- Cell: lines 72 (create — reused at 78/86), 185 (join), 377 (transfer), 443 (regenerate invite)

No `time.Now()` without `.UTC()`. Single `now` variable reused for party + member row on create guarantees `CreatedAt == JoinedAt` for the owner. Same in both.

## Structural Parity

### Schema
Identical. Cell reproduces Bun's auto-migration output verbatim:

```
parties(id TEXT PK, owner_id TEXT NOT NULL, invite_code TEXT UNIQUE NOT NULL,
        max_size INTEGER NOT NULL, created_at TIMESTAMP, updated_at TIMESTAMP)
party_members(party_id TEXT, account_id TEXT, role TEXT, joined_at TIMESTAMP)
UNIQUE INDEX idx_party_members_unique  ON (party_id, account_id)
UNIQUE INDEX idx_party_members_account ON (account_id)
```

The `UNIQUE(account_id)` index is the structural enforcer of "one party per player" — relied on implicitly by `findMembership` returning a single row.

### Routes
All 11 routes (including `/health`) match method, path, middleware chain, and handler:

```
GET  /health                              — no auth
POST /parties                             — JWT
GET  /parties/mine                        — JWT
POST /parties/join                        — JWT
POST /parties/leave                       — JWT
POST /parties/kick                        — JWT
POST /parties/transfer                    — JWT
DELETE /parties                           — JWT
POST /parties/invite                      — JWT
GET  /internal/parties/:partyId           — Service
GET  /internal/parties/player/:userId     — Service
```

### Config
- Native: `JWT_SECRET`, `SERVICE_TOKEN`, `DATABASE_URL`, `HOST`, `PORT` via env
- Cell: `jwt_secret`, `service_token` via `[config]` in `pulp.cell.toml` (MessagePack at runtime). DB path and listen addr are owned by the host, not the cell — correct delegation.

### DB Pool
Cell pins `SetMaxOpenConns(1)` / `SetMaxIdleConns(1)` to match the host single-writer SQLite pool, preventing nested-BEGIN. Native uses Potassium's `database.Connect` which does the same internally.

## Files Audited

Native:
- `C:\Users\Nicholas\GolandProjects\Hand\cmd\server\main.go`
- `C:\Users\Nicholas\GolandProjects\Hand\internal\models\models.go`
- `C:\Users\Nicholas\GolandProjects\Hand\internal\parties\handler.go`
- `C:\Users\Nicholas\GolandProjects\Hand\internal\router\router.go`

Cell:
- `C:\Users\Nicholas\GolandProjects\Hand\pulp-cell\main.go`
- `C:\Users\Nicholas\GolandProjects\Hand\pulp-cell\models.go`
- `C:\Users\Nicholas\GolandProjects\Hand\pulp-cell\handler.go`
- `C:\Users\Nicholas\GolandProjects\Hand\pulp-cell\msgpack.go`
- `C:\Users\Nicholas\GolandProjects\Hand\pulp-cell\pulp.cell.toml`
- `C:\Users\Nicholas\GolandProjects\Hand\pulp-cell\testtools\genjwt\main.go`

Runtime dependencies cross-checked:
- `C:\Users\Nicholas\GolandProjects\Fiber\pulp\gin\context.go`
- `C:\Users\Nicholas\GolandProjects\Fiber\pulp\gin\gin.go`
- `C:\Users\Nicholas\GolandProjects\Fiber\pulp\gin\middleware\jwt.go`
- `C:\Users\Nicholas\GolandProjects\Potassium\middleware\jwt.go`
- `C:\Users\Nicholas\GolandProjects\Potassium\middleware\response.go`

## Fixes Applied

None required. The cell was already at behavioral parity.

## Final Status

- Prior audit: 0 gaps
- This pass: 0 gaps
- Compiled: yes (both targets)
- Ready: ship.
