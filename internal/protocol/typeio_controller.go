package protocol

// Controller type IDs (from Java readController):
// 0 player, 1 formation (ignored), 3 logic, 4/6/7/8/9 command, 5 assembler, 2 generic AI

func WriteController(w *Writer, ctrl UnitController) error {
	switch v := ctrl.(type) {
	case *ControllerState:
		return writeControllerState(w, v)
	default:
		return w.WriteByte(byte(ControllerGenericAI))
	}
}

func ReadController(r *Reader, _ UnitController) (UnitController, error) {
	typ, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	state := &ControllerState{Type: ControllerType(typ)}
	switch ControllerType(typ) {
	case ControllerPlayer:
		id, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		state.PlayerID = id
	case ControllerFormation:
		if _, err := r.ReadInt32(); err != nil {
			return nil, err
		}
	case ControllerLogic:
		pos, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		state.LogicPos = pos
	case ControllerCommand4, ControllerCommand6, ControllerCommand7, ControllerCommand8, ControllerCommand9:
		if err := readCommandState(r, state); err != nil {
			return nil, err
		}
	case ControllerAssembler:
		// no extra payload
	default:
		// unknown; leave blank
	}
	return state, nil
}

func writeControllerState(w *Writer, state *ControllerState) error {
	if state == nil {
		return w.WriteByte(byte(ControllerGenericAI))
	}
	if err := w.WriteByte(byte(state.Type)); err != nil {
		return err
	}
	switch state.Type {
	case ControllerPlayer:
		return w.WriteInt32(state.PlayerID)
	case ControllerFormation:
		return w.WriteInt32(0)
	case ControllerLogic:
		return w.WriteInt32(state.LogicPos)
	case ControllerCommand4, ControllerCommand6, ControllerCommand7, ControllerCommand8, ControllerCommand9:
		return writeCommandState(w, state)
	case ControllerAssembler:
		return nil
	default:
		return nil
	}
}

func readCommandState(r *Reader, state *ControllerState) error {
	hasAttack, err := r.ReadBool()
	if err != nil {
		return err
	}
	hasPos, err := r.ReadBool()
	if err != nil {
		return err
	}
	state.Command.HasAttack = hasAttack
	state.Command.HasPos = hasPos
	if hasPos {
		x, err := r.ReadFloat32()
		if err != nil {
			return err
		}
		y, err := r.ReadFloat32()
		if err != nil {
			return err
		}
		state.Command.TargetPos = Vec2{X: x, Y: y}
	}
	if hasAttack {
		etype, err := r.ReadByte()
		if err != nil {
			return err
		}
		id, err := r.ReadInt32()
		if err != nil {
			return err
		}
		state.Command.Target = CommandTarget{Type: etype, Pos: id}
	}
	if state.Type == ControllerCommand6 || state.Type == ControllerCommand7 || state.Type == ControllerCommand8 || state.Type == ControllerCommand9 {
		cmd, err := r.ReadByte()
		if err != nil {
			return err
		}
		state.Command.CommandID = int8(cmd)
	}
	if state.Type == ControllerCommand7 || state.Type == ControllerCommand8 || state.Type == ControllerCommand9 {
		qlen, err := r.ReadUByte()
		if err != nil {
			return err
		}
		state.Command.Queue = make([]CommandTarget, 0, qlen)
		for i := 0; i < int(qlen); i++ {
			ct, err := r.ReadByte()
			if err != nil {
				return err
			}
			switch ct {
			case 0:
				pos, err := r.ReadInt32()
				if err != nil {
					return err
				}
				state.Command.Queue = append(state.Command.Queue, CommandTarget{Type: 0, Pos: pos})
			case 1:
				id, err := r.ReadInt32()
				if err != nil {
					return err
				}
				state.Command.Queue = append(state.Command.Queue, CommandTarget{Type: 1, Pos: id})
			case 2:
				x, err := r.ReadFloat32()
				if err != nil {
					return err
				}
				y, err := r.ReadFloat32()
				if err != nil {
					return err
				}
				state.Command.Queue = append(state.Command.Queue, CommandTarget{Type: 2, Vec: Vec2{X: x, Y: y}})
			default:
				state.Command.Queue = append(state.Command.Queue, CommandTarget{Type: 3})
			}
		}
	}
	if state.Type == ControllerCommand8 {
		stance, err := ReadStance(r, nil)
		if err != nil {
			return err
		}
		state.Command.Stances = []UnitStance{stance}
	} else if state.Type == ControllerCommand9 {
		cnt, err := r.ReadUByte()
		if err != nil {
			return err
		}
		state.Command.Stances = make([]UnitStance, 0, cnt)
		for i := 0; i < int(cnt); i++ {
			stance, err := ReadStance(r, nil)
			if err != nil {
				return err
			}
			state.Command.Stances = append(state.Command.Stances, stance)
		}
	}
	return nil
}

func writeCommandState(w *Writer, state *ControllerState) error {
	cmd := state.Command
	if err := w.WriteBool(cmd.HasAttack); err != nil {
		return err
	}
	if err := w.WriteBool(cmd.HasPos); err != nil {
		return err
	}
	if cmd.HasPos {
		if err := w.WriteFloat32(cmd.TargetPos.X); err != nil {
			return err
		}
		if err := w.WriteFloat32(cmd.TargetPos.Y); err != nil {
			return err
		}
	}
	if cmd.HasAttack {
		if err := w.WriteByte(cmd.Target.Type); err != nil {
			return err
		}
		if err := w.WriteInt32(cmd.Target.Pos); err != nil {
			return err
		}
	}
	if state.Type == ControllerCommand6 || state.Type == ControllerCommand7 || state.Type == ControllerCommand8 || state.Type == ControllerCommand9 {
		if err := w.WriteByte(byte(cmd.CommandID)); err != nil {
			return err
		}
	}
	if state.Type == ControllerCommand7 || state.Type == ControllerCommand8 || state.Type == ControllerCommand9 {
		if err := w.WriteByte(byte(len(cmd.Queue))); err != nil {
			return err
		}
		for _, q := range cmd.Queue {
			if err := w.WriteByte(q.Type); err != nil {
				return err
			}
			switch q.Type {
			case 0, 1:
				if err := w.WriteInt32(q.Pos); err != nil {
					return err
				}
			case 2:
				if err := w.WriteFloat32(q.Vec.X); err != nil {
					return err
				}
				if err := w.WriteFloat32(q.Vec.Y); err != nil {
					return err
				}
			default:
				// no payload
			}
		}
	}
	if state.Type == ControllerCommand8 {
		if len(cmd.Stances) > 0 {
			if err := WriteStance(w, &cmd.Stances[0]); err != nil {
				return err
			}
		} else if err := WriteStance(w, nil); err != nil {
			return err
		}
	} else if state.Type == ControllerCommand9 {
		if err := w.WriteByte(byte(len(cmd.Stances))); err != nil {
			return err
		}
		for i := range cmd.Stances {
			if err := WriteStance(w, &cmd.Stances[i]); err != nil {
				return err
			}
		}
	}
	return nil
}
