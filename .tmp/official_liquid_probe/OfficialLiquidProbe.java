package mdtserver.oracle;

import arc.ApplicationCore;
import arc.Core;
import arc.backend.headless.HeadlessApplication;
import arc.files.Fi;
import arc.util.Log;
import arc.util.Time;
import mindustry.Vars;
import mindustry.core.FileTree;
import mindustry.core.Logic;
import mindustry.core.NetServer;
import mindustry.core.World;
import mindustry.game.Rules;
import mindustry.gen.Building;
import mindustry.gen.Groups;
import mindustry.io.MapIO;
import mindustry.io.SaveIO;
import mindustry.maps.Map;
import mindustry.mod.Mod;
import mindustry.net.Net;
import mindustry.type.Liquid;

import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Locale;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicReference;

public final class OfficialLiquidProbe {
    private OfficialLiquidProbe() {
    }

    public static void main(String[] args) throws Exception {
        AtomicBoolean done = new AtomicBoolean(false);
        AtomicReference<Throwable> failure = new AtomicReference<>();
        Log.useColors = false;
        HeadlessApplication app = new HeadlessApplication(new App(done, failure), throwable -> failure.compareAndSet(null, throwable));
        while(!done.get()){
            Throwable error = failure.get();
            if(error != null){
                app.exit();
                throw new RuntimeException(error);
            }
            Thread.sleep(10L);
        }
        if(failure.get() != null){
            throw new RuntimeException(failure.get());
        }
    }

    private static final class App extends ApplicationCore {
        private final AtomicBoolean done;
        private final AtomicReference<Throwable> failure;

        App(AtomicBoolean done, AtomicReference<Throwable> failure) {
            this.done = done;
            this.failure = failure;
        }

        @Override
        public void setup() {
            Path runtimeDir = Paths.get("/server/go-Mindustry-main/.tmp/official_liquid_probe/runtime");
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
        }

        @Override
        public void init() {
            try{
                super.init();
                loadWorld("/server/go-Mindustry-main/assets/worlds/file.msav", "");
                Time.setDeltaProvider(() -> 16f / (1000f / 60f));
                System.out.println("initial");
                dumpBuildOrder();
                dumpPoints();
                forceGraphicsDelta(16f / 1000f);
                Vars.logic.update();
                System.out.println("after");
                dumpPoints();
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

    private static void loadWorld(String path, String mode) throws Exception {
        Fi file = new Fi(path);
        try{
            SaveIO.load(file);
            if(Vars.state.rules == null){
                Vars.state.rules = new Rules();
            }
            Vars.state.set(mindustry.core.GameState.State.playing);
            return;
        }catch(Throwable error){
            if(Vars.logic != null){
                Vars.logic.reset();
            }
        }
        Map map = MapIO.createMap(file, true);
        var gameMode = switch(mode.trim().toLowerCase(Locale.ROOT)){
            case "survival" -> mindustry.game.Gamemode.survival;
            case "attack" -> mindustry.game.Gamemode.attack;
            case "pvp" -> mindustry.game.Gamemode.pvp;
            case "editor" -> mindustry.game.Gamemode.editor;
            default -> mindustry.game.Gamemode.sandbox;
        };
        Vars.world.loadMap(map, map.applyRules(gameMode));
        Vars.state.rules = map.applyRules(gameMode);
        Vars.logic.play();
    }

    private static void forceGraphicsDelta(float seconds) {
        if(Core.graphics == null){
            return;
        }
        try{
            var field = Core.graphics.getClass().getDeclaredField("deltaTime");
            field.setAccessible(true);
            field.setFloat(Core.graphics, seconds);
        }catch(Throwable ignored){
        }
    }

    private static void dumpPoints() {
        int[][] points = new int[][]{
            {315,486}, {316,486}, {319,486}, {320,486}, {380,484}, {381,484},
            {316,484}, {316,488}, {380,482}, {383,482}, {383,487}, {387,487}
        };
        for(int[] point : points){
            Building build = Vars.world.build(point[0], point[1]);
            if(build == null){
                System.out.printf("%d,%d build=null%n", point[0], point[1]);
                continue;
            }
            System.out.printf("%d,%d block=%s tile=%d,%d rot=%d cdump=%d liquids=%s prox=",
                point[0], point[1], build.block.name, build.tileX(), build.tileY(), build.rotation, build.cdump, liquids(build));
            System.out.printf(" reload=%s ", reload(build));
            for(int i = 0; i < build.proximity.size; i++){
                Building other = build.proximity.get(i);
                System.out.printf("%s%d,%d:%s", i == 0 ? "" : "|", other.tileX(), other.tileY(), other.block.name);
            }
            System.out.println();
        }
    }

    private static void dumpBuildOrder() {
        System.out.println("order");
        Groups.build.each(build -> {
            int x = build.tileX(), y = build.tileY();
            if((x >= 309 && x <= 392 && y >= 479 && y <= 490) || (x >= 376 && x <= 392 && y >= 485 && y <= 490)){
                System.out.printf("%d,%d:%s%n", x, y, build.block.name);
            }
        });
    }

    private static String liquids(Building build) {
        if(build.liquids == null){
            return "[]";
        }
        StringBuilder out = new StringBuilder("[");
        boolean first = true;
        for(Liquid liquid : Vars.content.liquids()){
            float amount = build.liquids.get(liquid);
            if(amount <= 0f){
                continue;
            }
            if(!first){
                out.append(",");
            }
            first = false;
            out.append(liquid.id).append(":").append(amount);
        }
        out.append("]");
        return out.toString();
    }

    private static String reload(Building build) {
        Class<?> current = build.getClass();
        while(current != null){
            try{
                var field = current.getDeclaredField("reloadCounter");
                field.setAccessible(true);
                return Float.toString(field.getFloat(build));
            }catch(NoSuchFieldException ignored){
                current = current.getSuperclass();
            }catch(Throwable error){
                return "err:" + error.getClass().getSimpleName();
            }
        }
        return "-";
    }
}
