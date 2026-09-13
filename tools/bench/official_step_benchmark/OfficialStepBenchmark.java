package mdtserver.bench;

import arc.ApplicationCore;
import arc.Core;
import arc.backend.headless.HeadlessApplication;
import arc.files.Fi;
import arc.util.Log;
import arc.util.Time;
import mindustry.Vars;
import mindustry.core.FileTree;
import mindustry.core.GameState.State;
import mindustry.core.Logic;
import mindustry.core.NetServer;
import mindustry.core.World;
import mindustry.game.Rules;
import mindustry.gen.Groups;
import mindustry.io.MapIO;
import mindustry.io.SaveIO;
import mindustry.maps.Map;
import mindustry.mod.Mod;
import mindustry.mod.Mods.LoadedMod;
import mindustry.net.Net;

import java.io.IOException;
import java.lang.reflect.Field;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Locale;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

public final class OfficialStepBenchmark{
    private OfficialStepBenchmark(){
    }

    public static void main(String[] args) throws Exception{
        Options options = Options.parse(args);
        AtomicBoolean done = new AtomicBoolean(false);
        AtomicReference<Throwable> failure = new AtomicReference<>();

        Log.useColors = false;
        HeadlessApplication app = new HeadlessApplication(new BenchApplication(options, done, failure), throwable -> failure.compareAndSet(null, throwable));

        while(!done.get()){
            Throwable error = failure.get();
            if(error != null){
                app.exit();
                throw rethrow(error);
            }
            Thread.sleep(10L);
        }
        Throwable error = failure.get();
        if(error != null){
            throw rethrow(error);
        }
    }

    private static RuntimeException rethrow(Throwable error){
        return error instanceof RuntimeException runtime ? runtime : new RuntimeException(error);
    }

    private static final class BenchApplication extends ApplicationCore{
        private final Options options;
        private final AtomicBoolean done;
        private final AtomicReference<Throwable> failure;

        BenchApplication(Options options, AtomicBoolean done, AtomicReference<Throwable> failure){
            this.options = options;
            this.done = done;
            this.failure = failure;
        }

        @Override
        public void setup(){
            Path runtimeDir = Paths.get(options.runtimeDir).toAbsolutePath();
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
                    if(!mod.hasContentErrors()) continue;
                    for(var content : mod.erroredContent){
                        throw new RuntimeException("mod content error in " + content.minfo.sourceFile.path(), content.minfo.baseError);
                    }
                }
            }
        }

        @Override
        public void init(){
            try{
                super.init();
                Time.setDeltaProvider(() -> options.deltaMs * 60f / 1000f);
                loadWorldScenario(options.mapPath, options.mode);
                for(int i = 0; i < options.warmup; i++){
                    forceGraphicsDelta(options.deltaMs / 1000f);
                    Vars.logic.update();
                }
                long start = System.nanoTime();
                for(int i = 0; i < options.ticks; i++){
                    forceGraphicsDelta(options.deltaMs / 1000f);
                    Vars.logic.update();
                }
                long elapsed = System.nanoTime() - start;
                double nsPerTick = elapsed / (double)options.ticks;
                double tps = 1_000_000_000.0 / nsPerTick;
                System.out.printf(Locale.ROOT,
                    "{\"producer\":\"mindustry-official\",\"mapPath\":\"%s\",\"mode\":\"%s\",\"warmup\":%d,\"ticks\":%d,\"deltaMs\":%.6f,\"elapsedNs\":%d,\"nsPerTick\":%.3f,\"ticksPerSecond\":%.3f,\"units\":%d,\"bullets\":%d}%n",
                    escape(options.mapPath), escape(options.mode), options.warmup, options.ticks, options.deltaMs, elapsed, nsPerTick, tps, Groups.unit.size(), Groups.bullet.size());
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

    private static String loadWorldScenario(String path, String mode) throws Exception{
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

    private static boolean looksLikeMapLoad(String mode){
        if(mode == null) return false;
        String normalized = mode.trim().toLowerCase(Locale.ROOT);
        return normalized.equals("mapio") || normalized.equals("map") || normalized.equals("sandbox") || normalized.equals("survival") || normalized.equals("attack") || normalized.equals("pvp") || normalized.equals("editor");
    }

    private static mindustry.game.Gamemode resolveMode(String mode){
        if(mode == null || mode.isBlank()) return mindustry.game.Gamemode.sandbox;
        return switch(mode.trim().toLowerCase(Locale.ROOT)){
            case "survival" -> mindustry.game.Gamemode.survival;
            case "attack" -> mindustry.game.Gamemode.attack;
            case "pvp" -> mindustry.game.Gamemode.pvp;
            case "editor" -> mindustry.game.Gamemode.editor;
            default -> mindustry.game.Gamemode.sandbox;
        };
    }

    private static void forceGraphicsDelta(float deltaSeconds){
        if(Core.graphics == null) return;
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

    private static String escape(String value){
        return value == null ? "" : value.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private static final class Options{
        String mapPath = "";
        String mode = "";
        String runtimeDir = "build/mdt-step-bench-runtime";
        int warmup = 20;
        int ticks = 300;
        float deltaMs = 1000f / 60f;

        static Options parse(String[] args){
            Options options = new Options();
            for(int i = 0; i < args.length; i++){
                String arg = args[i];
                String value = i + 1 < args.length ? args[++i] : "";
                switch(arg){
                    case "--map" -> options.mapPath = value;
                    case "--mode" -> options.mode = value;
                    case "--runtime-dir" -> options.runtimeDir = value;
                    case "--warmup" -> options.warmup = Integer.parseInt(value);
                    case "--ticks" -> options.ticks = Integer.parseInt(value);
                    case "--delta-ms" -> options.deltaMs = Float.parseFloat(value);
                    default -> throw new IllegalArgumentException("unknown arg: " + arg);
                }
            }
            if(options.mapPath.isBlank()){
                throw new IllegalArgumentException("--map is required");
            }
            if(options.ticks <= 0){
                throw new IllegalArgumentException("--ticks must be > 0");
            }
            return options;
        }
    }
}
