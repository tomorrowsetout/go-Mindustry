package ai

import (
	"container/heap"
	"math"
)

// AStar A*路径查找算法
type AStar struct {
	Width  int32
	Height int32
	// 0 = 可通行, 1 = 阻塞
	Field []byte
}

// NewAStar 创建新的A*路径查找器
func NewAStar(width, height int32) *AStar {
	return &AStar{
		Width:  width,
		Height: height,
		Field:  make([]byte, width*height),
	}
}

// SetBlock 设置块位置
func (a *AStar) SetBlock(x, y int32, blocked bool) {
	if x < 0 || x >= a.Width || y < 0 || y >= a.Height {
		return
	}
	idx := y*a.Width + x
	if blocked {
		a.Field[idx] = 1
	} else {
		a.Field[idx] = 0
	}
}

// IsBlocked 检查位置是否阻塞
func (a *AStar) IsBlocked(x, y int32) bool {
	if x < 0 || x >= a.Width || y < 0 || y >= a.Height {
		return true
	}
	return a.Field[y*a.Width+x] == 1
}

// Path 路径
type Path struct {
	Nodes []Point
}

// Point 点
type Point struct {
	X, Y int32
}

// Distance 计算距离（曼哈顿距离）
func Distance(x1, y1, x2, y2 int32) float64 {
	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	return math.Abs(dx) + math.Abs(dy)
}

// FindPath 查找路径
func (a *AStar) FindPath(startX, startY, endX, endY int32) *Path {
	start := Point{X: startX, Y: startY}
	end := Point{X: endX, Y: endY}

	// 检查起始点和终点是否阻塞
	if a.IsBlocked(startX, startY) || a.IsBlocked(endX, endY) {
		return nil
	}

	// 打开集合（优先队列）
	openSet := &PriorityQueue{}
	heap.Init(openSet)

	// 关闭集合
	closedSet := make(map[Point]bool)

	// 父节点映射
	parents := make(map[Point]Point)

	// G值映射（从起点到当前点的实际代价）
	gScore := make(map[Point]float64)

	// F值映射（G + H）
	fScore := make(map[Point]float64)

	// 初始化起点
	startPoint := Point{X: startX, Y: startY}
	gScore[startPoint] = 0
	h := Distance(startX, startY, endX, endY)
	fScore[startPoint] = h
	heap.Push(openSet, &Item{
		Point: startPoint,
		F:     fScore[startPoint],
	})

	for openSet.Len() > 0 {
		// 取出F值最小的点
		current := heap.Pop(openSet).(*Item).Point

		// 如果到达终点
		if current.X == end.X && current.Y == end.Y {
			return a.reconstructPath(parents, current)
		}

		// 加入关闭集合
		closedSet[current] = true

		// 检查邻居（上下左右）
		neighbors := []Point{
			{X: current.X + 1, Y: current.Y},
			{X: current.X - 1, Y: current.Y},
			{X: current.X, Y: current.Y + 1},
			{X: current.X, Y: current.Y - 1},
		}

		for _, neighbor := range neighbors {
			// 检查是否在边界内
			if neighbor.X < 0 || neighbor.X >= a.Width || neighbor.Y < 0 || neighbor.Y >= a.Height {
				continue
			}

			// 检查是否阻塞
			if a.IsBlocked(neighbor.X, neighbor.Y) {
				continue
			}

			// 检查是否在关闭集合中
			if closedSet[neighbor] {
				continue
			}

			// 计算G值（移动代价为1）
			tentativeG := gScore[current] + 1

			// 如果不是新点或者找到更短路径
			_, exists := gScore[neighbor]
			if !exists || tentativeG < gScore[neighbor] {
				parents[neighbor] = current
				gScore[neighbor] = tentativeG
				h := Distance(neighbor.X, neighbor.Y, end.X, end.Y)
				fScore[neighbor] = tentativeG + h

				if !exists {
					heap.Push(openSet, &Item{
						Point: neighbor,
						F:     fScore[neighbor],
					})
				} else {
					// 更新优先队列
					openSet.Update(&neighbor, fScore[neighbor])
				}
			}
		}
	}

	// 没有找到路径
	return nil
}

// reconstructPath 重建路径
func (a *AStar) reconstructPath(parents map[Point]Point, current Point) *Path {
	path := &Path{
		Nodes: []Point{current},
	}

	for {
		if prev, ok := parents[current]; ok {
			path.Nodes = append([]Point{prev}, path.Nodes...)
			current = prev
		} else {
			break
		}
	}

	return path
}

// Item 优先队列项
type Item struct {
	Point Point
	F     float64
	index int
}

// PriorityQueue 优先队列
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].F < pq[j].F
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// Update 更新队列中的项
func (pq *PriorityQueue) Update(item *Item, f float64) {
	item.F = f
	heap.Fix(pq, item.index)
}
