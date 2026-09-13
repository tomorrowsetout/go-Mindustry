#!/usr/bin/env python3
# Generates the 159.7 wire-compatible remotePacketFactories() ordered slice.
import re, sys

SRC = "internal/protocol/remote_packets.go"

# Parse actual Go Remote_ type names -> method part (strip trailing _<digits>)
actual = {}
with open(SRC, encoding="utf-8") as f:
    for line in f:
        m = re.search(r"type (Remote_[A-Za-z0-9_]+)_(\d+) struct", line)
        if m:
            full = m.group(1) + "_" + m.group(2)
            method = m.group(1)  # e.g. Remote_NetClient_kick or Remote_NetClient_kick2
            actual.setdefault(method, []).append(full)

# Official 159.7 ordered remote list: (official_id, logical_name_with_overload_digit, is_new)
# logical_name = Go-style name WITHOUT trailing _NN. Overload digit (2/3) kept in method part.
OFFICIAL = """
6 Remote_WaveSpawner_spawnEffect
7 Remote_Logic_gameOver
8 Remote_Logic_researched
9 Remote_Logic_sectorCapture
10 Remote_Logic_updateGameOver
11 Remote_NetClient_blockSnapshot
12 Remote_NetClient_clearObjectives
13 Remote_NetClient_clientBinaryPacketReliable
14 Remote_NetClient_clientBinaryPacketUnreliable
15 Remote_NetClient_clientPacketReliable
16 Remote_NetClient_clientPacketUnreliable
17 Remote_NetClient_completeObjective
18 Remote_NetClient_connect
19 Remote_NetClient_effect
20 Remote_NetClient_effect2
21 Remote_NetClient_effectReliable
22 Remote_NetClient_entitySnapshot
23 Remote_NetClient_hiddenSnapshot
24 Remote_NetClient_kick
25 Remote_NetClient_kick2
26 Remote_NetClient_ping
27 Remote_NetClient_pingResponse
28 Remote_NetClient_playMusic
29 Remote_NetClient_playerDisconnect
30 Remote_NetClient_sendChatMessage
31 Remote_NetClient_sendMessage
32 Remote_NetClient_sendMessage2
33 Remote_NetClient_setCameraPosition
34 Remote_NetClient_setObjectives
35 Remote_NetClient_setPosition
36 Remote_NetClient_setRule
37 Remote_NetClient_setRules
38 Remote_NetClient_sound
39 Remote_NetClient_soundAt
40 Remote_NetClient_stateSnapshot
41 Remote_NetClient_traceInfo
42 Remote_NetClient_worldDataBegin
43 Remote_NetServer_adminRequest
44 Remote_NetServer_clientLogicDataReliable
45 Remote_NetServer_clientLogicDataUnreliable
46 Remote_NetServer_clientPlanSnapshot
47 Remote_NetServer_clientPlanSnapshotReceived
48 Remote_NetServer_clientSnapshot
49 Remote_NetServer_connectConfirm
50 Remote_NetServer_debugStatusClient
51 Remote_NetServer_debugStatusClientUnreliable
52 Remote_NetServer_requestAssets
53 Remote_NetServer_requestBlockSnapshot
54 Remote_NetServer_requestDebugStatus
55 Remote_NetServer_requestWorld
56 Remote_NetServer_serverBinaryPacketReliable
57 Remote_NetServer_serverBinaryPacketUnreliable
58 Remote_NetServer_serverPacketReliable
59 Remote_NetServer_serverPacketUnreliable
60 Remote_Units_unitCapDeath
61 Remote_Units_unitDeath
62 Remote_Units_unitDespawn
63 Remote_Units_unitDestroy
64 Remote_Units_unitEnvDeath
65 Remote_Units_unitSafeDeath
66 Remote_Units_unitSpawn
67 Remote_BulletType_createBullet
68 Remote_Teams_destroyPayload
69 Remote_InputHandler_buildingControlSelect
70 Remote_InputHandler_clearItems
71 Remote_InputHandler_clearLiquids
72 Remote_InputHandler_commandBuilding
73 Remote_InputHandler_commandUnits
74 Remote_InputHandler_deletePlans
75 Remote_InputHandler_dropItem
76 Remote_InputHandler_payloadDropped
77 Remote_InputHandler_pickedBuildPayload
78 Remote_InputHandler_pickedUnitPayload
79 Remote_InputHandler_pingLocation
80 Remote_InputHandler_removeQueueBlock
81 Remote_InputHandler_requestBuildPayload
82 Remote_InputHandler_requestDropPayload
83 Remote_InputHandler_requestItem
84 Remote_InputHandler_requestUnitPayload
85 Remote_InputHandler_rotateBlock
86 Remote_InputHandler_setItem
87 Remote_InputHandler_setItems
88 Remote_InputHandler_setLiquid
89 Remote_InputHandler_setLiquids
90 Remote_InputHandler_setTileItems
91 Remote_InputHandler_setTileLiquids
92 Remote_InputHandler_setUnitCommand
93 Remote_InputHandler_setUnitStance
94 Remote_InputHandler_takeItems
95 Remote_InputHandler_tileConfig
96 Remote_InputHandler_tileTap
97 Remote_InputHandler_transferInventory
98 Remote_InputHandler_transferItemEffect
99 Remote_InputHandler_transferItemTo
100 Remote_InputHandler_transferItemToUnit
101 Remote_InputHandler_unitBuildingControlSelect
102 Remote_InputHandler_unitClear
103 Remote_InputHandler_unitControl
104 Remote_InputHandler_unitEnteredPayload
105 Remote_LExecutor_createMarker
106 Remote_LExecutor_logicExplosion
107 Remote_LExecutor_removeMarker
108 Remote_LExecutor_setFlag
109 Remote_LExecutor_setMapArea
110 Remote_LExecutor_syncVariable
111 Remote_LExecutor_updateMarker
112 Remote_LExecutor_updateMarkerText
113 Remote_LExecutor_updateMarkerTexture
114 Remote_Weather_createWeather
115 Remote_Menus_announce
116 Remote_Menus_copyToClipboard
117 Remote_Menus_followUpMenu
118 Remote_Menus_hideFollowUpMenu
119 Remote_Menus_hideHudText
120 Remote_Menus_infoMessage
121 Remote_Menus_infoPopup
122 Remote_Menus_infoPopup2
123 Remote_Menus_infoPopupReliable
124 Remote_Menus_infoPopupReliable2
125 Remote_Menus_infoToast
126 Remote_Menus_label
127 Remote_Menus_label2
128 Remote_Menus_label3
129 Remote_Menus_labelReliable
130 Remote_Menus_labelReliable2
131 Remote_Menus_labelReliable3
132 Remote_Menus_menu
133 Remote_Menus_menuChoose
134 Remote_Menus_openURI
135 Remote_Menus_removeWorldLabel
136 Remote_Menus_setHudText
137 Remote_Menus_setHudTextReliable
138 Remote_Menus_textInput
139 Remote_Menus_textInput2
140 Remote_Menus_textInputResult
141 Remote_Menus_warningToast
142 Remote_HudFragment_setPlayerTeamEditor
143 Remote_Build_beginBreak
144 Remote_Build_beginPlace
145 Remote_Tile_buildDestroyed
146 Remote_Tile_buildHealthUpdate
147 Remote_Tile_removeTile
148 Remote_Tile_setFloor
149 Remote_Tile_setOverlay
150 Remote_Tile_setTeam
151 Remote_Tile_setTeams
152 Remote_Tile_setTile
153 Remote_Tile_setTileBlocks
154 Remote_Tile_setTileFloors
155 Remote_Tile_setTileOverlays
156 Remote_ConstructBlock_constructFinish
157 Remote_ConstructBlock_deconstructFinish
158 Remote_LandingPad_landingPadLanded
159 Remote_AutoDoor_autoDoorToggle
160 Remote_CoreBlock_playerSpawn
161 Remote_UnitAssembler_assemblerDroneSpawned
162 Remote_UnitAssembler_assemblerUnitSpawned
163 Remote_UnitBlock_unitBlockSpawn
164 Remote_UnitCargoLoader_unitTetherBlockSpawned
"""

# Curated overload resolution: official logical name -> actual full type
OVERRIDE = {
    "Remote_NetClient_kick": "Remote_NetClient_kick_22",        # kick(String) id24
    "Remote_NetClient_kick2": "Remote_NetClient_kick_21",       # kick(KickReason) id25
    "Remote_NetClient_effect": "Remote_NetClient_effect_11",    # effect(Effect,...) id19
    "Remote_NetClient_effect2": "Remote_NetClient_effect_12",   # effect(...,Object) id20
    "Remote_NetClient_sendMessage": "Remote_NetClient_sendMessage_14",   # sendMessage(String) id31
    "Remote_NetClient_sendMessage2": "Remote_NetClient_sendMessage_15", # sendMessage(String,String,Player) id32
    "Remote_Menus_infoPopup": "Remote_Menus_infoPopup_118",     # infoPopup(String,float,...) id122
    "Remote_Menus_infoPopup2": "Remote_Menus_infoPopup_120",    # infoPopup(String,String,...) id121
    "Remote_Menus_infoPopupReliable": "Remote_Menus_infoPopupReliable_119",     # id123
    "Remote_Menus_infoPopupReliable2": "Remote_Menus_infoPopupReliable_121",   # id124
    "Remote_Menus_label": "Remote_Menus_label_124",             # label(String,float,float,float) id126
    "Remote_Menus_label2": "Remote_Menus_label_122",            # label(String,int,...) id127
    "Remote_Menus_label3": "Remote_Menus_label3",               # label(...,int flags) id128 [NEW]
    "Remote_Menus_labelReliable": "Remote_Menus_labelReliable_125",     # id129
    "Remote_Menus_labelReliable2": "Remote_Menus_labelReliable_123",    # id130
    "Remote_Menus_labelReliable3": "Remote_Menus_labelReliable3",       # id131 [NEW]
    "Remote_Menus_textInput": "Remote_Menus_textInput_110",    # 6-arg id138
    "Remote_Menus_textInput2": "Remote_Menus_textInput_111",   # 7-arg id139
    "Remote_NetClient_playMusic": "Remote_NetClient_playMusic",       # id28 [NEW]
    "Remote_NetServer_requestAssets": "Remote_NetServer_requestAssets", # id52 [NEW]
    "Remote_NetServer_requestWorld": "Remote_NetServer_requestWorld",   # id55 [NEW]
}

lines = []
unresolved = []
for raw in OFFICIAL.strip().splitlines():
    raw = raw.strip()
    if not raw:
        continue
    oid, name = raw.split()
    oid = int(oid)
    full = OVERRIDE.get(name)
    if full is None:
        # auto-match: name == method part in actual
        if name in actual:
            full = actual[name][0]
        else:
            unresolved.append((oid, name))
            full = "NEW_%s" % name
    lines.append((oid, name, full))

# New remote packets that are client->server pull / server->client music, not yet defined:
# requestWorld(55), requestAssets(52), playMusic(28) are NEW too (not in actual)
for oid, name, full in lines:
    if full.startswith("NEW_"):
        unresolved.append((oid, name))

print("// AUTO-RESOLVED ORDER (official 159.7 wire IDs):")
for oid, name, full in lines:
    print(f"//   {oid:3d}  {name}  ->  {full}")

print("\n// UNRESOLVED (must be defined):")
for oid, name in unresolved:
    print(f"//   {oid:3d}  {name}")

# Emit factory slice
print("\n// ===== generated remotePacketFactories body =====")
out_lines = []
for oid, name, full in lines:
    if full.startswith("NEW_"):
        # placeholder; will be replaced after definition
        print(f'\t\t// TODO define {name} (id {oid})')
        out_lines.append(f'\t\t// TODO define {name} (id {oid})')
    else:
        print(f'\t\tfunc() Packet {{ return &{full}{{}} }},')
        out_lines.append(f'\t\tfunc() Packet {{ return &{full}{{}} }},')

with open("tools/factory_body.txt", "w", encoding="utf-8") as fw:
    fw.write("\n".join(out_lines) + "\n")

print("\nTOTAL remote:", len(lines))
