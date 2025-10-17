package manager

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// 连接统计信息
type ConnectionStats struct {
	ActiveConnections int32 `json:"active_connections"`
	TotalConnections  int32 `json:"total_connections"`
	Errors            int32 `json:"errors"`
	MessagesForwarded int32 `json:"messages_forwarded"`
}

// 连接管理器
type ConnectionManager struct {
	mu         sync.RWMutex
	conns      map[*websocket.Conn]context.CancelFunc
	sem        chan struct{}
	shutdown   context.Context
	cancel     context.CancelFunc
	stats      ConnectionStats
	roomCache  sync.Map // 房间信息缓存
	danmuCache sync.Map // 弹幕信息缓存
	buvidCache sync.Map // buvid缓存
}

func NewConnectionManager(maxConns int) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConnectionManager{
		conns:    make(map[*websocket.Conn]context.CancelFunc),
		sem:      make(chan struct{}, maxConns),
		shutdown: ctx,
		cancel:   cancel,
	}
}

func (cm *ConnectionManager) Add(conn *websocket.Conn, cancel context.CancelFunc) bool {
	select {
	case cm.sem <- struct{}{}:
		cm.mu.Lock()
		cm.conns[conn] = cancel
		cm.mu.Unlock()
		atomic.AddInt32(&cm.stats.ActiveConnections, 1)
		atomic.AddInt32(&cm.stats.TotalConnections, 1)
		return true
	case <-cm.shutdown.Done():
		return false
	default:
		return false
	}
}

func (cm *ConnectionManager) Remove(conn *websocket.Conn) {
	cm.mu.Lock()
	if cancel, ok := cm.conns[conn]; ok {
		cancel()
		delete(cm.conns, conn)
	}
	cm.mu.Unlock()
	<-cm.sem
	atomic.AddInt32(&cm.stats.ActiveConnections, -1)
}

func (cm *ConnectionManager) CloseAll() {
	cm.cancel()
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for conn, cancel := range cm.conns {
		cancel()
		conn.Close()
	}
}

func (cm *ConnectionManager) Stats() ConnectionStats {
	return ConnectionStats{
		ActiveConnections: atomic.LoadInt32(&cm.stats.ActiveConnections),
		TotalConnections:  atomic.LoadInt32(&cm.stats.TotalConnections),
		Errors:            atomic.LoadInt32(&cm.stats.Errors),
		MessagesForwarded: atomic.LoadInt32(&cm.stats.MessagesForwarded),
	}
}

func (cm *ConnectionManager) IncrementErrors() {
	atomic.AddInt32(&cm.stats.Errors, 1)
}

func (cm *ConnectionManager) IncrementMessages() {
	atomic.AddInt32(&cm.stats.MessagesForwarded, 1)
}

// 缓存管理方法
func (cm *ConnectionManager) GetRoomCache(roomID int) (int, bool) {
	if val, ok := cm.roomCache.Load(roomID); ok {
		return val.(int), true
	}
	return 0, false
}

func (cm *ConnectionManager) SetRoomCache(roomID, realRoomID int) {
	cm.roomCache.Store(roomID, realRoomID)
}

func (cm *ConnectionManager) GetDanmuCache(realRoomID int) (interface{}, bool) {
	return cm.danmuCache.Load(realRoomID)
}

func (cm *ConnectionManager) SetDanmuCache(realRoomID int, data interface{}) {
	cm.danmuCache.Store(realRoomID, data)
}

func (cm *ConnectionManager) GetBuvidCache() (string, bool) {
	if val, ok := cm.buvidCache.Load("buvid3"); ok {
		return val.(string), true
	}
	return "", false
}

func (cm *ConnectionManager) SetBuvidCache(buvid3 string) {
	cm.buvidCache.Store("buvid3", buvid3)
}
