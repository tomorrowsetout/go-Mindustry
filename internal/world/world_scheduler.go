package world

import "mdt-server/internal/sim"

type worldDispatcher interface {
	Dispatch(totalWork int, fn sim.DispatchFunc) sim.DispatchStats
	WorkerCount() int
	PartitionCount(totalWork int) int
}

const minParallelBatch = 64

func (w *World) SetScheduler(dispatcher worldDispatcher) {
	if w == nil {
		return
	}
	w.scheduler = dispatcher
}

func (w *World) clearScheduler() {
	if w == nil {
		return
	}
	w.scheduler = nil
}

func (w *World) parallelizeRanges(total int, fn func(partitionID, start, end int)) sim.DispatchStats {
	if fn == nil || total <= 0 {
		return sim.DispatchStats{}
	}
	if w == nil || w.scheduler == nil || w.scheduler.WorkerCount() <= 1 || total < minParallelBatch {
		fn(0, 0, total)
		return sim.DispatchStats{
			Partitions:  1,
			TotalWork:   total,
			WorkerCount: 1,
		}
	}
	return w.scheduler.Dispatch(total, func(p sim.Partition) {
		if p.Start >= p.End {
			return
		}
		fn(p.ID, p.Start, p.End)
	})
}
