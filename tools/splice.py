import io
src = "internal/protocol/remote_packets.go"
with open(src, encoding="utf-8") as f:
    text = f.read()

with open("tools/factory_body.txt", encoding="utf-8") as f:
    body = f.read().rstrip("\n")

start_marker = "func remotePacketFactories() []PacketFactory {"
end_marker = "func initRemotePackets("

si = text.index(start_marker)
open_brace = text.index("return []PacketFactory{", si)
ei = text.index(end_marker)
close_idx = text.rindex("}", si, ei)
new_func = text[si:open_brace] + "return []PacketFactory{\n" + body + "\n}\n}\n\n"
result = text[:si] + new_func + text[close_idx + 1:]
with open(src, "w", encoding="utf-8") as f:
    f.write(result)
print("Spliced OK. new length:", len(result))
