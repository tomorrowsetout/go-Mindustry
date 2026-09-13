package mdtserver.oracle;

import arc.ApplicationCore;
import arc.Core;
import arc.files.Fi;
import arc.backend.headless.HeadlessApplication;
import arc.struct.Seq;
import arc.util.Log;
import arc.util.Time;
import arc.util.serialization.Json;
import arc.util.serialization.JsonWriter;
import mindustry.Vars;
import mindustry.core.FileTree;
import mindustry.core.GameState.State;
import mindustry.core.Logic;
import mindustry.core.NetServer;
import mindustry.core.World;
import mindustry.game.Rules;
import mindustry.gen.Building;
import mindustry.gen.Groups;
import mindustry.gen.Unit;
import mindustry.io.MapIO;
import mindustry.io.SaveIO;
import mindustry.maps.Map;
import mindustry.mod.Mod;
import mindustry.mod.Mods.LoadedMod;
import mindustry.net.Net;
import mindustry.type.Item;
import mindustry.type.Liquid;
import mindustry.world.Tile;
import mindustry.world.blocks.ConstructBlock;

import java.io.IOException;
import java.lang.reflect.Field;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

public final class OfficialTraceLauncher {
    private OfficialTraceLauncher() {
    }

    public static void main(String[] args) throws Exception {
        CliOptions options = CliOptions.parse(args);
        if(options.scenarioPath.isEmpty()){
            throw new IllegalArgumentException("--scenario is required");
        }
        if(options.outputPath.isEmpty()){
            throw new IllegalArgumentException("--out is required");
        }

        ScenarioData scenario = readScenario(options.scenarioPath);
        scenario.normalize();

        AtomicBoolean done = new AtomicBoolean(false);
        AtomicReference<Throwable> failure = new AtomicReference<>();

        Log.useColors = false;
        HeadlessApplication app = new HeadlessApplication(new OracleApplication(scenario, options.outputPath, done, failure), throwable -> failure.compareAndSet(null, throwable));

        while(!done.get()){
            Throwable error = failure.get();
            if(error != null){
                try{
                    app.exit();
                }catch(Throwable ignored){
                }
                throw rethrow(error);
            }
            Thread.sleep(10L);
        }

        Throwable error = failure.get();
        if(error != null){
            throw rethrow(error);
        }
    }

    private static RuntimeException rethrow(Throwable error) {
        return error instanceof RuntimeException runtime ? runtime : new RuntimeException(error);
    }

    private static ScenarioData readScenario(String path) throws IOException {
        String raw = Files.readString(Paths.get(path), StandardCharsets.UTF_8);
        ScenarioData scenario = new Json().fromJson(ScenarioData.class, raw);
        if(scenario == null){
            throw new IOException("scenario json is empty");
        }
        return scenario;
    }

    private static final class OracleApplication extends ApplicationCore {
        private final ScenarioData scenario;
        private final String outputPath;
        private final AtomicBoolean done;
        private final AtomicReference<Throwable> failure;

        OracleApplication(ScenarioData scenario, String outputPath, AtomicBoolean done, AtomicReference<Throwable> failure) {
            this.scenario = scenario;
            this.outputPath = outputPath;
            this.done = done;
            this.failure = failure;
        }

        @Override
        public void setup() {
            Path out = Paths.get(outputPath).toAbsolutePath();
            Path runtimeDir = out.getParent() == null ? Paths.get(".").toAbsolutePath().resolve("oracle-runtime") : out.getParent().resolve("oracle-runtime");

            Core.settings.setDataDirectory(new Fi(runtimeDir.toString()));
            Vars.headless = true;
            Vars.net = new Net(null);
            Vars.tree = new FileTree();
            Vars.init();
            Vars.world = new World(){
                @Override
                public float getDarkness(int x, int y){
                    return 0f;
                }
            };

            Vars.content.createBaseContent();
            Vars.mods.loadScripts();
            Vars.content.createModContent();

            add(Vars.logic = new Logic());
            add(Vars.netServer = new NetServer());

            Vars.content.init();
            Vars.mods.eachClass(Mod::init);

            if(Vars.mods.hasContentErrors()){
                for(LoadedMod mod : Vars.mods.list()){
                    if(!mod.hasContentErrors()){
                        continue;
                    }
                    for(var content : mod.erroredContent){
                        throw new RuntimeException("mod content error in " + content.minfo.sourceFile.path(), content.minfo.baseError);
                    }
                }
            }
        }

        @Override
        public void init() {
            try{
                super.init();
                Time.setDeltaProvider(scenario::deltaTicks);
                TraceData trace = collectTrace(scenario);
                writeTrace(outputPath, trace);
            }catch(Throwable error){
                failure.compareAndSet(null, error);
            }finally{
                done.set(true);
                if(Core.app != null){
                    Core.app.exit();
                }
            }
        }
    }

    private static TraceData collectTrace(ScenarioData scenario) throws Exception {
        TraceData trace = new TraceData();
        trace.producer = "mindustry-official";
        trace.scenario = scenario.copy();
        trace.tolerances = TolerancesData.defaults();

        String loadMode = loadWorldScenario(scenario.mapPath, scenario.mode);
        trace.metadata.put("mapPathResolved", resolvePath(scenario.mapPath));
        trace.metadata.put("loadMode", loadMode);
        trace.metadata.put("mapWidth", Integer.toString(Vars.world.width()));
        trace.metadata.put("mapHeight", Integer.toString(Vars.world.height()));
        putContentMetadata(trace.metadata);

        if(scenario.captureInitial){
            trace.ticks.add(snapshotTick(0, null));
        }

        for(int tick = 1; tick <= scenario.ticks; tick++){
            ArrayList<TraceEventData> events = null;
            if(scenario.hotReloadTick > 0 && tick == scenario.hotReloadTick && !scenario.hotReloadMapPath.isEmpty()){
                String hotReloadMode = loadWorldScenario(scenario.hotReloadMapPath, scenario.mode);
                events = new ArrayList<>();
                TraceEventData event = new TraceEventData();
                event.tick = tick;
                event.kind = "map_hot_reload";
                event.fields = new LinkedHashMap<>();
                event.fields.put("path", resolvePath(scenario.hotReloadMapPath));
                event.fields.put("loadMode", hotReloadMode);
                events.add(event);
            }

            forceGraphicsDelta(scenario.deltaSeconds());
            Vars.logic.update();
            trace.ticks.add(snapshotTick(tick, events));
        }

        return trace;
    }

    private static String loadWorldScenario(String path, String mode) throws Exception {
        Fi file = new Fi(path);
        if(!file.exists()){
            throw new IOException("world file not found: " + path);
        }

        Throwable saveError = null;
        if(!looksLikeMapLoad(mode)){
            try{
                SaveIO.load(file);
                if(Vars.state.rules == null){
                    Vars.state.rules = new Rules();
                }
                Vars.state.set(State.playing);
                return "saveio";
            }catch(Throwable error){
                saveError = error;
                if(Vars.logic != null){
                    Vars.logic.reset();
                }
            }
        }

        try{
            Map map = MapIO.createMap(file, true);
            var gameMode = resolveMode(mode);
            Vars.world.loadMap(map, map.applyRules(gameMode));
            Vars.state.rules = map.applyRules(gameMode);
            Vars.logic.play();
            return "mapio";
        }catch(Throwable mapError){
            if(saveError != null){
                mapError.addSuppressed(saveError);
            }
            throw mapError;
        }
    }

    private static boolean looksLikeMapLoad(String mode) {
        if(mode == null){
            return false;
        }
        String normalized = mode.trim().toLowerCase(Locale.ROOT);
        return normalized.equals("mapio") || normalized.equals("map") || normalized.equals("sandbox") || normalized.equals("survival") || normalized.equals("attack") || normalized.equals("pvp") || normalized.equals("editor");
    }

    private static mindustry.game.Gamemode resolveMode(String mode) {
        if(mode == null || mode.isBlank()){
            return mindustry.game.Gamemode.sandbox;
        }
        return switch(mode.trim().toLowerCase(Locale.ROOT)){
            case "survival" -> mindustry.game.Gamemode.survival;
            case "attack" -> mindustry.game.Gamemode.attack;
            case "pvp" -> mindustry.game.Gamemode.pvp;
            case "editor" -> mindustry.game.Gamemode.editor;
            default -> mindustry.game.Gamemode.sandbox;
        };
    }

    private static TickTraceData snapshotTick(int index, ArrayList<TraceEventData> events) {
        TickTraceData tick = new TickTraceData();
        tick.index = index;
        tick.tiles = collectTileStates();
        tick.units = collectUnitStates();
        if(events != null && !events.isEmpty()){
            tick.events = events;
        }
        return tick;
    }

    private static ArrayList<TileStateData> collectTileStates() {
        ArrayList<TileStateData> out = new ArrayList<>();
        for(int y = 0; y < Vars.world.height(); y++){
            for(int x = 0; x < Vars.world.width(); x++){
                Tile tile = Vars.world.tile(x, y);
                if(tile == null || !includeTile(tile)){
                    continue;
                }
                TileStateData state = new TileStateData();
                state.x = x;
                state.y = y;
                state.floorId = tile.floor() == null ? 0 : tile.floor().id;
                state.overlayId = tile.overlay() == null ? 0 : tile.overlay().id;
                state.blockId = tile.block() == null ? 0 : tile.block().id;
                state.teamId = tile.team() == null ? 0 : tile.team().id;
                state.rotation = tile.build == null ? 0 : tile.build.rotation;
                state.logic = new LogicStateData();
                if(tile.build != null){
                    Building build = tile.build;
                    state.buildHealth = build.health;
                    state.items = collectItems(build);
                    state.liquids = collectLiquids(build);
                    if(build instanceof ConstructBlock.ConstructBuild construct){
                        state.constructBlockId = construct.current == null ? 0 : construct.current.id;
                        state.constructProgress = construct.progress;
                    }
                }
                out.add(state);
            }
        }
        return out;
    }

    private static boolean includeTile(Tile tile) {
        return tile.build != null || (tile.block() != null && tile.block().id != 0);
    }

    private static ArrayList<SlotStateData> collectItems(Building build) {
        if(build.items == null){
            return null;
        }
        ArrayList<SlotStateData> out = new ArrayList<>();
        build.items.each((item, amount) -> {
            if(amount == 0){
                return;
            }
            SlotStateData slot = new SlotStateData();
            slot.id = item.id;
            slot.amount = amount;
            out.add(slot);
        });
        return out.isEmpty() ? null : out;
    }

    private static ArrayList<SlotStateData> collectLiquids(Building build) {
        if(build.liquids == null){
            return null;
        }
        ArrayList<SlotStateData> out = new ArrayList<>();
        build.liquids.each((liquid, amount) -> {
            if(amount <= 0f){
                return;
            }
            SlotStateData slot = new SlotStateData();
            slot.id = liquid.id;
            slot.amount = amount;
            out.add(slot);
        });
        return out.isEmpty() ? null : out;
    }

    private static ArrayList<UnitStateData> collectUnitStates() {
        ArrayList<Unit> units = new ArrayList<>();
        for(Unit unit : Groups.unit){
            units.add(unit);
        }
        units.sort((left, right) -> Integer.compare(left.id, right.id));

        ArrayList<UnitStateData> out = new ArrayList<>(units.size());
        for(Unit unit : units){
            UnitStateData state = new UnitStateData();
            state.id = unit.id;
            state.typeId = unit.type == null ? 0 : unit.type.id;
            state.teamId = unit.team == null ? 0 : unit.team.id;
            state.x = unit.x;
            state.y = unit.y;
            state.rotation = unit.rotation;
            state.velocityX = unit.vel.x;
            state.velocityY = unit.vel.y;
            state.health = unit.health;
            state.shield = unit.shield;
            state.controllerType = controllerType(unit);
            out.add(state);
        }
        return out;
    }

    private static String controllerType(Unit unit) {
        if(unit == null){
            return "none";
        }
        if(unit.isPlayer()){
            return "player";
        }
        var controller = unit.controller();
        if(controller == null){
            return "none";
        }
        String simple = controller.getClass().getSimpleName();
        if(simple.contains("Command")){
            return "command";
        }
        return simple.isEmpty() ? "none" : simple;
    }

    private static void forceGraphicsDelta(float deltaSeconds) {
        if(Core.graphics == null){
            return;
        }
        Class<?> current = Core.graphics.getClass();
        while(current != null){
            for(Field field : current.getDeclaredFields()){
                String name = field.getName().toLowerCase(Locale.ROOT);
                if(field.getType() == float.class && (name.equals("delta") || name.equals("deltatime"))){
                    try{
                        field.setAccessible(true);
                        field.setFloat(Core.graphics, deltaSeconds);
                    }catch(Throwable ignored){
                    }
                }
            }
            current = current.getSuperclass();
        }
    }

    private static String resolvePath(String path) throws IOException {
        return Paths.get(path).toAbsolutePath().normalize().toString();
    }

    private static void putContentMetadata(LinkedHashMap<String, String> metadata) {
        metadata.put("content.blocks.count", Integer.toString(Vars.content.blocks().size));
        Vars.content.blocks().each(block -> metadata.put("content.block." + block.name, Short.toString(block.id)));
        metadata.put("content.units.count", Integer.toString(Vars.content.units().size));
        Vars.content.units().each(unit -> metadata.put("content.unit." + unit.name, Short.toString(unit.id)));
    }

    private static void writeTrace(String outputPath, TraceData trace) throws IOException {
        Json json = new Json();
        json.setOutputType(JsonWriter.OutputType.json);
        String raw = json.prettyPrint(trace);
        Path out = Paths.get(outputPath).toAbsolutePath();
        Path parent = out.getParent();
        if(parent != null){
            Files.createDirectories(parent);
        }
        Files.writeString(out, raw + System.lineSeparator(), StandardCharsets.UTF_8);
    }

    public static final class CliOptions {
        public String scenarioPath = "";
        public String outputPath = "";

        static CliOptions parse(String[] args) {
            CliOptions out = new CliOptions();
            for(int i = 0; i < args.length; i++){
                if("--scenario".equals(args[i]) && i + 1 < args.length){
                    out.scenarioPath = args[++i];
                }else if("--out".equals(args[i]) && i + 1 < args.length){
                    out.outputPath = args[++i];
                }
            }
            return out;
        }
    }

    public static final class ScenarioData {
        public String name = "";
        public String mapPath = "";
        public long seed = 0L;
        public int ticks = 0;
        public int deltaMs = 16;
        public String mode = "";
        public int hotReloadTick = 0;
        public String hotReloadMapPath = "";
        public String vanillaProfilesPath = "";
        public boolean captureInitial = false;
        public String[] tags = new String[0];

        void normalize() {
            if(ticks < 0){
                ticks = 0;
            }
            if(deltaMs <= 0){
                deltaMs = 16;
            }
            if(name == null){
                name = "";
            }
            if(mapPath == null){
                mapPath = "";
            }
            if(mode == null){
                mode = "";
            }
            if(hotReloadMapPath == null){
                hotReloadMapPath = "";
            }
            if(vanillaProfilesPath == null){
                vanillaProfilesPath = "";
            }
            if(tags == null){
                tags = new String[0];
            }
        }

        float deltaTicks() {
            return (deltaMs * 60f) / 1000f;
        }

        float deltaSeconds() {
            return deltaMs / 1000f;
        }

        ScenarioData copy() {
            ScenarioData copy = new ScenarioData();
            copy.name = name;
            copy.mapPath = mapPath;
            copy.seed = seed;
            copy.ticks = ticks;
            copy.deltaMs = deltaMs;
            copy.mode = mode;
            copy.hotReloadTick = hotReloadTick;
            copy.hotReloadMapPath = hotReloadMapPath;
            copy.vanillaProfilesPath = vanillaProfilesPath;
            copy.captureInitial = captureInitial;
            copy.tags = tags.clone();
            return copy;
        }
    }

    public static final class TolerancesData {
        public double position;
        public double angle;
        public double scalar;

        static TolerancesData defaults() {
            TolerancesData out = new TolerancesData();
            out.position = 0.01d;
            out.angle = 0.1d;
            out.scalar = 1e-4d;
            return out;
        }
    }

    public static final class LogicStateData {
        public boolean enabled = false;
        public String controlledBy = "";
        public LinkedHashMap<String, Boolean> flags = null;
        public LinkedHashMap<String, Double> numbers = null;
    }

    public static final class SlotStateData {
        public int id;
        public double amount;
    }

    public static final class TileStateData {
        public int x;
        public int y;
        public int floorId;
        public int overlayId;
        public int blockId;
        public int teamId;
        public int rotation;
        public double buildHealth;
        public int constructBlockId;
        public double constructProgress;
        public String controllerType = "";
        public double powerStored;
        public double powerBalance;
        public double heat;
        public double reload;
        public ArrayList<SlotStateData> items;
        public ArrayList<SlotStateData> liquids;
        public ArrayList<Integer> payloadUnitTypeIds;
        public LogicStateData logic;
    }

    public static final class UnitStateData {
        public int id;
        public int typeId;
        public int teamId;
        public double x;
        public double y;
        public double rotation;
        public double velocityX;
        public double velocityY;
        public double health;
        public double shield;
        public String controllerType = "none";
        public LinkedHashMap<String, String> aiState;
        public LinkedHashMap<String, Double> mountReloads;
    }

    public static final class TraceEventData {
        public int tick;
        public String kind = "";
        public int subjectId;
        public int x;
        public int y;
        public LinkedHashMap<String, String> fields;
    }

    public static final class TickTraceData {
        public int index;
        public ArrayList<TileStateData> tiles;
        public ArrayList<UnitStateData> units;
        public ArrayList<TraceEventData> events;
    }

    public static final class TraceData {
        public String producer = "";
        public ScenarioData scenario;
        public TolerancesData tolerances;
        public LinkedHashMap<String, String> metadata = new LinkedHashMap<>();
        public ArrayList<TickTraceData> ticks = new ArrayList<>();
    }
}
