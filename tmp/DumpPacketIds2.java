import java.lang.reflect.*;
public class DumpPacketIds2 {
  public static void main(String[] args) throws Exception {
    Class<?> netCls = Class.forName("mindustry.net.Net");
    Field f = netCls.getDeclaredField("packetToId");
    f.setAccessible(true);
    Object map = f.get(null);
    Method get = map.getClass().getMethod("get", Object.class, int.class);
    String[] names = new String[]{
      "mindustry.gen.SendChatMessageCallPacket",
      "mindustry.gen.SendMessageCallPacket",
      "mindustry.gen.SendMessageCallPacket2",
      "mindustry.gen.ConnectConfirmCallPacket",
      "mindustry.gen.PingCallPacket",
      "mindustry.gen.PingResponseCallPacket",
      "mindustry.gen.UnitClearCallPacket"
    };
    for(String n:names){
      Class<?> c = Class.forName(n);
      int id = (Integer)get.invoke(map, c, -1);
      System.out.println(n+" => "+id);
    }
  }
}
