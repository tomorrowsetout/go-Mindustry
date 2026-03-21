import java.lang.reflect.*;

public class DumpPacketIds {
    public static void main(String[] args) throws Exception {
        Class<?> netCls = Class.forName("mindustry.net.Net");
        Field f = netCls.getDeclaredField("packetToId");
        f.setAccessible(true);
        Object map = f.get(null);
        Method get = map.getClass().getMethod("get", Object.class, int.class);

        String[] names = new String[]{
            "mindustry.gen.PlayerSpawnCallPacket",
            "mindustry.gen.StateSnapshotCallPacket",
            "mindustry.gen.EntitySnapshotCallPacket",
            "mindustry.gen.HiddenSnapshotCallPacket",
            "mindustry.gen.BuildHealthUpdateCallPacket",
            "mindustry.gen.BeginPlaceCallPacket",
            "mindustry.gen.BeginBreakCallPacket",
            "mindustry.gen.ConstructFinishCallPacket",
            "mindustry.gen.DeconstructFinishCallPacket",
            "mindustry.gen.UpdateMarkerCallPacket",
            "mindustry.gen.TransferItemToCallPacket",
            "mindustry.gen.UnitDestroyCallPacket"
        };

        for (String n : names) {
            Class<?> c = Class.forName(n);
            Object inst = c.getConstructor().newInstance();
            int id = (Integer)get.invoke(map, inst.getClass(), -1);
            System.out.println(n + " => " + id);
        }
    }
}
