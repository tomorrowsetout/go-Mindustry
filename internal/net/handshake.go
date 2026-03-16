package net

import (
	"errors"
	"fmt"

	"mdt-server/internal/protocol"
)

var (
	ErrClientOutdated = errors.New("client_outdated")
	ErrServerOutdated = errors.New("server_outdated")
)

// ValidateConnect checks protocol compatibility. It mirrors the Mindustry server's
// version gating behavior and should be wired into connect handling.
func ValidateConnect(pkt *protocol.ConnectPacket, serverBuild int) error {
	if serverBuild <= 0 {
		// Custom builds disable strict checking.
		return nil
	}
	if pkt.Version < int32(serverBuild) {
		return fmt.Errorf("%w: client=%d server=%d", ErrClientOutdated, pkt.Version, serverBuild)
	}
	if pkt.Version > int32(serverBuild) {
		return fmt.Errorf("%w: client=%d server=%d", ErrServerOutdated, pkt.Version, serverBuild)
	}
	return nil
}
