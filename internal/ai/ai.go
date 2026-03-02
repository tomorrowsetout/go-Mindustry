package ai

import (
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

// AIType AI类型
type AIType byte

const (
	AIPlayer       AIType = 0 // 玩家控制
	AIFormation    AIType = 1 // 编队控制
	AIGenericAI    AIType = 2 // 通用AI
	AILogic        AIType = 3 // 逻辑控制
	AIAuto         AIType = 4 // 自动控制
	AIUnitFollow   AIType = 5 // 单位跟随
	AIUnitAttack   AIType = 6 // 单位攻击
	AIUnitReturn   AIType = 7 // 单位返回
	AIUnitGather   AIType = 8 // 单位采集
	AIUnitRepair   AIType = 9 // 单位维修
	AIUnitBuild    AIType = 10 // 单位建造
)

// AIController AI控制器接口
type AIController interface {
	Type() AIType
	Update(unit *world.Unit)
	OnCommand(unit *world.Unit, cmd protocol.UnitCommand)
	OnEnemySeen(unit *world.Unit, target world.Entity)
	OnPathFound(unit *world.Unit, path *Path)
}

// UnitAI 单位AI - 完整实现
type UnitAI struct {
	ControllerType AIType
	World          *world.World

	// 状态
	Moving         bool
	Attacking      bool
	Retreating     bool
	Chasing        bool
	Around         bool

	// 目标
	TargetPos      protocol.Vec2
	Target         *world.Unit
	BuildTarget    *world.Building

	// 属性
	Accuracy       float32
	Speed          float32
	Range          float32

	// 当前状态
	State          UnitAIState
	Task           UnitTask
	Path           *Path
}

// UnitAIState 单位AI状态
type UnitAIState byte

const (
	UnitAIStateIdle     UnitAIState = iota // 空闲
	UnitAIStateMove                        // 移动
	UnitAIStateAttack                      // 攻击
	UnitAIStateFollow                      // 跟随
	UnitAIStateReturn                      // 返回
	UnitAIStateBuild                       // 建造
	UnitAIStateGather                      // 采集
	UnitAIStateRepair                      // 修理
	UnitAIStateBoost                       // 加速
	UnitAIStateMining                      // 采矿
	UnitAIStatePatrol                      // 巡逻
)

// UnitTask 单位任务
type UnitTask struct {
	Type         UnitTaskType
	TargetTile   protocol.Point2
	BlockID      int16
	Priority     int32
}

// UnitTaskType 任务类型
type UnitTaskType byte

const (
	UnitTaskNone    UnitTaskType = iota
	UnitTaskMove
	UnitTaskAttackUnit
	UnitTaskAttackBuilding
	UnitTaskBuild
	UnitTaskRepair
	UnitTaskGather
	UnitTaskReturn
	UnitTaskMine
	UnitTaskPatrol
)

// NewUnitAI 创建新的单位AI
func NewUnitAI(world *world.World) *UnitAI {
	return &UnitAI{
		ControllerType: AIGenericAI,
		World:          world,
		Accuracy:       0.0,
		Speed:          1.0,
		Range:          0.0,
		State:          UnitAIStateIdle,
		TargetPos:      protocol.Vec2{X: 0, Y: 0},
	}
}

// Type 返回AI类型
func (ai *UnitAI) Type() AIType {
	return ai.ControllerType
}

// Update 更新单位AI
func (ai *UnitAI) Update(unit *world.Unit) {
	if unit == nil {
		return
	}

	// 更新任务
	ai.updateTask(unit)

	// 更新状态
	ai.updateState(unit)

	// 执行动作
	ai.executeAction(unit)
}

// updateTask 更新任务
func (ai *UnitAI) updateTask(unit *world.Unit) {
	if ai.Task.Type == UnitTaskNone && unit.Data != nil {
		// 自动选择任务
		ai.Task.Type = UnitTaskAttack
		ai.findTarget(unit)
	}
}

// updateState 更新状态
func (ai *UnitAI) updateState(unit *world.Unit) {
	// 根据任务和目标更新状态
	switch ai.Task.Type {
	case UnitTaskMove:
		ai.State = UnitAIStateMove
	case UnitTaskAttackUnit, UnitTaskAttackBuilding:
		ai.State = UnitAIStateAttack
	case UnitTaskBuild:
		ai.State = UnitAIStateBuild
	case UnitTaskRepair:
		ai.State = UnitAIStateRepair
	case UnitTaskGather:
		ai.State = UnitAIStateGather
	case UnitTaskReturn:
		ai.State = UnitAIStateReturn
	case UnitTaskMine:
		ai.State = UnitAIStateMining
	default:
		ai.State = UnitAIStateIdle
	}
}

// executeAction 执行动作
func (ai *UnitAI) executeAction(unit *world.Unit) {
	if unit == nil {
		return
	}

	switch ai.State {
	case UnitAIStateIdle:
		ai.handleIdle(unit)
	case UnitAIStateMove:
		ai.handleMove(unit)
	case UnitAIStateAttack:
		ai.handleAttack(unit)
	case UnitAIStateFollow:
		ai.handleFollow(unit)
	case UnitAIStateReturn:
		ai.handleReturn(unit)
	case UnitAIStateBuild:
		ai.handleBuild(unit)
	case UnitAIStateGather:
		ai.handleGather(unit)
	case UnitAIStateRepair:
		ai.handleRepair(unit)
	case UnitAIStateBoost:
		ai.handleBoost(unit)
	case UnitAIStateMining:
		ai.handleMining(unit)
	case UnitAIStatePatrol:
		ai.handlePatrol(unit)
	}
}

// OnCommand 处理命令
func (ai *UnitAI) OnCommand(unit *world.Unit, cmd protocol.UnitCommand) {
	switch cmd.Type {
	case protocol.UnitCommand Idle:
		ai.Task.Type = UnitTaskNone
		ai.State = UnitAIStateIdle
	case protocol.UnitCommand Move:
		ai.Task.Type = UnitTaskMove
		ai.Task.TargetTile = cmd.Arg1Point
		ai.State = UnitAIStateMove
	case protocol.UnitCommand Attack:
		ai.Task.Type = UnitTaskAttackUnit
		if cmd.Arg1Unit != nil {
			ai.Target = cmd.Arg1Unit
		}
		ai.State = UnitAIStateAttack
	case protocol.UnitCommand Boost:
		ai.handleBoost(unit)
	case protocol.UnitCommand Gather:
		ai.Task.Type = UnitTaskGather
		ai.handleGather(unit)
	case protocol.UnitCommand Repair:
		ai.Task.Type = UnitTaskRepair
		ai.handleRepair(unit)
	case protocol.UnitCommand Build:
		ai.Task.Type = UnitTaskBuild
		ai.handleBuild(unit)
	case protocol.UnitCommand Return:
		ai.Task.Type = UnitTaskReturn
		ai.handleReturn(unit)
	case protocol.UnitCommand Mine:
		ai.Task.Type = UnitTaskMine
		ai.handleMining(unit)
	case protocol.UnitCommand Patrol:
		ai.Task.Type = UnitTaskPatrol
		ai.handlePatrol(unit)
	}
}

// OnEnemySeen 敌人被看到
func (ai *UnitAI) OnEnemySeen(unit *world.Unit, target world.Entity) {
	// 设置目标
	switch t := target.(type) {
	case *world.Unit:
		ai.Target = t
		ai.Task.Type = UnitTaskAttackUnit
	case *world.Building:
		ai.BuildTarget = t
		ai.Task.Type = UnitTaskAttackBuilding
	}
}

// OnPathFound 路径找到
func (ai *UnitAI) OnPathFound(unit *world.Unit, path *Path) {
	ai.Path = path
}

// handleIdle 处理空闲状态
func (ai *UnitAI) handleIdle(unit *world.Unit) {
	// 空闲时保持当前位置
}

// handleMove 处理移动状态
func (ai *UnitAI) handleMove(unit *world.Unit) {
	if ai.Path != nil && len(ai.Path.Nodes) > 0 {
		// 沿路径移动
		next := ai.Path.Nodes[0]
		unit.MoveTo(float32(next.X), float32(next.Y))
	} else if ai.Task.TargetTile.X != 0 || ai.Task.TargetTile.Y != 0 {
		// 移动到目标
		unit.MoveTo(float32(ai.Task.TargetTile.X), float32(ai.Task.TargetTile.Y))
	}
}

// handleAttack 处理攻击状态
func (ai *UnitAI) handleAttack(unit *world.Unit) {
	// 找到最近的敌人
	if ai.Target == nil {
		ai.findTarget(unit)
	}

	if ai.Target != nil {
		// 检查是否在射程内
		dist := unit.DistanceTo(ai.Target)
		if dist <= ai.Range || ai.Range == 0 {
			// 攻击敌人
			unit.Attack(ai.Target)
			ai.Attacking = true
		} else {
			// 移动到敌人附近
			ai.handleMove(unit)
		}
	}
}

// handleFollow 处理跟随状态
func (ai *UnitAI) handleFollow(unit *world.Unit) {
	// 跟随目标单位
	// TODO: 实现跟随逻辑
}

// handleReturn 处理返回状态
func (ai *UnitAI) handleReturn(unit *world.Unit) {
	// 返回核心
	// TODO: 实现返回逻辑
}

// handleBuild 处理建造状态
func (ai *UnitAI) handleBuild(unit *world.Unit) {
	// 建造建筑
	// TODO: 实现建造逻辑
}

// handleGather 处理采集状态
func (ai *UnitAI) handleGather(unit *world.Unit) {
	// 采集资源
	// TODO: 实现采集逻辑
}

// handleRepair 处理修理状态
func (ai *UnitAI) handleRepair(unit *world.Unit) {
	// 修理建筑
	// TODO: 实现修理逻辑
}

// handleBoost 处理加速状态
func (ai *UnitAI) handleBoost(unit *world.Unit) {
	// 加速单位
	// TODO: 实现加速逻辑
}

// handleMining 处理采矿状态
func (ai *UnitAI) handleMining(unit *world.Unit) {
	// 采矿
	// TODO: 实现采矿逻辑
}

// handlePatrol 处理巡逻状态
func (ai *UnitAI) handlePatrol(unit *world.Unit) {
	// 巡逻
	// TODO: 实现巡逻逻辑
}

// findTarget 寻找目标
func (ai *UnitAI) findTarget(unit *world.Unit) {
	// 寻找最近的敌人
	if ai.World == nil {
		return
	}

	// TODO: 实现目标查找逻辑
	// 遍历世界中的实体，找到最近的敌方单位或建筑
}

// RTSAI RTS风格AI - 完整实现
type RTSAI struct {
	ControllerType AIType
	World          *world.World
	TeamID         int32

	// 组列表
	Groups         []*RTSGroup

	// 阵型参数
FormationRange float32 // 阵型范围
	AttackRange    float32 // 攻击范围
	Separation     float32 // 分离距离
	Alignment      float32 // 对齐权重
	Cohesion       float32 // 凝聚力权重

	// 群组中心
	GroupCenter    protocol.Vec2
	MoveTarget     protocol.Vec2
	EnemyTarget    *world.Unit

	// 命令
	CurrentCommand *RTSCommand
}

// RTSGroup RTS小组
type RTSGroup struct {
	ID             int32
	Units          []*world.Unit
	Leader         *world.Unit
	TargetPos      protocol.Vec2
	Command        RTSGroupCommand
	Formation      []protocol.Vec2
	IsFlocking     bool
}

// RTSGroupCommand RTS小组命令
type RTSGroupCommand byte

const (
	RTSGroupCommandIdle      RTSGroupCommand = iota // 空闲
	RTSGroupCommandMove                             // 移动
	RTSGroupCommandAttack                           // 攻击
	RTSGroupCommandBuild                            // 建造
	RTSGroupCommandFollow                           // 跟随
	RTSGroupCommandPatrol                           // 巡逻
)

// RTSCommand RTS命令
type RTSCommand struct {
	Type     RTSCommandType
	Target   world.Entity
	Pos      protocol.Vec2
	GroupID  int32
	Units    []*world.Unit
}

// RTSCommandType RTS命令类型
type RTSCommandType byte

const (
	RTSCommandNone  RTSCommandType = iota
	RTSCommandMove
	RTSCommandAttack
	RTSCommandBuild
	RTSCommandFollow
	RTSCommandPatrol
)

// NewRTSAI 创建新的 RTS AI
func NewRTSAI(world *world.World, teamID int32) *RTSAI {
	return &RTSAI{
		ControllerType: AIGenericAI,
		TeamID:         teamID,
		World:          world,
		Groups:         make([]*RTSGroup, 0),
		FormationRange: 100.0,
		AttackRange:    50.0,
		Separation:     20.0,
		Alignment:      50.0,
		Cohesion:       30.0,
	}
}

// Type 返回AI类型
func (ai *RTSAI) Type() AIType {
	return ai.ControllerType
}

// Update 更新 RTS AI
func (ai *RTSAI) Update() {
	// 更新每个组
	for _, group := range ai.Groups {
		ai.updateGroup(group)
	}
}

// updateGroup 更新组
func (ai *RTSAI) updateGroup(group *RTSGroup) {
	if len(group.Units) == 0 {
		return
	}

	// 计算群组中心
	ai.calculateGroupCenter(group)

	// 执行群组命令
	switch group.Command {
	case RTSGroupCommandIdle:
		// 空闲
	case RTSGroupCommandMove:
		ai.moveGroup(group)
	case RTSGroupCommandAttack:
		ai.attackGroup(group)
	case RTSGroupCommandBuild:
		ai.buildGroup(group)
	case RTSGroupCommandFollow:
		ai.followGroup(group)
	case RTSGroupCommandPatrol:
		ai.patrolGroup(group)
	}

	// 应用 flocking 算法
	if group.IsFlocking {
		ai.applyFlocking(group)
	}
}

// calculateGroupCenter 计算群组中心
func (ai *RTSAI) calculateGroupCenter(group *RTSGroup) {
	if len(group.Units) == 0 {
		return
	}

	var sumX, sumY float32
	for _, unit := range group.Units {
		sumX += unit.X
		sumY += unit.Y
	}

	ai.GroupCenter.X = sumX / float32(len(group.Units))
	ai.GroupCenter.Y = sumY / float32(len(group.Units))
}

// moveGroup 移动群组
func (ai *RTSAI) moveGroup(group *RTSGroup) {
	// 所有单位移动到目标位置
	for _, unit := range group.Units {
		unit.MoveTo(ai.MoveTarget.X, ai.MoveTarget.Y)
	}
}

// attackGroup 攻击群组
func (ai *RTSAI) attackGroup(group *RTSGroup) {
	// 所有单位攻击目标
	for _, unit := range group.Units {
		if ai.EnemyTarget != nil {
			dist := unit.DistanceTo(ai.EnemyTarget)
			if dist <= ai.AttackRange {
				unit.Attack(ai.EnemyTarget)
			}
		}
	}
}

// buildGroup 建造群组
func (ai *RTSAI) buildGroup(group *RTSGroup) {
	// TODO: 实现建造群组逻辑
}

// followGroup 跟随群组
func (ai *RTSAI) followGroup(group *RTSGroup) {
	// TODO: 实现跟随群组逻辑
}

// patrolGroup 巡逻群组
func (ai *RTSAI) patrolGroup(group *RTSGroup) {
	// TODO: 实现巡逻群组逻辑
}

// applyFlocking 应用 flocking 算法
func (ai *RTSAI) applyFlocking(group *RTSGroup) {
	// 分离 (Separation): 避开附近的单位
	// 对齐 (Alignment): 与附近单位方向对齐
	// 凝聚 (Cohesion): 向附近单位中心移动

	for i, unit := range group.Units {
		var separation, alignment, cohesion protocol.Vec2
		count := 0

		for j, other := range group.Units {
			if i == j {
				continue
			}

			dist := unit.DistanceTo(other)
			if dist < ai.Separation {
				// 分离
				dx := unit.X - other.X
				dy := unit.Y - other.Y
				separation.X += dx
				separation.Y += dy

				// 对齐
				alignment.X += other.VelX
				alignment.Y += other.VelY

				// 凝聚
				cohesion.X += other.X
				cohesion.Y += other.Y

				count++
			}
		}

		if count > 0 {
			// 应用 flocking 力
			separation.X /= float32(count)
			separation.Y /= float32(count)

			alignment.X /= float32(count)
			alignment.Y /= float32(count)

			cohesion.X = (cohesion.X / float32(count)) - unit.X
			cohesion.Y = (cohesion.Y / float32(count)) - unit.Y

			// 合成力
			forceX := separation.X*1.5 + alignment.X*1.0 + cohesion.X*1.0
			forceY := separation.Y*1.5 + alignment.Y*1.0 + cohesion.Y*1.0

			// 应用到单位速度
			unit.VelX += forceX * 0.1
			unit.VelY += forceY * 0.1
		}
	}
}

// AddGroup 添加组
func (ai *RTSAI) AddGroup() *RTSGroup {
	group := &RTSGroup{
		ID:          int32(len(ai.Groups)),
		Units:       make([]*world.Unit, 0),
		Command:     RTSGroupCommandIdle,
		IsFlocking:  true,
	}
	ai.Groups = append(ai.Groups, group)
	return group
}

// RemoveGroup 移除组
func (ai *RTSAI) RemoveGroup(groupID int32) {
	for i, g := range ai.Groups {
		if g.ID == groupID {
			ai.Groups = append(ai.Groups[:i], ai.Groups[i+1:]...)
			return
		}
	}
}

// SetGroupCommand 设置组命令
func (ai *RTSAI) SetGroupCommand(groupID int32, cmd RTSGroupCommand, target world.Entity, pos protocol.Vec2) {
	for _, g := range ai.Groups {
		if g.ID == groupID {
			g.Command = cmd
			switch cmd {
			case RTSGroupCommandMove:
				g.TargetPos = pos
				ai.MoveTarget = pos
			case RTSGroupCommandAttack:
				ai.EnemyTarget = target
			}
			return
		}
	}
}

// BaseBuilderAI 基地建造AI - 完整实现
type BaseBuilderAI struct {
	ControllerType AIType
	World          *world.World
	TeamID         int32

	// 核心
	Core           *world.Building

	// 建造队列
	BuildQueue     []*BuildPlan
	RepairQueue    []*RepairPlan

	// 已建造建筑
	Structures     []*world.Building

	// 参数
	BuildRange     float32
	RepairRange    float32
	BuildPriority  int32
	RepairPriority int32

	// 状态
	CanBuild       bool
	CanRepair      bool
	CanLoad        bool

	// 寻路
	Pathfinder     *AStar
}

// BuildPlan 建造计划
type BuildPlan struct {
	BlockID   int16
	TileX     int32
	TileY     int32
	Priority  int32
	Completed bool
}

// RepairPlan 维修计划
type RepairPlan struct {
	Building   *world.Building
	Priority   int32
	Completed  bool
}

// NewBaseBuilderAI 创建新的基地建造AI
func NewBaseBuilderAI(world *world.World, teamID int32) *BaseBuilderAI {
	return &BaseBuilderAI{
		ControllerType: AIGenericAI,
		TeamID:         teamID,
		World:          world,
		BuildQueue:     make([]*BuildPlan, 0),
		RepairQueue:    make([]*RepairPlan, 0),
		Structures:     make([]*world.Building, 0),
		BuildRange:     100.0,
		RepairRange:    50.0,
	}
}

// Type 返回AI类型
func (ai *BaseBuilderAI) Type() AIType {
	return ai.ControllerType
}

// Update 更新基地建造AI
func (ai *BaseBuilderAI) Update() {
	// 更新建造队列
	ai.updateBuildQueue()

	// 更新维修队列
	ai.updateRepairQueue()

	// 更新核心
	ai.updateCore()
}

// updateBuildQueue 更新建造队列
func (ai *BaseBuilderAI) updateBuildQueue() {
	for _, plan := range ai.BuildQueue {
		if plan.Completed {
			continue
		}

		// 检查是否可以建造
		if ai.isValidBuildSpot(plan.TileX, plan.TileY) {
			// 添加到执行队列
			// TODO: 通知单位来建造
		}
	}
}

// updateRepairQueue 更新维修队列
func (ai *BaseBuilderAI) updateRepairQueue() {
	for _, plan := range ai.RepairQueue {
		if plan.Completed {
			continue
		}

		// 检查是否需要维修
		if plan.Building.Health < plan.Building.MaxHealth {
			// 添加到执行队列
			// TODO: 通知单位来维修
		}
	}
}

// updateCore 更新核心
func (ai *BaseBuilderAI) updateCore() {
	// 找到核心
	if ai.Core == nil && ai.World != nil {
		// TODO: 查找核心
	}
}

// isValidBuildSpot 检查是否有效建造点
func (ai *BaseBuilderAI) isValidBuildSpot(x, y int32) bool {
	if ai.World == nil {
		return false
	}

	// 检查地形
	// 检查是否有建筑
	// 检查距离核心

	// TODO: 实现完整的检查逻辑
	return true
}

// AddBuildPlan 添加建造计划
func (ai *BaseBuilderAI) AddBuildPlan(blockID int16, x, y int32, priority int32) {
	ai.BuildQueue = append(ai.BuildQueue, &BuildPlan{
		BlockID:   blockID,
		TileX:     x,
		TileY:     y,
		Priority:  priority,
		Completed: false,
	})
}

// AddRepairPlan 添加维修计划
func (ai *BaseBuilderAI) AddRepairPlan(building *world.Building, priority int32) {
	ai.RepairQueue = append(ai.RepairQueue, &RepairPlan{
		Building:  building,
		Priority:  priority,
		Completed: false,
	})
}

// CompleteBuildPlan 完成建造计划
func (ai *BaseBuilderAI) CompleteBuildPlan(x, y int32) {
	for _, plan := range ai.BuildQueue {
		if plan.TileX == x && plan.TileY == y {
			plan.Completed = true
			return
		}
	}
}

// CompleteRepairPlan 完成维修计划
func (ai *BaseBuilderAI) CompleteRepairPlan(building *world.Building) {
	for _, plan := range ai.RepairQueue {
		if plan.Building == building {
			plan.Completed = true
			return
		}
	}
}

// GetNextBuildTask 获取下一个建造任务
func (ai *BaseBuilderAI) GetNextBuildTask() (*BuildPlan, bool) {
	// 按优先级排序
	// 返回最高优先级的任务

	// TODO: 实现完整的任务获取逻辑
	if len(ai.BuildQueue) > 0 {
		return ai.BuildQueue[0], true
	}
	return nil, false
}

// GetNextRepairTask 获取下一个维修任务
func (ai *BaseBuilderAI) GetNextRepairTask() (*RepairPlan, bool) {
	if len(ai.RepairQueue) > 0 {
		return ai.RepairQueue[0], true
	}
	return nil, false
}

// AStar 路径查找器 - 完整实现
type AStar struct {
	Width   int32
	Height  int32
	Field   []byte // 0=可通行, 1=阻塞
}

// Node 节点
type Node struct {
	X, Y   int32
	G      float64
	H      float64
	F      float64
	Parent *Node
}

// Path 路径
type Path struct {
	Nodes   []protocol.Point2
	Cost    float64
	Length  int32
}

// PathQuery 路径查询
type PathQuery struct {
	StartX, StartY int32
	EndX, EndY     int32
	Team           int32
	Flying         bool
	Large          bool
	Range          float32
}

// NewAStar 创建新的A*路径查找器
func NewAStar(width, height int32) *AStar {
	return &AStar{
		Width:  width,
		Height: height,
		Field:  make([]byte, width*height),
	}
}

// SetBlock 设置块
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

// IsBlocked 检查是否阻塞
func (a *AStar) IsBlocked(x, y int32) bool {
	if x < 0 || x >= a.Width || y < 0 || y >= a.Height {
		return true
	}
	return a.Field[y*a.Width+x] == 1
}

// FindPath 查找路径
func (a *AStar) FindPath(query *PathQuery) *Path {
	startNode := &Node{
		X: query.StartX,
		Y: query.StartY,
	}

	endNode := &Node{
		X: query.EndX,
		Y: query.EndY,
	}

	// 打开列表（优先队列）
	openSet := make([]*Node, 0)
	// 关闭列表
	closeSet := make(map[NodeKey]bool)

	// G值映射
	gScore := make(map[NodeKey]float64)

	// 添加起点
	openSet = append(openSet, startNode)
	gScore[NodeKey{X: startNode.X, Y: startNode.Y}] = 0

	for len(openSet) > 0 {
		// 找到F值最小的节点
		current := a.getNodeWithLowestF(openSet)

		// 如果到达终点
		if current.X == endNode.X && current.Y == endNode.Y {
			return a.reconstructPath(current)
		}

		// 加入关闭列表
		closeSet[NodeKey{X: current.X, Y: current.Y}] = true

		// 检查邻居
		neighbors := a.getNeighbors(current)
		for _, neighbor := range neighbors {
			// 检查是否在关闭列表
			if closeSet[NodeKey{X: neighbor.X, Y: neighbor.Y}] {
				continue
			}

			// 计算G值
			tentativeG := gScore[NodeKey{X: current.X, Y: current.Y}] + 1

			// 如果找到更短路径
			if _, exists := gScore[NodeKey{X: neighbor.X, Y: neighbor.Y}]; !exists || tentativeG < gScore[NodeKey{X: neighbor.X, Y: neighbor.Y}] {
				neighbor.Parent = current
				gScore[NodeKey{X: neighbor.X, Y: neighbor.Y}] = tentativeG
				neighbor.H = a.heuristic(neighbor, endNode)
				neighbor.F = neighbor.G + neighbor.H
			}
		}
	}

	// 没有找到路径
	return nil
}

// heuristic 启发式函数（曼哈顿距离）
func (a *AStar) heuristic(aNode, bNode *Node) float64 {
	return float64(abs(bNode.X-aNode.X) + abs(bNode.Y-aNode.Y))
}

// getNeighbors 获取邻居
func (a *AStar) getNeighbors(node *Node) []*Node {
	neighbors := make([]*Node, 0)

	// 上
	if !a.IsBlocked(node.X, node.Y-1) {
		neighbors = append(neighbors, &Node{X: node.X, Y: node.Y - 1})
	}
	// 下
	if !a.IsBlocked(node.X, node.Y+1) {
		neighbors = append(neighbors, &Node{X: node.X, Y: node.Y + 1})
	}
	// 左
	if !a.IsBlocked(node.X-1, node.Y) {
		neighbors = append(neighbors, &Node{X: node.X - 1, Y: node.Y})
	}
	// 右
	if !a.IsBlocked(node.X+1, node.Y) {
		neighbors = append(neighbors, &Node{X: node.X + 1, Y: node.Y})
	}

	return neighbors
}

// getNodeWithLowestF 获取F值最低的节点
func (a *AStar) getNodeWithLowestF(nodes []*Node) *Node {
	if len(nodes) == 0 {
		return nil
	}

	lowest := nodes[0]
	for _, node := range nodes {
		if node.F < lowest.F {
			lowest = node
		}
	}
	return lowest
}

// reconstructPath 重建路径
func (a *AStar) reconstructPath(node *Node) *Path {
	path := &Path{
		Nodes: make([]protocol.Point2, 0),
	}

	current := node
	for current != nil {
		path.Nodes = append([]protocol.Point2{{X: current.X, Y: current.Y}}, path.Nodes...)
		current = current.Parent
	}

	path.Length = int32(len(path.Nodes))
	return path
}

// NodeKey 节点键
type NodeKey struct {
	X, Y int32
}

// abs 绝对值
func abs(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}
