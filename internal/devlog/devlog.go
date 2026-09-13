package devlog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志等级
type LogLevel int

const (
	LogLevelTrace LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
	LogLevelError
	LogLevelNone
)

func (l LogLevel) String() string {
	switch l {
	case LogLevelTrace:
		return "TRACE"
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogEntry 日志条目
type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Category  string
	Message   string
	Fields    map[string]any
	ConnID    int32
	PlayerID  int32
	PacketID  int
	PacketName string
}

// DevLogger 开发者日志记录器
type DevLogger struct {
	mu       sync.Mutex
	out      io.Writer
	minLevel LogLevel
	enabled  bool
}

// New 创建新的开发者日志记录器
func New(out io.Writer) *DevLogger {
	if out == nil {
		out = os.Stdout
	}
	return &DevLogger{
		out:      out,
		minLevel: LogLevelDebug,
		enabled:  true,
	}
}

// SetLevel 设置日志等级
func (dl *DevLogger) SetLevel(level LogLevel) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.minLevel = level
}

// Enable 启用日志
func (dl *DevLogger) Enable(enabled bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.enabled = enabled
}

// ShouldLog 检查是否应该记录
func (dl *DevLogger) ShouldLog(level LogLevel) bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.enabled && level >= dl.minLevel
}

// Log 记录日志
func (dl *DevLogger) Log(entry LogEntry) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if !dl.enabled || entry.Level < dl.minLevel {
		return
	}

	payload := map[string]any{
		"ts":       entry.Timestamp.UTC().Format(time.RFC3339Nano),
		"level":    entry.Level.String(),
		"category": entry.Category,
		"msg":      entry.Message,
	}

	if entry.ConnID != 0 {
		payload["conn_id"] = entry.ConnID
	}
	if entry.PlayerID != 0 {
		payload["player_id"] = entry.PlayerID
	}
	if entry.PacketID != 0 {
		payload["packet_id"] = entry.PacketID
	}
	if entry.PacketName != "" {
		payload["packet_name"] = entry.PacketName
	}

	if len(entry.Fields) > 0 {
		payload["fields"] = entry.Fields
	}

	// 格式化输出
	enc := json.NewEncoder(dl.out)
	_ = enc.Encode(payload)
}

// Trace 记录跟踪日志
func (dl *DevLogger) Trace(category, message string, fields ...Field) {
	if dl.ShouldLog(LogLevelTrace) {
		dl.Log(LogEntry{
			Timestamp: time.Now(),
			Level:     LogLevelTrace,
			Category:  category,
			Message:   message,
			Fields:    buildFields(fields...),
		})
	}
}

// Debug 记录调试日志
func (dl *DevLogger) Debug(category, message string, fields ...Field) {
	if dl.ShouldLog(LogLevelDebug) {
		dl.Log(LogEntry{
			Timestamp: time.Now(),
			Level:     LogLevelDebug,
			Category:  category,
			Message:   message,
			Fields:    buildFields(fields...),
		})
	}
}

// Info 记录信息日志
func (dl *DevLogger) Info(category, message string, fields ...Field) {
	if dl.ShouldLog(LogLevelInfo) {
		dl.Log(LogEntry{
			Timestamp: time.Now(),
			Level:     LogLevelInfo,
			Category:  category,
			Message:   message,
			Fields:    buildFields(fields...),
		})
	}
}

// Warn 记录警告日志
func (dl *DevLogger) Warn(category, message string, fields ...Field) {
	if dl.ShouldLog(LogLevelWarn) {
		dl.Log(LogEntry{
			Timestamp: time.Now(),
			Level:     LogLevelWarn,
			Category:  category,
			Message:   message,
			Fields:    buildFields(fields...),
		})
	}
}

// Error 记录错误日志
func (dl *DevLogger) Error(category, message string, fields ...Field) {
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelError,
		Category:  category,
		Message:   message,
		Fields:    buildFields(fields...),
	})
}

// Field 日志字段
type Field struct {
	Key   string
	Value any
}

func buildFields(fields ...Field) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	result := make(map[string]any)
	for _, f := range fields {
		if f.Key != "" {
			result[f.Key] = f.Value
		}
	}
	return result
}

// Helper functions for common log patterns

// LogPacketReceived 记录接收到的数据包
func (dl *DevLogger) LogPacketReceived(connID, playerID, packetID int32, packetName, details string) {
	dl.Log(LogEntry{
		Timestamp:  time.Now(),
		Level:      LogLevelDebug,
		Category:   "packet",
		Message:    fmt.Sprintf("packet received from conn=%d", connID),
		Fields:     map[string]any{"packet_id": packetID, "packet_name": packetName},
		ConnID:     connID,
		PlayerID:   playerID,
		PacketID:   int(packetID),
		PacketName: packetName,
	})
}

// LogPacketSent 记录发送的数据包
func (dl *DevLogger) LogPacketSent(connID, playerID, packetID int32, packetName, details string) {
	dl.Log(LogEntry{
		Timestamp:  time.Now(),
		Level:      LogLevelDebug,
		Category:   "packet",
		Message:    fmt.Sprintf("packet sent to conn=%d", connID),
		Fields:     map[string]any{"packet_id": packetID, "packet_name": packetName},
		ConnID:     connID,
		PlayerID:   playerID,
		PacketID:   int(packetID),
		PacketName: packetName,
	})
}

// LogEvent 记录游戏事件
func (dl *DevLogger) LogEvent(category, eventType, details string) {
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelDebug,
		Category:  category,
		Message:   fmt.Sprintf("event: %s", eventType),
		Fields:    map[string]any{"details": details},
	})
}

// LogConnection 记录连接事件（增强版）
func (dl *DevLogger) LogConnection(event string, connID int32, remoteIP, name, uuid string, fields ...Field) {
	msg := fmt.Sprintf("%s conn=%d remote=%s name=%q uuid=%s", event, connID, remoteIP, name, uuid)
	
	logFields := map[string]any{
		"event":      event,
		"conn_id":    connID,
		"remote_ip":  remoteIP,
		"name":       name,
		"uuid":       uuid,
	}
	
	// 添加额外字段
	for _, f := range fields {
		if f.Key != "" {
			logFields[f.Key] = f.Value
		}
	}
	
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  "net",
		Message:   msg,
		Fields:    logFields,
		ConnID:    connID,
	})
}

// LogEntity 记录实体事件
func (dl *DevLogger) LogEntity(entityType string, entityID int32, action string, fields ...Field) {
	msg := fmt.Sprintf("%s entity id=%d %s", entityType, entityID, action)
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelDebug,
		Category:  "entity",
		Message:   msg,
		Fields:    buildFields(append([]Field{{Key: "action", Value: action}}, fields...)...),
	})
}

// LogPlayer 记录玩家事件
func (dl *DevLogger) LogPlayer(playerID int32, name, event string, fields ...Field) {
	msg := fmt.Sprintf("player id=%d name=%q %s", playerID, name, event)
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  "player",
		Message:   msg,
		Fields:    buildFields(append([]Field{{Key: "event", Value: event}}, fields...)...),
		PlayerID:  playerID,
	})
}

// LogWave 记录波次事件
func (dl *DevLogger) LogWave(wave int, eventType string, fields ...Field) {
	msg := fmt.Sprintf("wave %d %s", wave, eventType)
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelInfo,
		Category:  "wave",
		Message:   msg,
		Fields:    buildFields(append([]Field{{Key: "wave", Value: wave}}, fields...)...),
	})
}

// LogBuild 记录建造事件
func (dl *DevLogger) LogBuild(x, y int32, blockID int16, team string, action string, fields ...Field) {
	msg := fmt.Sprintf("build at (%d,%d) block=%d team=%s %s", x, y, blockID, team, action)
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelDebug,
		Category:  "build",
		Message:   msg,
		Fields:    buildFields(append([]Field{
			{Key: "x", Value: x},
			{Key: "y", Value: y},
			{Key: "block_id", Value: blockID},
			{Key: "team", Value: team},
			{Key: "action", Value: action},
		}, fields...)...),
	})
}

// LogUnit 记录单位事件（增强版 - 记录完整数据）
func (dl *DevLogger) LogUnit(unitID int32, unitType string, action string, fields ...Field) {
	// 获取完整单位数据
	unitData := map[string]any{
		"unit_id":    unitID,
		"unit_type":  unitType,
		"action":     action,
	}
	
	// 添加额外字段
	for _, f := range fields {
		if f.Key != "" {
			unitData[f.Key] = f.Value
		}
	}
	
	msg := fmt.Sprintf("unit id=%d type=%s %s", unitID, unitType, action)
	
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelDebug,
		Category:  "unit",
		Message:   msg,
		Fields:    unitData,
	})
}

// LogUnitFull 记录完整的单位同步数据
func (dl *DevLogger) LogUnitFull(unitID int32, unitData map[string]any, action string) {
	msg := fmt.Sprintf("unit id=%d %s", unitID, action)
	
	// 添加详细数据
	logFields := map[string]any{
		"unit_id": unitID,
		"action":  action,
	}
	for k, v := range unitData {
		logFields[k] = v
	}
	
	dl.Log(LogEntry{
		Timestamp: time.Now(),
		Level:     LogLevelDebug,
		Category:  "unit",
		Message:   msg,
		Fields:    logFields,
	})
}

// StringMapFld 构建字段映射
func StringMapFld(m map[string]any) Field {
	return Field{Key: "fields", Value: m}
}

//BoolFld 布尔字段
func BoolFld(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

//Int32Fld 32位整数字段
func Int32Fld(key string, value int32) Field {
	return Field{Key: key, Value: value}
}

//Float32Fld 32位浮点字段
func Float32Fld(key string, value float32) Field {
	return Field{Key: key, Value: value}
}

//StringFld 字符串字段
func StringFld(key, value string) Field {
	return Field{Key: key, Value: value}
}

//Int16Fld 16位整数字段
func Int16Fld(key string, value int16) Field {
	return Field{Key: key, Value: int32(value)}
}

//IntFld 整数字段（别名）
func IntFld(key string, value int) Field {
	return Field{Key: key, Value: int32(value)}
}

//PrintDebugHeader 打印调试头信息
func (dl *DevLogger) PrintDebugHeader() {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("🎮 MDT-SERVER 开发者日志")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("oggles: DEBUG (showing levels >= DEBUG)\n")
	fmt.Println(strings.Repeat("-", 80))
}
