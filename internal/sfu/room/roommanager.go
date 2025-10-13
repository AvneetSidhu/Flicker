package room

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/AvneetSidhu/Flicker/internal/sfu/control"
	"github.com/AvneetSidhu/Flicker/internal/sfu/ingest"
	"github.com/AvneetSidhu/Flicker/internal/sfu/session"
)

// make thread safe
type RoomManager struct {
	rooms                  map[string]*Room
	logger                 *slog.Logger
	roomsCreatedLifetime   uint32
	mu                     sync.RWMutex
	controlEventMessageBus chan<- *control.CommandEvent
	ingestManager          *ingest.IngestManager
}

func NewRoomManager(logger *slog.Logger, controlEventMessageBus chan<- *control.CommandEvent, ingestManager *ingest.IngestManager) *RoomManager {
	return &RoomManager{
		rooms:                  make(map[string]*Room),
		logger:                 logger,
		roomsCreatedLifetime:   0,
		mu:                     sync.RWMutex{},
		controlEventMessageBus: controlEventMessageBus,
		ingestManager:          ingestManager,
	}
}

func (rm *RoomManager) GetRoom(roomID string) (session.Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	room, ok := rm.rooms[roomID]
	return room, ok
}

func (rm *RoomManager) CreateRoom(ctx context.Context, roomName string, creator string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if len(rm.rooms) >= 10 {
		return fmt.Errorf("Maximum rooms created, unable to create new room")
	}
	audioPort, _, videoPort, _, err := allocateRoomPorts()
	if err != nil {
		return fmt.Errorf("error allocating room ports: %w", err)
	}

	roomID := "12345"
	rm.logger.Info("Creating room", slog.Any("roomID", roomID), slog.String("creator", creator))

	room := NewRoom(rm.logger, roomID, rm.controlEventMessageBus)

	rm.ingestManager.AddRoom(roomID, audioPort, videoPort, room, ctx)

	//only call broadcast when room ingest is live. room.broadcast(ctx)
	//vm spin up go routine here, async ask for ingest and hook up the agent to ingest and grpc before showing the room is joinable
	rm.rooms[room.RoomID] = room
	return nil
}

func (rm *RoomManager) DeleteRoom(roomID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, ok := rm.rooms[roomID]; !ok {
		return nil
	}

	rm.ingestManager.RemoveRoom(roomID)

	delete(rm.rooms, roomID)
	rm.logger.Info("Removed room", slog.Any("roomID", roomID))
	return nil
}

func allocateRoomPorts() (audioRTP, audioRTCP, videoRTP, videoRTCP int, err error) {
	audioRTP, err = findFreeUDPPort()
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to allocate audio RTP port: %w", err)
	}
	audioRTCP = audioRTP + 1

	for {
		videoRTP, err = findFreeUDPPort()
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("failed to allocate video RTP port: %w", err)
		}
		videoRTCP = videoRTP + 1

		if videoRTP != audioRTCP {
			break
		}

	}

	return audioRTP, audioRTCP, videoRTP, videoRTCP, nil
}

func findFreeUDPPort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	if err != nil {
		return 0, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return 0, fmt.Errorf("failed to listen on port 0: %w", err)
	}
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	return port, nil
}
