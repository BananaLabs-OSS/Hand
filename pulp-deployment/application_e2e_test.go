package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/BananaLabs-OSS/Pulp/run"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

type partyProviderObserver struct {
	ready chan run.ApplicationProviderAccess
}

func (o *partyProviderObserver) AfterApplicationStart(context.Context, run.ApplicationIdentity) error {
	return nil
}

func (o *partyProviderObserver) AfterApplicationStartWithProvider(
	_ context.Context,
	_ run.ApplicationIdentity,
	access run.ApplicationProviderAccess,
) error {
	o.ready <- access
	return nil
}

func (o *partyProviderObserver) BeforeApplicationShutdown(context.Context, run.ApplicationIdentity) error {
	return nil
}

type testPartyCommand struct {
	RequestID  string    `msgpack:"request_id"`
	PartyID    uuid.UUID `msgpack:"party_id,omitempty"`
	OwnerID    uuid.UUID `msgpack:"owner_id"`
	InviteCode string    `msgpack:"invite_code,omitempty"`
	MaxSize    int       `msgpack:"max_size,omitempty"`
	NowUnixMS  int64     `msgpack:"now_unix_ms"`
}

type testPartyQuery struct {
	AccountID uuid.UUID `msgpack:"account_id"`
}

type testParty struct {
	ID      uuid.UUID `msgpack:"id"`
	OwnerID uuid.UUID `msgpack:"owner_id"`
}

type testPartyResult struct {
	Version string          `msgpack:"version"`
	OK      bool            `msgpack:"ok"`
	Value   testParty       `msgpack:"value,omitempty"`
	Error   *testPartyError `msgpack:"error,omitempty"`
}

type testPartyError struct {
	Code    string `msgpack:"code"`
	Message string `msgpack:"message"`
}

type testDispatchRequest struct {
	Event   string         `msgpack:"event"`
	Payload map[string]any `msgpack:"payload,omitempty"`
}

type testDispatchResult struct {
	Value any `msgpack:"value,omitempty"`
}

func TestPartyOwnerRealPulpLuaAndIdempotency(t *testing.T) {
	t.Setenv("HTTP_PORT", "0")
	buildPartyWASM(t)

	storageRoot := t.TempDir()
	actorID := uuid.MustParse("10000000-0000-0000-0000-000000000001")
	partyID := uuid.MustParse("20000000-0000-0000-0000-000000000002")
	command := testPartyCommand{
		RequestID:  "create-party-1",
		PartyID:    partyID,
		OwnerID:    actorID,
		InviteCode: "invite-1",
		MaxSize:    8,
		NowUnixMS:  time.Unix(1_800_000_000, 0).UnixMilli(),
	}

	shutdown := runPartyApplication(t, storageRoot, func(ctx context.Context, access run.ApplicationProviderAccess) {
		result := callPartyProvider[testPartyResult](t, ctx, access, "hand.party.create.v1", command)
		if !result.OK || result.Error != nil || result.Value.ID != partyID {
			t.Fatalf("initial create = %#v", result)
		}
		replayed := callPartyProvider[testPartyResult](t, ctx, access, "hand.party.create.v1", command)
		if !replayed.OK || replayed.Error != nil || replayed.Value.ID != partyID {
			t.Fatalf("idempotent replay = %#v", replayed)
		}
		projected := callPartyProvider[testPartyResult](t, ctx, access, "hand.party.get-for-player.v1", testPartyQuery{
			AccountID: actorID,
		})
		if !projected.OK || projected.Error != nil || projected.Value.OwnerID != actorID {
			t.Fatalf("owner projection = %#v", projected)
		}
		conflict := command
		conflict.InviteCode = "different"
		rejected := callPartyProvider[testPartyResult](t, ctx, access, "hand.party.create.v1", conflict)
		if rejected.Error == nil || rejected.Error.Code != "idempotency_conflict" {
			t.Fatalf("conflicting replay = %#v", rejected)
		}
	})
	shutdown()
}

func buildPartyWASM(t *testing.T) {
	t.Helper()
	cellDir := filepath.Clean(filepath.Join("..", "pulp-cell"))
	output, err := filepath.Abs(filepath.Join(cellDir, "party.wasm"))
	if err != nil {
		t.Fatalf("resolve party WASM output: %v", err)
	}
	command := exec.Command("go", "build", "-trimpath", "-buildmode=c-shared", "-o", output, ".")
	command.Dir = cellDir
	command.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build party WASM: %v\n%s", err, output)
	}

	luaRoot := filepath.Clean(filepath.Join("..", "..", "Pulp-Lua"))
	luaOutput, err := filepath.Abs(filepath.Clean(filepath.Join("..", "application", "lua-orchestrator.wasm")))
	if err != nil {
		t.Fatalf("resolve Pulp-Lua WASM output: %v", err)
	}
	luaCommand := exec.Command("go", "build", "-trimpath", "-buildmode=c-shared", "-o", luaOutput, "./pulp-cell")
	luaCommand.Dir = luaRoot
	luaCommand.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if output, err := luaCommand.CombinedOutput(); err != nil {
		t.Fatalf("build Pulp-Lua WASM: %v\n%s", err, output)
	}
}

func runPartyApplication(
	t *testing.T,
	storageRoot string,
	exercise func(context.Context, run.ApplicationProviderAccess),
) func() {
	t.Helper()
	observer := &partyProviderObserver{ready: make(chan run.ApplicationProviderAccess, 1)}
	appPath := filepath.Clean(filepath.Join("..", "application", "pulp.app.toml"))
	runtime, err := run.NewDirectApplicationRuntime(appPath, run.DirectApplicationOptions{
		StorageRoot: storageRoot,
		Lifecycle:   observer,
	})
	if err != nil {
		t.Fatalf("create Hand application runtime: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := runtime.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start Hand application runtime: %v", err)
	}
	access := <-observer.ready
	exercise(ctx, access)
	cancel()
	return func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := runtime.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("shutdown Hand application runtime: %v", err)
		}
	}
}

func callPartyProvider[T any](
	t *testing.T,
	ctx context.Context,
	access run.ApplicationProviderAccess,
	provider string,
	request any,
) T {
	t.Helper()
	requestWire, err := msgpack.Marshal(request)
	if err != nil {
		t.Fatalf("encode %s request: %v", provider, err)
	}
	dispatchWire, err := msgpack.Marshal(testDispatchRequest{
		Event: provider,
		Payload: map[string]any{
			"request_msgpack": string(requestWire),
		},
	})
	if err != nil {
		t.Fatalf("encode %s dispatch: %v", provider, err)
	}
	response, err := access.CallProvider(ctx, "lua-orchestrator", "orchestrator.dispatch", dispatchWire)
	if err != nil {
		t.Fatalf("dispatch %s: %v", provider, err)
	}
	var dispatched testDispatchResult
	if err := msgpack.Unmarshal(response, &dispatched); err != nil {
		t.Fatalf("decode %s dispatch: %v", provider, err)
	}
	object, ok := dispatched.Value.(map[string]any)
	if !ok {
		t.Fatalf("%s result is %T, want object", provider, dispatched.Value)
	}
	wireValue, ok := object["response_msgpack"]
	if !ok {
		t.Fatalf("%s result lacks response_msgpack", provider)
	}
	var ownerResponse []byte
	switch value := wireValue.(type) {
	case string:
		ownerResponse = []byte(value)
	case []byte:
		ownerResponse = value
	default:
		t.Fatalf("%s response_msgpack is %T", provider, wireValue)
	}
	var decoded T
	if err := msgpack.Unmarshal(ownerResponse, &decoded); err != nil {
		t.Fatalf("decode %s: %v", provider, err)
	}
	return decoded
}
