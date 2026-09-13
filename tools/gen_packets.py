#!/usr/bin/env python3
# Regenerates the struct/Read/Write definitions for the 159.7 wire-compatible
# remote packets into internal/protocol/remote_packets.go.
#
# Strategy
# --------
# * The wire order (and thus the set of Go type names) is ALREADY correct in
#   the current remote_packets.go factory body (produced by gen_registry.py
#   from the official 159.7 registration order). We keep that order.
# * For each Go type name we resolve the exact Java @Remote parameter list:
#     - overloaded methods  -> curated OVERLOAD_PARAMS (verified against Java)
#     - all other methods   -> extracted from the 159.7 Java source via grp
# * Each Java parameter type is mapped to the matching protocol Read*/Write*
#   helper, which already implements Mindustry's exact wire format.
# * The 5 hand-written 159.7 packets (playMusic, requestAssets, requestWorld,
#   label3, labelReliable3) already live in remote_packets_159.go and are
#   SKIPPED here so we don't duplicate their definitions.
import re, os

JAVA_ROOT = "C:/Users/43551/Desktop/go-mdt/server/Mindustry-159.7/core/src/mindustry"
SRC = "C:/Users/43551/Desktop/go-mdt/server/go-Mindustry-main/internal/protocol/remote_packets.go"

# ---------------------------------------------------------------------------
# 1. Extract @Remote signatures from 159.7 Java  (cls, method) -> [param-lists]
#    Enclosing class is resolved via brace-depth tracking (handles inner classes
#    such as those in LExecutor.java / Teams.java / UnitAssembler.java).
# ---------------------------------------------------------------------------
import bisect
pat = re.compile(
    r'@Remote(?:\([^)]*\))?\s*(?:public|private|protected)?\s*static\s+(?:void|[\w<>\[\],\s]+?)\s*'
    r'([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)', re.DOTALL)
grp = {}
grp_method = {}  # method -> [param-lists] (fallback when class can't be resolved)
class_open = re.compile(r'\b(?:class|interface|enum)\s+([A-Za-z_][A-Za-z0-9_]*)\b')
for dp, _, fns in os.walk(JAVA_ROOT):
    for fn in fns:
        if not fn.endswith(".java"):
            continue
        txt = open(os.path.join(dp, fn), encoding="utf-8", errors="ignore").read()
        remote_matches = list(pat.finditer(txt))
        if not remote_matches:
            continue
        events = []
        for m in class_open.finditer(txt):
            events.append((m.start(), 'open', m.group(1)))
        for m in re.finditer(r'\}', txt):
            events.append((m.start(), 'close', None))
        events.sort(key=lambda e: e[0])
        stack = []
        snaps = []
        for ev in events:
            if ev[1] == 'open':
                stack.append(ev[2])
            else:
                if stack:
                    stack.pop()
            snaps.append((ev[0], list(stack)))
        poslist = [s[0] for s in snaps]

        def enclosing(pos):
            idx = bisect.bisect_right(poslist, pos) - 1
            if idx < 0:
                return None
            st = snaps[idx][1]
            return st[-1] if st else None

        for m in remote_matches:
            cls = enclosing(m.start())
            method = m.group(1)
            params = m.group(2).strip()
            plist = []
            if params:
                for p in params.split(','):
                    p = p.strip()
                    if not p:
                        continue
                    plist.append(p)
            grp.setdefault((cls, method), []).append(plist)
            grp_method.setdefault(method, []).append(plist)

# ---------------------------------------------------------------------------
# 2. Trusted overload resolution (from gen_registry.py)
# ---------------------------------------------------------------------------
OVERRIDE = {
    "Remote_NetClient_kick": "Remote_NetClient_kick_22",
    "Remote_NetClient_kick2": "Remote_NetClient_kick_21",
    "Remote_NetClient_effect": "Remote_NetClient_effect_11",
    "Remote_NetClient_effect2": "Remote_NetClient_effect_12",
    "Remote_NetClient_sendMessage": "Remote_NetClient_sendMessage_14",
    "Remote_NetClient_sendMessage2": "Remote_NetClient_sendMessage_15",
    "Remote_Menus_infoPopup": "Remote_Menus_infoPopup_118",
    "Remote_Menus_infoPopup2": "Remote_Menus_infoPopup_120",
    "Remote_Menus_infoPopupReliable": "Remote_Menus_infoPopupReliable_119",
    "Remote_Menus_infoPopupReliable2": "Remote_Menus_infoPopupReliable_121",
    "Remote_Menus_label": "Remote_Menus_label_124",
    "Remote_Menus_label2": "Remote_Menus_label_122",
    "Remote_Menus_label3": "Remote_Menus_label3",
    "Remote_Menus_labelReliable": "Remote_Menus_labelReliable_125",
    "Remote_Menus_labelReliable2": "Remote_Menus_labelReliable_123",
    "Remote_Menus_labelReliable3": "Remote_Menus_labelReliable3",
    "Remote_Menus_textInput": "Remote_Menus_textInput_110",
    "Remote_Menus_textInput2": "Remote_Menus_textInput_111",
    "Remote_NetClient_playMusic": "Remote_NetClient_playMusic",
    "Remote_NetServer_requestAssets": "Remote_NetServer_requestAssets",
    "Remote_NetServer_requestWorld": "Remote_NetServer_requestWorld",
}
REV = {v: k for k, v in OVERRIDE.items()}

# Curated exact Java parameter lists for overloaded logical names.
OVERLOAD_PARAMS = {
    "Remote_NetClient_kick": ["String reason"],
    "Remote_NetClient_kick2": ["KickReason reason"],
    "Remote_NetClient_effect": ["Effect effect", "float x", "float y", "float rotation", "Color color"],
    "Remote_NetClient_effect2": ["Effect effect", "float x", "float y", "float rotation", "Color color", "Object data"],
    "Remote_NetClient_sendMessage": ["String message"],
    "Remote_NetClient_sendMessage2": ["String message", "@Nullable String unformatted", "@Nullable Player playersender"],
    "Remote_Menus_infoPopup": ["@Nullable String message", "float duration", "int align", "int top", "int left", "int bottom", "int right"],
    "Remote_Menus_infoPopup2": ["@Nullable String message", "@Nullable String id", "float duration", "int align", "int top", "int left", "int bottom", "int right"],
    "Remote_Menus_infoPopupReliable": ["@Nullable String message", "float duration", "int align", "int top", "int left", "int bottom", "int right"],
    "Remote_Menus_infoPopupReliable2": ["@Nullable String message", "@Nullable String id", "float duration", "int align", "int top", "int left", "int bottom", "int right"],
    "Remote_Menus_label": ["@Nullable String message", "int id", "float duration", "float worldx", "float worldy"],
    "Remote_Menus_label2": ["@Nullable String message", "int id", "float duration", "float worldx", "float worldy", "int flags"],
    "Remote_Menus_labelReliable": ["@Nullable String message", "int id", "float duration", "float worldx", "float worldy"],
    "Remote_Menus_labelReliable2": ["@Nullable String message", "int id", "float duration", "float worldx", "float worldy", "int flags"],
    "Remote_Menus_textInput": ["int textInputId", "String title", "String message", "int textLength", "String def", "boolean numeric"],
    "Remote_Menus_textInput2": ["int textInputId", "String title", "String message", "int textLength", "String def", "boolean numeric", "boolean allowEmpty"],
}

SKIP = {
    "Remote_NetClient_playMusic",
    "Remote_NetServer_requestAssets",
    "Remote_NetServer_requestWorld",
    "Remote_Menus_label3",
    "Remote_Menus_labelReliable3",
}

# ---------------------------------------------------------------------------
# 3. Java type -> (gotype, read_expr, write_expr)  (read/write take field {F})
# ---------------------------------------------------------------------------
def emit(jt, f, i):
    jt0 = jt.replace("@Nullable ", "").strip()
    nullable = jt.strip().startswith("@Nullable")
    var = "v%d" % i  # unique per-field variable declared in function scope

    def sread(expr):
        return ("%s, err := %s\n\tif err != nil {\n\t\treturn err\n\t}\n\tp.%s = %s"
                % (var, expr, f, var))
    def swrite(expr):
        return "if err := %s; err != nil {\n\t\treturn err\n\t}" % expr

    table = {
        "Player": ("Entity", "ReadEntity(r, r.Ctx)", "WriteEntity(w, p.%s)"),
        "Unit": ("Entity", "ReadEntity(r, r.Ctx)", "WriteEntity(w, p.%s)"),
        "Building": ("Entity", "ReadEntity(r, r.Ctx)", "WriteEntity(w, p.%s)"),
        "UnitSyncContainer": ("Entity", "ReadEntity(r, r.Ctx)", "WriteEntity(w, p.%s)"),
        "Block": ("Content", "ReadContent(r, r.Ctx)", "WriteContent(w, p.%s)"),
        "Content": ("Content", "ReadContent(r, r.Ctx)", "WriteContent(w, p.%s)"),
        "Item": ("Item", "ReadItem(r, r.Ctx)", "WriteItem(w, p.%s)"),
        "Liquid": ("Liquid", "ReadLiquid(r, r.Ctx)", "WriteLiquid(w, p.%s)"),
        "UnitType": ("UnitType", "ReadUnitType(r, r.Ctx)", "WriteUnitType(w, p.%s)"),
        "BulletType": ("BulletType", "ReadBulletType(r, r.Ctx)", "WriteBulletType(w, p.%s)"),
        "Team": ("Team", "ReadTeam(r, r.Ctx)", "WriteTeam(w, &p.%s)"),
        "Effect": ("Effect", "ReadEffect(r, r.Ctx)", "WriteEffect(w, p.%s)"),
        "Color": ("Color", "ReadColor(r)", "WriteColor(w, p.%s)"),
        "Sound": ("Sound", "ReadSound(r, r.Ctx)", "WriteSound(w, p.%s)"),
        "Weather": ("Weather", "ReadWeather(r, r.Ctx)", "WriteWeather(w, p.%s)"),
        "TraceInfo": ("TraceInfo", "ReadTraceInfo(r)", "WriteTraceInfo(w, p.%s)"),
        "Rules": ("Rules", "ReadRules(r)", "WriteRules(w, p.%s)"),
        "MapObjectives": ("MapObjectives", "ReadObjectives(r)", "WriteObjectives(w, p.%s)"),
        "ObjectiveMarker": ("ObjectiveMarker", "ReadObjectiveMarker(r)", "WriteObjectiveMarker(w, p.%s)"),
        "AdminAction": ("AdminAction", "ReadAction(r)", "WriteAction(w, p.%s)"),
        "KickReason": ("KickReason", "ReadKick(r)", "WriteKick(w, p.%s)"),
        "LMarkerControl": ("LMarkerControl", "ReadMarkerControl(r)", "WriteMarkerControl(w, p.%s)"),
        "Vec2": ("*Vec2", "ReadVecNullable(r)", "WriteVecNullable(w, p.%s)"),
        "UnitCommand": ("*UnitCommand", "ReadCommand(r, r.Ctx)", "WriteCommand(w, p.%s)"),
        "UnitStance": ("UnitStance", "ReadStance(r, r.Ctx)", "WriteStance(w, &p.%s)"),
        "ClientBuildPlans": ("[]*BuildPlan", "ReadClientPlans(r, r.Ctx)", "WriteClientPlans(w, p.%s, w.Ctx)"),
        "Queue<BuildPlan>": ("[]*BuildPlan", "ReadPlansQueue(r, r.Ctx)", "WritePlansQueueNet(w, p.%s, w.Ctx)"),
        "Object": ("any", "ReadObject(r, false, r.Ctx)", "WriteObject(w, p.%s, w.Ctx)"),
        "Itemsc": ("any", "ReadObject(r, false, r.Ctx)", "WriteObject(w, p.%s, w.Ctx)"),
        "String[][]": ("[][]string", "ReadStringArray(r)", "WriteStringArray(w, p.%s)"),
        "ItemStack[]": ("[]ItemStack", "ReadItemStacks(r, r.Ctx)", "WriteItemStacks(w, p.%s)"),
        "LiquidStack[]": ("[]LiquidStack", "ReadLiquidStacks(r, r.Ctx)", "WriteLiquidStacks(w, p.%s)"),
        "IntSeq": ("IntSeq", "ReadIntSeq(r)", "WriteIntSeq(w, p.%s)"),
        "int[]": ("[]int32", "ReadInts(r)", "WriteInts(w, p.%s)"),
        "byte[]": ("[]byte", "readLenBytes(r)", "writeLenBytes(w, p.%s)"),
        "short[]": ("[]int16", "readLenInt16s(r)", "writeLenInt16s(w, p.%s)"),
        "Tile": ("Tile", "ReadTile(r, r.Ctx)", "WriteTile(w, p.%s)"),
        "boolean": ("bool", "r.ReadBool()", "w.WriteBool(p.%s)"),
        "byte": ("byte", "r.ReadByte()", "w.WriteByte(p.%s)"),
        "short": ("int16", "r.ReadInt16()", "w.WriteInt16(p.%s)"),
        "int": ("int32", "r.ReadInt32()", "w.WriteInt32(p.%s)"),
        "long": ("int64", "r.ReadInt64()", "w.WriteInt64(p.%s)"),
        "double": ("float64", "r.ReadFloat64()", "w.WriteFloat64(p.%s)"),
        "float": ("float32", "r.ReadFloat32()", "w.WriteFloat32(p.%s)"),
    }
    if jt0 == "String":
        if nullable:
            gt = "string"
            rb = ("%s, err := r.ReadStringNullable()\n\tif err != nil {\n\t\treturn err\n\t}" % var
                  + "\n\tif %s != nil {\n\t\tp.%s = *%s\n\t}" % (var, f, var))
            wb = "if err := w.WriteStringNullable(&p.%s); err != nil {\n\t\treturn err\n\t}" % f
            return gt, rb, wb
        gt = "string"
        return gt, sread("r.ReadStringRaw()"), swrite("w.WriteStringRaw(p.%s)" % f)
    if jt0 in table:
        gt, re_, we_ = table[jt0]
        return gt, sread(re_), swrite(we_ % f)
    # Unknown -> generic Object (compiles; wire may be wrong for exotic types)
    print("  WARN unknown java type %r -> any/ReadObject" % jt0)
    return "any", sread("ReadObject(r, false, r.Ctx)"), swrite("WriteObject(w, p.%s, w.Ctx)" % f)

# ---------------------------------------------------------------------------
# 4. Resolve params for a factory type name
# ---------------------------------------------------------------------------
def resolve_params(name):
    if name in SKIP:
        return None
    logical = REV.get(name)
    if logical is not None and logical in OVERLOAD_PARAMS:
        return OVERLOAD_PARAMS[logical]
    base = re.sub(r'_\d+$', '', name)
    cm = base[len("Remote_"):]
    if "_" in cm:
        cls, method = cm.split("_", 1)
    else:
        cls, method = cm, ""
    plists = grp.get((cls, method), [])
    if len(plists) == 1:
        return plists[0]
    if len(plists) == 0:
        # fall back to method-name lookup (handles inner-class resolution drift)
        alt = grp_method.get(method, [])
        if len(alt) == 1:
            return alt[0]
        print("  WARN no @Remote found for %s (cls=%s method=%s) -> empty struct" % (name, cls, method))
        return []
    print("  WARN multiple signatures for %s, using first" % name)
    return plists[0]

# ---------------------------------------------------------------------------
# 5. Read the official factory order from the current file
# ---------------------------------------------------------------------------
with open(SRC, encoding="utf-8") as f:
    cur = f.read()
ordered = re.findall(r'&([A-Za-z0-9_]+)\{\}', cur)
seen = set()
uniq = []
for n in ordered:
    if n not in seen:
        seen.add(n)
        uniq.append(n)
print("Factory entries (unique, official order):", len(uniq))

# ---------------------------------------------------------------------------
# 6. Generate
# ---------------------------------------------------------------------------
HELPERS = '''
func readLenBytes(r *Reader) ([]byte, error) {
	n, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	return r.ReadBytes(int(n))
}
func writeLenBytes(w *Writer, b []byte) error {
	if b == nil {
		return w.WriteInt32(-1)
	}
	if err := w.WriteInt32(int32(len(b))); err != nil {
		return err
	}
	return w.WriteBytes(b)
}
func readLenInt16s(r *Reader) ([]int16, error) {
	n, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, nil
	}
	out := make([]int16, n)
	for i := int32(0); i < n; i++ {
		v, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
func writeLenInt16s(w *Writer, v []int16) error {
	if v == nil {
		return w.WriteInt32(-1)
	}
	if err := w.WriteInt32(int32(len(v))); err != nil {
		return err
	}
	for _, x := range v {
		if err := w.WriteInt16(x); err != nil {
			return err
		}
	}
	return nil
}
'''

def field_name(name, i, used):
    s = ''.join(ch if (ch.isalnum() or ch == '_') else '' for ch in name)
    if s == '' or not s[0].isalpha():
        s = "F%d" % i
    s = s[0].upper() + s[1:]
    base, n = s, 1
    while s in used:
        s = "%s%d" % (base, n)
        n += 1
    used.add(s)
    return s

structs = []
for name in uniq:
    params = resolve_params(name)
    if params is None:
        continue  # hand-written in remote_packets_159.go
    fields, reads, writes = [], [], []
    used = set()
    for i, full in enumerate(params):
        parts = full.split()
        if len(parts) >= 2:
            ptype = ' '.join(parts[:-1]).strip()
            pname = parts[-1]
        else:
            ptype = full.strip()
            pname = "f%d" % i
        f = field_name(pname, i, used)
        gt, rb, wb = emit(ptype, f, i)
        fields.append("%s %s" % (f, gt))
        reads.append(rb)
        writes.append(wb)
    lines = []
    lines.append("type %s struct {" % name)
    if fields:
        lines.append("\t" + "\n\t".join(fields))
    else:
        lines.append("\t// no payload")
    lines.append("}")
    lines.append("")
    lines.append("func (p *%s) Read(r *Reader, _ int) error {" % name)
    if reads:
        lines.append("\t" + "\n\t".join(reads))
    else:
        lines.append("\t// no fields")
    lines.append("\treturn nil")
    lines.append("}")
    lines.append("")
    lines.append("func (p *%s) Write(w *Writer) error {" % name)
    if writes:
        lines.append("\t" + "\n\t".join(writes))
    else:
        lines.append("\t// no fields")
    lines.append("\treturn nil")
    lines.append("}")
    lines.append("")
    lines.append("func (p *%s) Priority() int { return PriorityNormal }" % name)
    lines.append("")
    structs.append("\n".join(lines))

header = '''package protocol

// Regenerated from Mindustry build-159.7 @Remote method signatures by
// tools/gen_packets.py. Wire IDs follow the official 159.7 registration order
// (see MD/wire-protocol.md). Each Java parameter type is mapped to the matching
// protocol Read*/Write* helper, which already implements Mindustry's exact wire
// format. The 5 new 159.7 packets (playMusic, requestAssets, requestWorld,
// label3, labelReliable3) are defined in remote_packets_159.go.

'''

factory_block = "func remotePacketFactories() []PacketFactory {\n\treturn []PacketFactory{\n"
for n in uniq:
    factory_block += "\t\tfunc() Packet { return &%s{} },\n" % n
factory_block += "\t}\n}\n\n"
init_block = ("func initRemotePackets(r *PacketRegistry) {\n"
             "\tfor _, f := range remotePacketFactories() {\n\t\tr.Register(f)\n\t}\n}\n")

out = header + HELPERS + "\n" + "\n".join(structs) + "\n" + factory_block + init_block
with open(SRC, "w", encoding="utf-8") as f:
    f.write(out)
print("REGENERATED", len(uniq) - len(SKIP), "packet structs into", SRC)
