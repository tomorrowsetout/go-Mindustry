package world

import (
	"mdt-server/internal/protocol"
)

// writeBlockPayloadTurretAmmoLocked writes PayloadSeq data for PayloadAmmoTurret
// Based on Mindustry 157: PayloadAmmoTurret.PayloadTurretBuild.write()
// Format: negated size (short), then for each entry: contentType (byte), contentID (short), amount (int)
func (w *World) writeBlockPayloadTurretAmmoLocked(writer *protocol.Writer, tile *Tile) error {
	if writer == nil {
		return nil
	}
	if tile == nil || tile.Build == nil {
		// Empty PayloadSeq: write size = 0 (negated)
		return writer.WriteInt16(0)
	}

	// PayloadSeq stores payloads as ObjectIntMap<UnlockableContent>
	// For now, we'll write the Items as block payloads (similar to ItemTurret but with PayloadSeq format)
	// In Mindustry 157, PayloadSeq.write() format:
	// - write.s(-payloads.size)  // negated size
	// - for each entry:
	//   - write.b(entry.key.getContentType().ordinal())
	//   - write.s(entry.key.id)
	//   - write.i(entry.value)

	items := make([]ItemStack, 0, len(tile.Build.Items))
	for _, stack := range tile.Build.Items {
		if stack.Amount > 0 {
			items = append(items, stack)
		}
	}

	// Write negated size (new format indicator)
	if err := writer.WriteInt16(int16(-len(items))); err != nil {
		return err
	}

	// Write each payload entry
	for _, stack := range items {
		// ContentType for Item is 1 (from ContentType enum)
		if err := writer.WriteByte(1); err != nil {
			return err
		}
		// Content ID
		if err := writer.WriteInt16(int16(stack.Item)); err != nil {
			return err
		}
		// Amount
		if err := writer.WriteInt32(stack.Amount); err != nil {
			return err
		}
	}

	return nil
}
