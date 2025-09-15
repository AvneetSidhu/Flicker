package room

import (
	"log/slog"
	"sync"

	"github.com/pion/rtp"
)

// make thread safe
type RoomManager struct {
	rooms                map[uint32]*Room
	logger               *slog.Logger
	roomsCreatedLifetime uint32
	mu                   sync.RWMutex
}

func NewRoomManager(logger *slog.Logger) *RoomManager {
	return &RoomManager{
		rooms:                make(map[uint32]*Room),
		logger:               logger,
		roomsCreatedLifetime: 0,
		mu:                   sync.RWMutex{},
	}
}

func (rm *RoomManager) GetRoom(roomId uint32) (*Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	room, ok := rm.rooms[roomId]
	return room, ok
}

func (rm *RoomManager) AddRoom(creator string, mediaChan chan *rtp.Packet) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if len(rm.rooms) >= 10 {
		return nil
	}
	rm.logger.Info("Creating room", slog.Any("roomId", rm.roomsCreatedLifetime), slog.String("creator", creator))

	room := NewRoom(rm.logger, rm.roomsCreatedLifetime, mediaChan)

	rm.roomsCreatedLifetime++

	rm.rooms[room.RoomId] = room
	return nil
}

func (rm *RoomManager) RemoveRoom(roomId uint32) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, ok := rm.rooms[roomId]; !ok {
		return nil
	}
	delete(rm.rooms, roomId)
	rm.logger.Info("Removed room", slog.Any("roomId", roomId))
	return nil
}
