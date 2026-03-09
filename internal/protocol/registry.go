package protocol

// RegistryStatus reports coarse protocol-registry coverage for diagnostics.
type RegistryStatus struct {
	BasePackets   int
	RemotePackets int
	TotalPackets  int
}

// GetRegistryStatus returns registration counts for a build-version registry.
func GetRegistryStatus(buildVersion int) RegistryStatus {
	r := NewRegistry(buildVersion)
	total := r.Count()
	base := 4 // StreamBegin, StreamChunk, WorldStream, ConnectPacket
	remote := total - base
	if remote < 0 {
		remote = 0
	}
	return RegistryStatus{
		BasePackets:   base,
		RemotePackets: remote,
		TotalPackets:  total,
	}
}
