package net

import (
	"errors"
	"fmt"
	"strings"

	"mdt-server/internal/protocol"
)

var (
	ErrClientOutdated = errors.New("client_outdated")
	ErrServerOutdated = errors.New("server_outdated")
	ErrTypeMismatch   = errors.New("type_mismatch")
	ErrCustomClient   = errors.New("custom_client")
	ErrIDInUse        = errors.New("id_in_use")
	ErrNameEmpty      = errors.New("name_empty")
)

// ValidateConnect checks protocol compatibility. It mirrors the Mindustry server's
// version gating behavior and should be wired into connect handling.
func ValidateConnect(pkt *protocol.ConnectPacket, serverBuild int, strict bool) error {
	if pkt == nil {
		return fmt.Errorf("%w: nil packet", ErrIDInUse)
	}
	if strings.TrimSpace(pkt.UUID) == "" || strings.TrimSpace(pkt.USID) == "" {
		return fmt.Errorf("%w: uuid/usid required", ErrIDInUse)
	}
	if strings.TrimSpace(pkt.Name) == "" {
		return fmt.Errorf("%w: name required", ErrNameEmpty)
	}

	if serverBuild > 0 {
		versionType := strings.ToLower(strings.TrimSpace(pkt.VersionType))
		if strict {
			// Headless server is "official" in vanilla.
			if versionType == "" {
				return fmt.Errorf("%w: empty versionType", ErrTypeMismatch)
			}
			if pkt.Version == -1 {
				return fmt.Errorf("%w: modded client build", ErrCustomClient)
			}
			if versionType != "official" {
				return fmt.Errorf("%w: versionType=%q", ErrTypeMismatch, pkt.VersionType)
			}
		}
		if pkt.Version != -1 {
			if pkt.Version < int32(serverBuild) {
				return fmt.Errorf("%w: client=%d server=%d", ErrClientOutdated, pkt.Version, serverBuild)
			}
			if pkt.Version > int32(serverBuild) {
				return fmt.Errorf("%w: client=%d server=%d", ErrServerOutdated, pkt.Version, serverBuild)
			}
		}
	}
	return nil
}
