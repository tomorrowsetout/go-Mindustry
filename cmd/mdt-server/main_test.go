package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pierrec/lz4/v4"

	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/persist"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

type testBulletType struct {
	id   int16
	name string
}

func TestValidateBuildVersionOnlyAllows157(t *testing.T) {
	if err := validateBuildVersion(157); err != nil {
		t.Fatalf("expected build 157 to be accepted, got %v", err)
	}
	if err := validateBuildVersion(156); err == nil {
		t.Fatal("expected build 156 to be rejected")
	}
}

func TestBindStatusResolverReturnsFalseWithoutIdentityStore(t *testing.T) {
	resolver := newBindStatusResolver("internal", "", 0, 0, nil)
	if resolver == nil {
		t.Fatal("expected resolver instance")
	}
	if resolver.Bound("conn-uuid") {
		t.Fatal("expected resolver without identity store to report unbound")
	}
}

func TestRuntimePathHelpersKeepWorkspaceRelativeWorldPath(t *testing.T) {
	prevBase := runtimeBaseDir
	base := t.TempDir()
	runtimeBaseDir = base
	t.Cleanup(func() {
		runtimeBaseDir = prevBase
	})

	rel := filepath.Join("assets", "worlds", "maps", "erekir", "origin.msav")
	abs := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir world dir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("msav"), 0o600); err != nil {
		t.Fatalf("write world file: %v", err)
	}

	if got := canonicalRuntimePath(abs); got != rel {
		t.Fatalf("expected workspace-relative path %q, got %q", rel, got)
	}
	if got := resolveRuntimePath(rel); filepath.Clean(got) != filepath.Clean(abs) {
		t.Fatalf("expected resolved path %q, got %q", abs, got)
	}
}

func TestEnsureConnIdentityRecordsCreatesPublicConnUUIDAndIdentityRecord(t *testing.T) {
	prevPublicConnUUIDEnabled := runtimePublicConnUUIDEnabled.Load()
	runtimePublicConnUUIDEnabled.Store(true)
	t.Cleanup(func() {
		runtimePublicConnUUIDEnabled.Store(prevPublicConnUUIDEnabled)
	})

	publicStore, err := persist.NewPublicConnUUIDStore(filepath.Join(t.TempDir(), "conn_uuid.json"), true)
	if err != nil {
		t.Fatalf("new public conn uuid store: %v", err)
	}
	identityStore, err := persist.NewPlayerIdentityStore(filepath.Join(t.TempDir(), "player_identity.json"), true)
	if err != nil {
		t.Fatalf("new player identity store: %v", err)
	}

	connUUID, identityReady := ensureConnIdentityRecords(publicStore, identityStore, "uuid-1", "alpha", "127.0.0.1")
	if strings.TrimSpace(connUUID) == "" {
		t.Fatal("expected non-empty conn_uuid")
	}
	if !identityReady {
		t.Fatal("expected identity record to be ready on first ensure")
	}
	if got := publicConnUUIDValue(publicStore, "uuid-1"); got != connUUID {
		t.Fatalf("expected public conn_uuid lookup %q, got %q", connUUID, got)
	}
	if _, ok := identityStore.Lookup(connUUID); !ok {
		t.Fatalf("expected identity store to contain conn_uuid %q", connUUID)
	}
}

func TestAppendConnectionCheckpointDetailIncludesConnUUIDIdentityAndDisplayState(t *testing.T) {
	prevPublicConnUUIDEnabled := runtimePublicConnUUIDEnabled.Load()
	runtimePublicConnUUIDEnabled.Store(true)
	t.Cleanup(func() {
		runtimePublicConnUUIDEnabled.Store(prevPublicConnUUIDEnabled)
	})

	publicStore, err := persist.NewPublicConnUUIDStore(filepath.Join(t.TempDir(), "conn_uuid.json"), true)
	if err != nil {
		t.Fatalf("new public conn uuid store: %v", err)
	}
	identityStore, err := persist.NewPlayerIdentityStore(filepath.Join(t.TempDir(), "player_identity.json"), true)
	if err != nil {
		t.Fatalf("new player identity store: %v", err)
	}
	connUUID, identityReady := ensureConnIdentityRecords(publicStore, identityStore, "uuid-1", "alpha", "127.0.0.1")
	if strings.TrimSpace(connUUID) == "" || !identityReady {
		t.Fatalf("expected conn_uuid and identity record, got conn_uuid=%q identityReady=%v", connUUID, identityReady)
	}

	detail := appendConnectionCheckpointDetail("version=157", netserver.NetEvent{
		Kind: "connect_packet",
		UUID: "uuid-1",
		Name: "[scarlet]alpha[]",
	}, publicStore, identityStore)
	if !strings.Contains(detail, `conn_uuid_ready=true`) {
		t.Fatalf("expected conn_uuid_ready=true in detail, got %q", detail)
	}
	if !strings.Contains(detail, `identity_ready=true`) {
		t.Fatalf("expected identity_ready=true in detail, got %q", detail)
	}
	if !strings.Contains(detail, `display_name_ready=true`) {
		t.Fatalf("expected display_name_ready=true in detail, got %q", detail)
	}
	if !strings.Contains(detail, connUUID) {
		t.Fatalf("expected detail to include conn_uuid %q, got %q", connUUID, detail)
	}
}

func TestBuildRuntimeRulesRawAvoidsInjectingComplexRuleObjects(t *testing.T) {
	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(8, 8)
	model.Tags = map[string]string{
		"rules": `{"waves":true,"fog":true,"staticFog":true,"defaultTeam":"sharded","waveTeam":"crux","planet":"erekir","teams":{"1":{"infiniteResources":true}}}`,
	}
	w.SetModel(model)

	raw := buildRuntimeRulesRaw(w, filepath.FromSlash("assets/worlds/maps/erekir/aegis.msav"))
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal rules: %v", err)
	}
	if got, _ := decoded["defaultTeam"].(string); got != "sharded" {
		t.Fatalf("expected defaultTeam to remain untouched, got %#v", decoded["defaultTeam"])
	}
	if got, _ := decoded["waveTeam"].(string); got != "crux" {
		t.Fatalf("expected waveTeam to remain untouched, got %#v", decoded["waveTeam"])
	}
	if got, _ := decoded["planet"].(string); got != "erekir" {
		t.Fatalf("expected planet to remain untouched, got %#v", decoded["planet"])
	}
	if got, _ := decoded["fog"].(bool); got {
		t.Fatal("expected runtime rules to disable unsupported fog sync")
	}
	if got, _ := decoded["staticFog"].(bool); got {
		t.Fatal("expected runtime rules to disable unsupported static fog sync")
	}
}

func (t testBulletType) ContentType() protocol.ContentType { return protocol.ContentBullet }
func (t testBulletType) ID() int16                         { return t.id }
func (t testBulletType) Name() string                      { return t.name }

func placeSyncTestBuilding(t *testing.T, model *world.WorldModel, x, y int, block int16, team world.TeamID, rotation int8) int32 {
	t.Helper()
	tile, err := model.TileAt(x, y)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed at (%d,%d): %v", x, y, err)
	}
	tile.Block = world.BlockID(block)
	tile.Team = team
	tile.Rotation = rotation
	tile.Build = &world.Building{
		Block:     world.BlockID(block),
		Team:      team,
		Rotation:  rotation,
		X:         x,
		Y:         y,
		Health:    1000,
		MaxHealth: 1000,
	}
	return protocol.PackPoint2(int32(x), int32(y))
}

func collectRawPacketsUntilTimeout(t *testing.T, conn net.Conn, max int) [][]byte {
	t.Helper()
	out := make([][]byte, 0, max)
	for len(out) < max {
		_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		lenBuf := make([]byte, 2)
		_, err := io.ReadFull(conn, lenBuf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			t.Fatalf("read packet length failed: %v", err)
		}
		size := int(lenBuf[0])<<8 | int(lenBuf[1])
		if size <= 0 {
			t.Fatalf("invalid packet length %d", size)
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(conn, payload); err != nil {
			t.Fatalf("read packet payload failed: %v", err)
		}
		out = append(out, payload)
	}
	return out
}

func decodeFramedPacket(t *testing.T, framed []byte) (byte, []byte) {
	t.Helper()
	r := bytes.NewReader(framed)
	packetID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read packet id failed: %v", err)
	}
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		t.Fatalf("read packet payload length failed: %v", err)
	}
	size := int(lenBuf[0])<<8 | int(lenBuf[1])
	comp, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read packet compression flag failed: %v", err)
	}
	payload := make([]byte, size)
	switch comp {
	case 0:
		if _, err := io.ReadFull(r, payload); err != nil {
			t.Fatalf("read framed payload failed: %v", err)
		}
	case 1:
		compressed := make([]byte, r.Len())
		if _, err := io.ReadFull(r, compressed); err != nil {
			t.Fatalf("read compressed framed payload failed: %v", err)
		}
		if _, err := lz4.UncompressBlock(compressed, payload); err != nil {
			t.Fatalf("decompress framed payload failed: %v", err)
		}
	default:
		t.Fatalf("unexpected compressed packet flag in test: comp=%d", comp)
	}
	return packetID, payload
}

func buildTestMSAV(t *testing.T, version int32) []byte {
	t.Helper()

	var raw bytes.Buffer
	if _, err := raw.WriteString("MSAV"); err != nil {
		t.Fatalf("write msav magic failed: %v", err)
	}
	if err := binary.Write(&raw, binary.BigEndian, version); err != nil {
		t.Fatalf("write msav version failed: %v", err)
	}

	writeChunk := func(chunk []byte) {
		t.Helper()
		if err := binary.Write(&raw, binary.BigEndian, int32(len(chunk))); err != nil {
			t.Fatalf("write msav chunk len failed: %v", err)
		}
		if _, err := raw.Write(chunk); err != nil {
			t.Fatalf("write msav chunk failed: %v", err)
		}
	}

	meta := &bytes.Buffer{}
	if err := binary.Write(meta, binary.BigEndian, int16(0)); err != nil {
		t.Fatalf("write empty meta failed: %v", err)
	}
	writeChunk(meta.Bytes())
	writeChunk(nil)
	if version >= 11 {
		writeChunk(nil)
	}
	writeChunk(nil)
	writeChunk(nil)
	if version >= 8 {
		writeChunk(nil)
	}
	if version >= 7 {
		writeChunk(nil)
	}

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatalf("compress msav failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close msav compressor failed: %v", err)
	}
	return compressed.Bytes()
}

func TestLoadWorldStreamAcceptsLegacyMSAVVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.msav")
	if err := os.WriteFile(path, buildTestMSAV(t, 5), 0o600); err != nil {
		t.Fatalf("write legacy msav failed: %v", err)
	}

	payload, err := loadWorldStream(path, nil)
	if err != nil {
		t.Fatalf("expected legacy msav load to succeed, got %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected legacy msav world stream payload")
	}
}

func TestBuildInitialWorldDataPayloadPrefersLiveModelSnapshotForInitialJoin(t *testing.T) {
	path := filepath.Join("assets", "worlds", "maps", "serpulo", "hidden", "138.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("map 138 not present in workspace")
		}
		t.Fatalf("stat map 138 failed: %v", err)
	}

	model, err := worldstream.LoadWorldModelFromMSAV(path, nil)
	if err != nil {
		t.Fatalf("load world model: %v", err)
	}
	w := world.New(world.Config{TPS: 60})
	w.SetModel(model)

	liveModel := w.CloneModelForWorldStream()
	if liveModel == nil {
		t.Fatal("expected live model clone")
	}
	expectedPayload, err := worldstream.BuildWorldStreamFromModelSnapshot(liveModel, 1, w.Snapshot())
	if err != nil {
		t.Fatalf("build expected live world payload: %v", err)
	}

	cache := &worldCache{}
	basePayload, err := cache.get(path)
	if err != nil {
		t.Fatalf("cache get base payload: %v", err)
	}
	srv := netserver.NewServer("127.0.0.1:0", 157)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := netserver.NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.SetLiveWorldStream(true)

	payload, err := buildInitialWorldDataPayload(conn, w, cache, path)
	if err != nil {
		t.Fatalf("build initial world payload: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected non-empty initial world payload")
	}
	if !conn.UsesLiveWorldStream() {
		t.Fatal("expected initial join live snapshot path to enable live world stream mode")
	}
	if bytes.Equal(payload, basePayload) {
		t.Fatal("expected initial world payload to use live world snapshot instead of base msav cache")
	}
	if !bytes.Equal(payload, expectedPayload) {
		t.Fatal("expected initial world payload to use live world snapshot bytes")
	}
}

func TestBuildInitialWorldDataPayloadFallsBackToBaseWorldStreamWithoutWorld(t *testing.T) {
	path := filepath.Join("assets", "worlds", "maps", "erekir", "origin.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("origin map not present in workspace")
		}
		t.Fatalf("stat origin map failed: %v", err)
	}

	cache := &worldCache{}
	basePayload, err := cache.get(path)
	if err != nil {
		t.Fatalf("cache get base payload: %v", err)
	}

	srv := netserver.NewServer("127.0.0.1:0", 157)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	conn := netserver.NewConn(serverSide, srv.Serial)
	defer conn.Close()

	payload, err := buildInitialWorldDataPayload(conn, nil, cache, path)
	if err != nil {
		t.Fatalf("build initial world payload: %v", err)
	}
	if conn.UsesLiveWorldStream() {
		t.Fatal("expected default initial connect path not to enable live world stream")
	}
	if bytes.Equal(payload, []byte{}) {
		t.Fatal("expected non-empty initial world payload")
	}
	if !bytes.Equal(payload, basePayload) {
		t.Fatal("expected nil-world initial connect payload to use base cached worldstream")
	}
}

func TestBuilderSnapshotActiveTreatsQueuedPlansAsActive(t *testing.T) {
	plans := []*protocol.BuildPlan{{
		X:      4,
		Y:      6,
		Block:  protocol.BlockRef{BlkID: 45, BlkName: "router"},
		Config: int32(1),
	}}
	if !builderSnapshotActive(false, 99, false, plans, false) {
		t.Fatalf("expected queued plans to keep builder active even when building flag is false")
	}
	if builderSnapshotActive(true, 99, true, plans, true) {
		t.Fatalf("expected dead builder to stay inactive")
	}
	if builderSnapshotActive(false, 0, true, plans, true) {
		t.Fatalf("expected missing unit id to keep builder inactive")
	}
}

func TestAuthoritativeTileStatePacketsForPackedLiveBuilding(t *testing.T) {
	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		322: "container",
	}
	pos := placeSyncTestBuilding(t, model, 3, 4, 322, 1, 2)
	tile, err := model.TileAt(3, 4)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	tile.Build.Health = 87.5
	tile.Build.MaxHealth = 90
	w.SetModel(model)

	packets := authoritativeTileStatePacketsForPacked(w, pos)
	if len(packets) < 4 {
		t.Fatalf("expected authoritative live state packets, got %d", len(packets))
	}

	constructFinish, ok := packets[0].(*protocol.Remote_ConstructBlock_constructFinish_146)
	if !ok {
		t.Fatalf("expected first packet constructFinish, got %T", packets[0])
	}
	if got := constructFinish.Block.ID(); got != 322 {
		t.Fatalf("expected constructFinish block=322, got %d", got)
	}
	if got := constructFinish.Team.ID; got != 1 {
		t.Fatalf("expected constructFinish team=1, got %d", got)
	}

	setTile, ok := packets[1].(*protocol.Remote_Tile_setTile_140)
	if !ok {
		t.Fatalf("expected second packet setTile, got %T", packets[1])
	}
	if got := setTile.Block.ID(); got != 322 {
		t.Fatalf("expected setTile block=322, got %d", got)
	}
	if got := setTile.Team.ID; got != 1 {
		t.Fatalf("expected setTile team=1, got %d", got)
	}

	health, ok := packets[2].(*protocol.Remote_Tile_buildHealthUpdate_144)
	if !ok {
		t.Fatalf("expected third packet buildHealthUpdate, got %T", packets[2])
	}
	if len(health.Buildings.Items) != 2 {
		t.Fatalf("expected one health pair, got %v", health.Buildings.Items)
	}
	if health.Buildings.Items[0] != pos {
		t.Fatalf("expected health packet pos=%d, got %d", pos, health.Buildings.Items[0])
	}
	if got := math.Float32frombits(uint32(health.Buildings.Items[1])); math.Abs(float64(got-87.5)) > 0.0001 {
		t.Fatalf("expected health=87.5, got %f", got)
	}

	last := packets[len(packets)-1]
	if _, ok := last.(*protocol.Remote_NetClient_blockSnapshot_34); !ok {
		t.Fatalf("expected final packet blockSnapshot, got %T", last)
	}
}

func TestAuthoritativeTileStatePacketsForPackedEmptyTile(t *testing.T) {
	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(8, 8)
	w.SetModel(model)
	pos := protocol.PackPoint2(2, 5)

	packets := authoritativeTileStatePacketsForPacked(w, pos)
	if len(packets) != 3 {
		t.Fatalf("expected empty tile clear packets, got %d", len(packets))
	}

	if _, ok := packets[0].(*protocol.Remote_Tile_buildDestroyed_143); !ok {
		t.Fatalf("expected first packet buildDestroyed, got %T", packets[0])
	}
	setTile, ok := packets[1].(*protocol.Remote_Tile_setTile_140)
	if !ok {
		t.Fatalf("expected second packet setTile, got %T", packets[1])
	}
	if got := setTile.Block.ID(); got != 0 {
		t.Fatalf("expected clear setTile block=0, got %d", got)
	}
	if _, ok := packets[2].(*protocol.Remote_ConstructBlock_deconstructFinish_145); !ok {
		t.Fatalf("expected third packet deconstructFinish, got %T", packets[2])
	}
}

func TestSharedTeamItemStatePacketsForPackedDisabled(t *testing.T) {
	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		431: "vault",
	}

	coreTile, err := model.TileAt(5, 5)
	if err != nil || coreTile == nil {
		t.Fatalf("core tile lookup failed: %v", err)
	}
	coreTile.Block = 339
	coreTile.Team = 1
	coreTile.Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         5,
		Y:         5,
		Health:    1000,
		MaxHealth: 1000,
		Items: []world.ItemStack{
			{Item: 0, Amount: 12},
			{Item: 1, Amount: 7},
		},
	}

	storeTile, err := model.TileAt(8, 5)
	if err != nil || storeTile == nil {
		t.Fatalf("storage tile lookup failed: %v", err)
	}
	storeTile.Block = 431
	storeTile.Team = 1
	storeTile.Build = &world.Building{
		Block:     431,
		Team:      1,
		X:         8,
		Y:         5,
		Health:    1000,
		MaxHealth: 1000,
	}

	w.SetModel(model)
	pos := protocol.PackPoint2(8, 5)
	packets := sharedTeamItemStatePacketsForPacked(w, pos)
	if len(packets) != 0 {
		t.Fatalf("expected shared team item packets to stay disabled, got %d", len(packets))
	}
}

func TestBuildCoreSnapshotDataEncodesCoreInventory(t *testing.T) {
	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}

	coreTile, err := model.TileAt(5, 5)
	if err != nil || coreTile == nil {
		t.Fatalf("core tile lookup failed: %v", err)
	}
	coreTile.Block = 339
	coreTile.Team = 1
	coreTile.Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         5,
		Y:         5,
		Health:    1000,
		MaxHealth: 1000,
		Items: []world.ItemStack{
			{Item: 0, Amount: 12},
			{Item: 1, Amount: 7},
		},
	}
	w.SetModel(model)

	data := buildCoreSnapshotData(w)
	r := protocol.NewReader(data)

	teams, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read teams failed: %v", err)
	}
	if teams != 1 {
		t.Fatalf("expected 1 team snapshot, got %d", teams)
	}
	teamID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read team id failed: %v", err)
	}
	if teamID != 1 {
		t.Fatalf("expected team id 1, got %d", teamID)
	}
	itemCount, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read item count failed: %v", err)
	}
	if itemCount != 2 {
		t.Fatalf("expected 2 items, got %d", itemCount)
	}
	firstItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read first item id failed: %v", err)
	}
	firstAmount, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read first item amount failed: %v", err)
	}
	secondItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read second item id failed: %v", err)
	}
	secondAmount, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read second item amount failed: %v", err)
	}
	if firstItem != 0 || firstAmount != 12 {
		t.Fatalf("expected first stack (0,12), got (%d,%d)", firstItem, firstAmount)
	}
	if secondItem != 1 || secondAmount != 7 {
		t.Fatalf("expected second stack (1,7), got (%d,%d)", secondItem, secondAmount)
	}
}

func TestSyncWorldDiffToConnFinishesConstructTileBeforeSetTile(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		return protocol.BuildingBox{PosValue: pos}
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	liveModel := world.NewWorldModel(16, 16)
	liveModel.BlockNames = map[int16]string{
		345: "container",
	}
	targetPos := placeSyncTestBuilding(t, liveModel, 7, 6, 345, 1, 0)
	w.SetModel(liveModel)

	baseModel := world.NewWorldModel(16, 16)
	baseModel.BlockNames = map[int16]string{
		6:   "build2",
		345: "container",
	}
	baseTile, err := baseModel.TileAt(7, 6)
	if err != nil || baseTile == nil {
		t.Fatalf("base tile lookup failed: %v", err)
	}
	baseTile.Block = 6
	baseTile.Team = 1
	baseTile.Rotation = 0
	baseTile.Build = &world.Building{
		Block:     6,
		Team:      1,
		Rotation:  0,
		X:         7,
		Y:         6,
		Health:    10,
		MaxHealth: 10,
	}

	syncWorldDiffToConn(sendConn, w, baseModel)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 12)

	constructFinishID, ok := srv.Registry.PacketID(&protocol.Remote_ConstructBlock_constructFinish_146{})
	if !ok {
		t.Fatal("resolve constructFinish packet id")
	}
	setTileID, ok := srv.Registry.PacketID(&protocol.Remote_Tile_setTile_140{})
	if !ok {
		t.Fatal("resolve setTile packet id")
	}

	constructIdx := -1
	setTileIdx := -1
	descriptions := make([]string, 0, len(packets))
	for idx, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		switch packetID {
		case constructFinishID:
			typed := &protocol.Remote_ConstructBlock_constructFinish_146{}
			r := protocol.NewReaderWithContext(payload, srv.TypeIO)
			if err := typed.Read(r, 0); err != nil {
				t.Fatalf("decode constructFinish failed: %v", err)
			}
			pos := int32(-1)
			if typed.Tile != nil {
				pos = typed.Tile.Pos()
			}
			descriptions = append(descriptions, fmt.Sprintf("constructFinish(pos=%d)", pos))
			if pos == targetPos && constructIdx < 0 {
				constructIdx = idx
			}
		case setTileID:
			typed := &protocol.Remote_Tile_setTile_140{}
			r := protocol.NewReaderWithContext(payload, srv.TypeIO)
			if err := typed.Read(r, 0); err != nil {
				t.Fatalf("decode setTile failed: %v", err)
			}
			pos := int32(-1)
			if typed.Tile != nil {
				pos = typed.Tile.Pos()
			}
			descriptions = append(descriptions, fmt.Sprintf("setTile(pos=%d)", pos))
			if pos == targetPos && setTileIdx < 0 {
				setTileIdx = idx
			}
		default:
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
		}
	}

	if constructIdx < 0 {
		t.Fatalf("expected diff sync to emit constructFinish for construct tile transition, got packets: %s", strings.Join(descriptions, ", "))
	}
	if setTileIdx < 0 {
		t.Fatalf("expected diff sync to emit setTile for construct tile transition, got packets: %s", strings.Join(descriptions, ", "))
	}
	if constructIdx > setTileIdx {
		t.Fatalf("expected constructFinish before setTile, got packets: %s", strings.Join(descriptions, ", "))
	}
}

func TestSyncCurrentWorldToConnIncludesBuildingConfigs(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		return protocol.BuildingBox{PosValue: pos}
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	nodePos := placeSyncTestBuilding(t, model, 6, 10, 425, 1, 0)
	targetPos := placeSyncTestBuilding(t, model, 12, 10, 430, 1, 0)
	w.SetModel(model)
	w.ConfigureBuildingPacked(nodePos, targetPos)

	syncCurrentWorldToConn(sendConn, w)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 8)
	setTileID, ok := srv.Registry.PacketID(&protocol.Remote_Tile_setTile_140{})
	if !ok {
		t.Fatal("resolve setTile packet id")
	}
	tileConfigID, ok := srv.Registry.PacketID(&protocol.Remote_InputHandler_tileConfig_90{})
	if !ok {
		t.Fatal("resolve tileConfig packet id")
	}

	var foundNodeTile bool
	var foundNodeConfig bool
	descriptions := make([]string, 0, len(packets))
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		switch packetID {
		case setTileID:
			typed := &protocol.Remote_Tile_setTile_140{}
			r := protocol.NewReaderWithContext(payload, srv.TypeIO)
			if err := typed.Read(r, 0); err != nil {
				t.Fatalf("decode setTile failed: %v", err)
			}
			tilePos := int32(-1)
			if typed.Tile != nil {
				tilePos = typed.Tile.Pos()
			}
			descriptions = append(descriptions, fmt.Sprintf("setTile(pos=%d)", tilePos))
			if typed.Tile != nil && typed.Tile.Pos() == nodePos {
				foundNodeTile = true
			}
		case tileConfigID:
			r := protocol.NewReaderWithContext(payload, srv.TypeIO)
			if _, err := protocol.ReadEntity(r, srv.TypeIO); err != nil {
				t.Fatalf("decode tileConfig player failed: %v", err)
			}
			build, err := protocol.ReadBuilding(r, srv.TypeIO)
			if err != nil {
				t.Fatalf("decode tileConfig building failed: %v", err)
			}
			value, err := protocol.ReadObject(r, false, srv.TypeIO)
			if err != nil {
				t.Fatalf("decode tileConfig value failed: %v", err)
			}
			buildPos := int32(-1)
			if build != nil {
				buildPos = build.Pos()
			}
			descriptions = append(descriptions, fmt.Sprintf("tileConfig(pos=%d,value=%T)", buildPos, value))
			if build == nil || build.Pos() != nodePos {
				continue
			}
			links, ok := value.([]protocol.Point2)
			if !ok {
				t.Fatalf("expected power-node config payload as []Point2, got %T", value)
			}
			if len(links) != 1 {
				t.Fatalf("expected one power-node link, got %d", len(links))
			}
			target := protocol.UnpackPoint2(targetPos)
			node := protocol.UnpackPoint2(nodePos)
			want := protocol.Point2{X: target.X - node.X, Y: target.Y - node.Y}
			if links[0] != want {
				t.Fatalf("expected power-node link %+v, got %+v", want, links[0])
			}
			foundNodeConfig = true
		default:
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
		}
	}

	if !foundNodeTile {
		t.Fatalf("expected syncCurrentWorldToConn to send setTile for the power node, got packets: %s", strings.Join(descriptions, ", "))
	}
	if !foundNodeConfig {
		t.Fatalf("expected syncCurrentWorldToConn to send tileConfig for the power node, got packets: %s", strings.Join(descriptions, ", "))
	}
}

func TestSyncCurrentWorldToConnSendsBlockSnapshotsBeforeTileConfig(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		return protocol.BuildingBox{PosValue: pos}
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		422: "power-node",
		421: "battery",
		500: "container",
	}
	nodePos := placeSyncTestBuilding(t, model, 8, 10, 422, 1, 0)
	_ = placeSyncTestBuilding(t, model, 14, 10, 421, 1, 0)
	storePos := placeSyncTestBuilding(t, model, 9, 8, 500, 1, 0)
	storeTile, err := model.TileAt(9, 8)
	if err != nil || storeTile == nil || storeTile.Build == nil {
		t.Fatalf("container tile lookup failed: %v", err)
	}
	storeTile.Build.Items = []world.ItemStack{{Item: 0, Amount: 37}}
	w.SetModel(model)
	w.ConfigureBuildingPacked(nodePos, protocol.Point2{X: 6, Y: 0})

	syncCurrentWorldToConn(sendConn, w)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 24)
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_34{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}
	tileConfigID, ok := srv.Registry.PacketID(&protocol.Remote_InputHandler_tileConfig_90{})
	if !ok {
		t.Fatal("resolve tileConfig packet id")
	}

	firstBlockSnapshot := -1
	firstTileConfig := -1
	foundStoreSnapshot := false
	for idx, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		switch packetID {
		case blockSnapshotID:
			if firstBlockSnapshot < 0 {
				firstBlockSnapshot = idx
			}
			typed := &protocol.Remote_NetClient_blockSnapshot_34{}
			if err := typed.Read(protocol.NewReader(payload), 0); err != nil {
				t.Fatalf("decode blockSnapshot failed: %v", err)
			}
			r := protocol.NewReader(typed.Data)
			pos, err := r.ReadInt32()
			if err != nil {
				t.Fatalf("read blockSnapshot pos failed: %v", err)
			}
			if pos == storePos {
				foundStoreSnapshot = true
			}
		case tileConfigID:
			if firstTileConfig < 0 {
				firstTileConfig = idx
			}
		}
	}

	if !foundStoreSnapshot {
		t.Fatalf("expected syncCurrentWorldToConn to emit container blockSnapshot before configs")
	}
	if firstBlockSnapshot < 0 || firstTileConfig < 0 {
		t.Fatalf("expected both blockSnapshot and tileConfig packets, got block=%d config=%d", firstBlockSnapshot, firstTileConfig)
	}
	if firstBlockSnapshot > firstTileConfig {
		t.Fatalf("expected blockSnapshot before tileConfig, got block=%d config=%d", firstBlockSnapshot, firstTileConfig)
	}
}

func TestSyncAuthoritativeWorldToConnUsesLiveWorldStreamRuntimeOnly(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		return protocol.BuildingBox{PosValue: pos}
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()
	sendConn.SetLiveWorldStream(true)

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		422: "power-node",
		421: "battery",
		500: "container",
	}
	nodePos := placeSyncTestBuilding(t, model, 8, 10, 422, 1, 0)
	_ = placeSyncTestBuilding(t, model, 14, 10, 421, 1, 0)
	_ = placeSyncTestBuilding(t, model, 9, 8, 500, 1, 0)
	w.SetModel(model)
	w.ConfigureBuildingPacked(nodePos, protocol.Point2{X: 6, Y: 0})

	syncAuthoritativeWorldToConn(nil, sendConn, w, nil, config.AuthoritySyncOfficial)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 24)

	setTileID, ok := srv.Registry.PacketID(&protocol.Remote_Tile_setTile_140{})
	if !ok {
		t.Fatal("resolve setTile packet id")
	}
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_34{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}
	tileConfigID, ok := srv.Registry.PacketID(&protocol.Remote_InputHandler_tileConfig_90{})
	if !ok {
		t.Fatal("resolve tileConfig packet id")
	}

	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, _ := decodeFramedPacket(t, packet)
		if packetID == setTileID {
			t.Fatalf("expected live-world authoritative sync to avoid full setTile replay")
		}
		if packetID == blockSnapshotID {
			t.Fatalf("expected live-world authoritative sync to avoid extra blockSnapshot replay after live world stream")
		}
		if packetID == tileConfigID {
			t.Fatalf("expected live-world authoritative sync to avoid extra tileConfig replay after live world stream")
		}
	}
}

func TestSyncAuthoritativeWorldToConnWithBaseModelStillReplaysRuntimeBlockSnapshots(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		return protocol.BuildingBox{PosValue: pos}
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	nodePos := placeSyncTestBuilding(t, model, 6, 10, 425, 1, 0)
	targetPos := placeSyncTestBuilding(t, model, 12, 10, 430, 1, 0)
	w.SetModel(model)
	w.ConfigureBuildingPacked(nodePos, targetPos)
	baseModel := w.CloneModel()
	if baseModel == nil {
		t.Fatal("expected base model clone")
	}

	syncAuthoritativeWorldToConn(nil, sendConn, w, baseModel, config.AuthoritySyncDynamic)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 8)
	setTileID, ok := srv.Registry.PacketID(&protocol.Remote_Tile_setTile_140{})
	if !ok {
		t.Fatal("resolve setTile packet id")
	}
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_34{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}
	foundBlockSnapshot := false
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, _ := decodeFramedPacket(t, packet)
		if packetID == setTileID {
			t.Fatalf("expected matching-base dynamic sync to avoid full setTile replay")
		}
		if packetID == blockSnapshotID {
			foundBlockSnapshot = true
		}
	}
	if !foundBlockSnapshot {
		t.Fatal("expected matching-base dynamic sync to replay runtime blockSnapshot state")
	}
}

func TestSyncCurrentWorldToConnDoesNotSendSetTileItems(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		339: "core-shard",
		431: "vault",
	}
	corePos := placeSyncTestBuilding(t, model, 5, 5, 339, 1, 0)
	storePos := placeSyncTestBuilding(t, model, 8, 5, 431, 1, 0)
	coreTile, err := model.TileAt(5, 5)
	if err != nil || coreTile == nil || coreTile.Build == nil {
		t.Fatalf("core tile lookup failed: %v", err)
	}
	storeTile, err := model.TileAt(8, 5)
	if err != nil || storeTile == nil || storeTile.Build == nil {
		t.Fatalf("storage tile lookup failed: %v", err)
	}
	coreTile.Build.Items = []world.ItemStack{{Item: 0, Amount: 12}}
	storeTile.Build.Items = []world.ItemStack{{Item: 0, Amount: 3}}
	w.SetModel(model)

	syncCurrentWorldToConn(sendConn, w)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 16)
	setTileItemsID, ok := srv.Registry.PacketID(&protocol.Remote_InputHandler_setTileItems_65{})
	if !ok {
		t.Fatal("resolve setTileItems packet id")
	}

	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, _ := decodeFramedPacket(t, packet)
		if packetID == setTileItemsID {
			t.Fatalf("expected syncCurrentWorldToConn to avoid setTileItems for core/storage sync, core=%d storage=%d", corePos, storePos)
		}
	}
}

func TestSendBlockSnapshotsToConnEmitsBlockSnapshotPayload(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_34{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		500: "container",
	}
	containerPos := placeSyncTestBuilding(t, model, 9, 8, 500, 1, 0)
	tile, err := model.TileAt(9, 8)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("container tile lookup failed: %v", err)
	}
	tile.Build.Items = []world.ItemStack{{Item: 0, Amount: 37}}
	w.SetModel(model)

	sendBlockSnapshotsToConn(sendConn, w)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 4)

	foundSnapshot := false
	descriptions := make([]string, 0, len(packets))
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		if packetID != blockSnapshotID {
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
			continue
		}
		typed := &protocol.Remote_NetClient_blockSnapshot_34{}
		if err := typed.Read(protocol.NewReader(payload), 0); err != nil {
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d(decode-failed)", packetID))
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("packetID=%d blockSnapshot(amount=%d)", packetID, typed.Amount))
		r := protocol.NewReader(typed.Data)
		for i := 0; i < int(typed.Amount); i++ {
			pos, err := r.ReadInt32()
			if err != nil {
				t.Fatalf("read blockSnapshot pos failed: %v", err)
			}
			blockID, err := r.ReadInt16()
			if err != nil {
				t.Fatalf("read blockSnapshot block failed: %v", err)
			}
			if pos == containerPos && blockID == 500 {
				foundSnapshot = true
				break
			}
			if _, err := protocol.ReadBytes(r); err != nil {
				t.Fatalf("read blockSnapshot payload failed: %v", err)
			}
		}
	}

	if !foundSnapshot {
		t.Fatalf("expected sendBlockSnapshotsToConn to emit blockSnapshot payload for synced container, got packets: %s", strings.Join(descriptions, ", "))
	}
}

func TestSendRequestedBlockSnapshotToConnEmitsOnlyBlockSnapshotPayload(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_34{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		500: "container",
	}
	containerPos := placeSyncTestBuilding(t, model, 9, 8, 500, 1, 0)
	tile, err := model.TileAt(9, 8)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("container tile lookup failed: %v", err)
	}
	tile.Build.Items = []world.ItemStack{{Item: 0, Amount: 37}}
	w.SetModel(model)

	sendRequestedBlockSnapshotToConn(sendConn, w, containerPos)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 4)
	if len(packets) != 1 {
		descriptions := make([]string, 0, len(packets))
		for _, packet := range packets {
			if len(packet) == 0 {
				continue
			}
			packetID, _ := decodeFramedPacket(t, packet)
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
		}
		t.Fatalf("expected exactly one request blockSnapshot packet, got %d: %s", len(packets), strings.Join(descriptions, ", "))
	}

	packetID, payload := decodeFramedPacket(t, packets[0])
	if packetID != blockSnapshotID {
		t.Fatalf("expected requestBlockSnapshot to send only blockSnapshot, got packetID=%d", packetID)
	}
	typed := &protocol.Remote_NetClient_blockSnapshot_34{}
	if err := typed.Read(protocol.NewReader(payload), 0); err != nil {
		t.Fatalf("decode blockSnapshot failed: %v", err)
	}
	if typed.Amount != 1 {
		t.Fatalf("expected one requested block snapshot, got %d", typed.Amount)
	}
	r := protocol.NewReader(typed.Data)
	pos, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read requested blockSnapshot pos failed: %v", err)
	}
	blockID, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read requested blockSnapshot block failed: %v", err)
	}
	if pos != containerPos || blockID != 500 {
		t.Fatalf("expected requested container snapshot pos=%d block=500, got pos=%d block=%d", containerPos, pos, blockID)
	}
}

func TestBuildBlockSnapshotPacketsIsolatesSnapshots(t *testing.T) {
	packets := buildBlockSnapshotPackets([]world.BlockSyncSnapshot{
		{Pos: protocol.PackPoint2(2, 3), BlockID: 500, Data: []byte{1, 2, 3}},
		{Pos: protocol.PackPoint2(4, 5), BlockID: 910, Data: []byte{4, 5, 6}},
	})
	if len(packets) != 2 {
		t.Fatalf("expected two isolated blockSnapshot packets, got %d", len(packets))
	}
	for i, packet := range packets {
		if packet == nil {
			t.Fatalf("expected packet %d to be non-nil", i)
		}
		if packet.Amount != 1 {
			t.Fatalf("expected packet %d amount 1, got %d", i, packet.Amount)
		}
	}
}

func TestSyncWorldDiffToConnWithMatchingBaseStillReplaysRuntimeBlockSnapshots(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	nodePos := placeSyncTestBuilding(t, model, 6, 10, 425, 1, 0)
	targetPos := placeSyncTestBuilding(t, model, 12, 10, 430, 1, 0)
	w.SetModel(model)
	w.ConfigureBuildingPacked(nodePos, targetPos)
	baseModel := w.CloneModel()
	if baseModel == nil {
		t.Fatal("expected clone model for diff sync test")
	}

	syncWorldDiffToConn(sendConn, w, baseModel)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 8)
	setTileID, ok := srv.Registry.PacketID(&protocol.Remote_Tile_setTile_140{})
	if !ok {
		t.Fatal("resolve setTile packet id")
	}
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_34{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}
	foundBlockSnapshot := false
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, _ := decodeFramedPacket(t, packet)
		if packetID == setTileID {
			t.Fatalf("expected unchanged base/live diff sync to avoid structural setTile replay")
		}
		if packetID == blockSnapshotID {
			foundBlockSnapshot = true
		}
	}
	if !foundBlockSnapshot {
		t.Fatal("expected unchanged base/live diff sync to replay runtime blockSnapshot state")
	}
}

func TestSyncWorldDiffToConnReplaysRuntimeStateForUnchangedContainer(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		500: "container",
	}
	containerPos := placeSyncTestBuilding(t, model, 9, 8, 500, 1, 0)
	baseModel := model.Clone()
	if baseModel == nil {
		t.Fatal("expected base model clone")
	}
	tile, err := model.TileAt(9, 8)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("container tile lookup failed: %v", err)
	}
	tile.Build.Items = []world.ItemStack{{Item: 0, Amount: 42}}
	w.SetModel(model)

	syncWorldDiffToConn(sendConn, w, baseModel)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 24)
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_34{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}

	foundContainerSnapshot := false
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		if packetID != blockSnapshotID {
			continue
		}
		typed := &protocol.Remote_NetClient_blockSnapshot_34{}
		if err := typed.Read(protocol.NewReader(payload), 0); err != nil {
			t.Fatalf("decode blockSnapshot failed: %v", err)
		}
		r := protocol.NewReader(typed.Data)
		pos, err := r.ReadInt32()
		if err != nil {
			t.Fatalf("read blockSnapshot pos failed: %v", err)
		}
		if pos == containerPos {
			foundContainerSnapshot = true
			break
		}
	}
	if !foundContainerSnapshot {
		t.Fatal("expected diff sync to replay runtime blockSnapshot for unchanged container inventory")
	}
}

func TestSyncWorldDiffToConnRemovedBuildEmitsBuildDestroyed(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		return protocol.BuildingBox{PosValue: pos}
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	liveModel := world.NewWorldModel(24, 24)
	liveModel.BlockNames = map[int16]string{
		500: "container",
	}
	w.SetModel(liveModel)

	baseModel := world.NewWorldModel(24, 24)
	baseModel.BlockNames = map[int16]string{
		500: "container",
	}
	removedPos := placeSyncTestBuilding(t, baseModel, 9, 8, 500, 1, 0)

	syncWorldDiffToConn(sendConn, w, baseModel)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 8)

	buildDestroyedID, ok := srv.Registry.PacketID(&protocol.Remote_Tile_buildDestroyed_143{})
	if !ok {
		t.Fatal("resolve buildDestroyed packet id")
	}

	foundDestroyed := false
	descriptions := make([]string, 0, len(packets))
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		if packetID != buildDestroyedID {
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
			continue
		}
		typed := &protocol.Remote_Tile_buildDestroyed_143{}
		r := protocol.NewReaderWithContext(payload, srv.TypeIO)
		if err := typed.Read(r, 0); err != nil {
			t.Fatalf("decode buildDestroyed failed: %v", err)
		}
		buildPos := int32(-1)
		if typed.Build != nil {
			buildPos = typed.Build.Pos()
		}
		descriptions = append(descriptions, fmt.Sprintf("buildDestroyed(pos=%d)", buildPos))
		if typed.Build != nil && typed.Build.Pos() == removedPos {
			foundDestroyed = true
		}
	}

	if !foundDestroyed {
		t.Fatalf("expected diff sync to emit buildDestroyed for removed build, got packets: %s", strings.Join(descriptions, ", "))
	}
}

func TestBulletCreatePacketFromEventBuildsCreateBulletPacket(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterBulletType(testBulletType{id: 7, name: "test-bullet"})

	packet := bulletCreatePacketFromEvent(srv, world.BulletEvent{
		Team:      2,
		X:         144.5,
		Y:         96.25,
		Angle:     135,
		Damage:    42,
		BulletTyp: 7,
	})
	if packet == nil {
		t.Fatal("expected createBullet packet")
	}
	if packet.Type == nil || packet.Type.ID() != 7 {
		t.Fatalf("expected bullet type 7, got %#v", packet.Type)
	}
	if packet.Team.ID != 2 {
		t.Fatalf("expected team 2, got %d", packet.Team.ID)
	}
	if packet.X != 144.5 || packet.Y != 96.25 {
		t.Fatalf("unexpected bullet origin (%v,%v)", packet.X, packet.Y)
	}
	if packet.Angle != 135 || packet.Damage != 42 {
		t.Fatalf("unexpected bullet angle/damage angle=%v damage=%v", packet.Angle, packet.Damage)
	}
	if packet.VelocityScl != 1 || packet.LifetimeScl != 1 {
		t.Fatalf("expected default createBullet scales 1/1, got %v/%v", packet.VelocityScl, packet.LifetimeScl)
	}
}
