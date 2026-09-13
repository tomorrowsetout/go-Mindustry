package protocol

// Build-160.3 @Remote additions that shift wire IDs vs 159.7.
// Full TypeIO for NodeBuilder/MenuResult is out of scope for join; these
// types keep registry slots aligned and retain raw payloads when length>0.

type Remote_Tile_fillTileBlocks_160 struct {
	X, Y, X2, Y2 int32
	Block        int16
	Team         int8
	Raw          []byte
}

func (p *Remote_Tile_fillTileBlocks_160) Read(r *Reader, length int) error {
	if length > 0 {
		raw, err := r.ReadBytes(length)
		if err != nil {
			return err
		}
		p.Raw = raw
		return nil
	}
	x, err := r.ReadInt32()
	if err != nil {
		return err
	}
	y, err := r.ReadInt32()
	if err != nil {
		return err
	}
	x2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	y2, err := r.ReadInt32()
	if err != nil {
		return err
	}
	block, err := r.ReadInt16()
	if err != nil {
		return err
	}
	team, err := r.ReadByte()
	if err != nil {
		return err
	}
	p.X, p.Y, p.X2, p.Y2 = x, y, x2, y2
	p.Block = block
	p.Team = int8(team)
	return nil
}

func (p *Remote_Tile_fillTileBlocks_160) Write(w *Writer) error {
	if len(p.Raw) > 0 {
		return w.WriteBytes(p.Raw)
	}
	if err := w.WriteInt32(p.X); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Y); err != nil {
		return err
	}
	if err := w.WriteInt32(p.X2); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Y2); err != nil {
		return err
	}
	if err := w.WriteInt16(p.Block); err != nil {
		return err
	}
	return w.WriteByte(byte(p.Team))
}

func (p *Remote_Tile_fillTileBlocks_160) Priority() int { return PriorityNormal }

type Remote_Tile_fillTileFloors_160 struct {
	X, Y, X2, Y2 int32
	Block        int16
	Raw          []byte
}

func (p *Remote_Tile_fillTileFloors_160) Read(r *Reader, length int) error {
	if length > 0 {
		raw, err := r.ReadBytes(length)
		if err != nil {
			return err
		}
		p.Raw = raw
		return nil
	}
	return nil
}

func (p *Remote_Tile_fillTileFloors_160) Write(w *Writer) error {
	if len(p.Raw) > 0 {
		return w.WriteBytes(p.Raw)
	}
	return nil
}

func (p *Remote_Tile_fillTileFloors_160) Priority() int { return PriorityNormal }

type Remote_Tile_fillTileOverlays_160 struct {
	X, Y, X2, Y2 int32
	Block        int16
	Raw          []byte
}

func (p *Remote_Tile_fillTileOverlays_160) Read(r *Reader, length int) error {
	if length > 0 {
		raw, err := r.ReadBytes(length)
		if err != nil {
			return err
		}
		p.Raw = raw
		return nil
	}
	return nil
}

func (p *Remote_Tile_fillTileOverlays_160) Write(w *Writer) error {
	if len(p.Raw) > 0 {
		return w.WriteBytes(p.Raw)
	}
	return nil
}

func (p *Remote_Tile_fillTileOverlays_160) Priority() int { return PriorityNormal }

type Remote_Menus_hideMenuBuilder_160 struct {
	MenuID int32
	Raw    []byte
}

func (p *Remote_Menus_hideMenuBuilder_160) Read(r *Reader, length int) error {
	if length > 0 {
		raw, err := r.ReadBytes(length)
		if err != nil {
			return err
		}
		p.Raw = raw
		if len(raw) >= 4 {
			p.MenuID = int32(raw[0])<<24 | int32(raw[1])<<16 | int32(raw[2])<<8 | int32(raw[3])
		}
		return nil
	}
	id, err := r.ReadInt32()
	if err != nil {
		return err
	}
	p.MenuID = id
	return nil
}

func (p *Remote_Menus_hideMenuBuilder_160) Write(w *Writer) error {
	if len(p.Raw) > 0 {
		return w.WriteBytes(p.Raw)
	}
	return w.WriteInt32(p.MenuID)
}

func (p *Remote_Menus_hideMenuBuilder_160) Priority() int { return PriorityNormal }

type Remote_Menus_menuBuilder_160 struct {
	Raw []byte
}

func (p *Remote_Menus_menuBuilder_160) Read(r *Reader, length int) error {
	if length > 0 {
		raw, err := r.ReadBytes(length)
		if err != nil {
			return err
		}
		p.Raw = raw
	}
	return nil
}

func (p *Remote_Menus_menuBuilder_160) Write(w *Writer) error {
	return w.WriteBytes(p.Raw)
}

func (p *Remote_Menus_menuBuilder_160) Priority() int { return PriorityNormal }

type Remote_Menus_menuBuilderChoose_160 struct {
	Raw []byte
}

func (p *Remote_Menus_menuBuilderChoose_160) Read(r *Reader, length int) error {
	if length > 0 {
		raw, err := r.ReadBytes(length)
		if err != nil {
			return err
		}
		p.Raw = raw
	}
	return nil
}

func (p *Remote_Menus_menuBuilderChoose_160) Write(w *Writer) error {
	return w.WriteBytes(p.Raw)
}

func (p *Remote_Menus_menuBuilderChoose_160) Priority() int { return PriorityNormal }

type Remote_Menus_menuBuilderUpdate_160 struct {
	Raw []byte
}

func (p *Remote_Menus_menuBuilderUpdate_160) Read(r *Reader, length int) error {
	if length > 0 {
		raw, err := r.ReadBytes(length)
		if err != nil {
			return err
		}
		p.Raw = raw
	}
	return nil
}

func (p *Remote_Menus_menuBuilderUpdate_160) Write(w *Writer) error {
	return w.WriteBytes(p.Raw)
}

func (p *Remote_Menus_menuBuilderUpdate_160) Priority() int { return PriorityNormal }
