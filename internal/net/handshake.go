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

// ValidateConnect checks the official build-157 protocol gate used by the server.
func ValidateConnect(pkt *protocol.ConnectPacket, serverBuild int) error {
	if serverBuild <= 0 {
		serverBuild = 157
	}
	if pkt == nil || pkt.Version == -1 {
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
