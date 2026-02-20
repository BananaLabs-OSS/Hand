# Hand

Party system that groups players across game sessions.

Part of [BananaLabs](https://github.com/BananaLabs-OSS).

## What It Does

Hand manages persistent player parties — social groups that survive across game sessions. Players form a party first, then matchmaking, orchestration, and routing treat them as a unit.

Unlike lobbies (temporary, game-specific waiting rooms), parties are platform-level and game-agnostic.

Depends on [BananAuth](https://github.com/bananalabs-oss/bananauth) for identity. Uses shared JWT validation from [Potassium](https://github.com/bananalabs-oss/potassium).

## Endpoints

### Parties (JWT auth)

| Method   | Path               | Body                            | Description                          |
| -------- | ------------------ | ------------------------------- | ------------------------------------ |
| `POST`   | `/parties`         | —                               | Create a party (you become owner)    |
| `GET`    | `/parties/mine`    | —                               | Get your current party               |
| `POST`   | `/parties/join`    | `{ "invite_code": "a3f9b21c" }`| Join via invite code                 |
| `POST`   | `/parties/leave`   | —                               | Leave party (owner leaving disbands) |
| `POST`   | `/parties/kick`    | `{ "account_id": "uuid" }`     | Kick a member (owner only)           |
| `POST`   | `/parties/transfer`| `{ "account_id": "uuid" }`     | Transfer ownership (owner only)      |
| `DELETE` | `/parties`         | —                               | Disband party (owner only)           |
| `POST`   | `/parties/invite`  | —                               | Regenerate invite code (owner only)  |

### Internal (service token)

| Method | Path                                | Description                  |
| ------ | ----------------------------------- | ---------------------------- |
| `GET`  | `/internal/parties/:partyId`        | Get party with members       |
| `GET`  | `/internal/parties/player/:userId`  | Get a player's current party |

### System

| Method | Path      | Description        |
| ------ | --------- | ------------------ |
| `GET`  | `/health` | Service health check |

## Rules

- One party per player at a time
- Owner creates the party, gets an invite code
- Invite codes are short hex strings (e.g. `a3f9b21c`)
- Owner leaving disbands the entire party
- Max party size defaults to 8

## Config

| Env Var         | Default            | Description                                   |
| --------------- | ------------------ | --------------------------------------------- |
| `JWT_SECRET`    | _required_         | Shared JWT signing key (must match BananAuth) |
| `SERVICE_TOKEN` | _required_         | Service-to-service auth token                 |
| `DATABASE_URL`  | `sqlite://hand.db` | SQLite database path                          |
| `HOST`          | `0.0.0.0`          | Server bind address                           |
| `PORT`          | `8003`             | HTTP port                                     |

## Run

```bash
JWT_SECRET=your-secret SERVICE_TOKEN=your-token go run ./cmd/server
```

## Docker

```bash
docker build -t hand .
docker run -p 8003:8003 -e JWT_SECRET=your-secret -e SERVICE_TOKEN=your-token hand
```
