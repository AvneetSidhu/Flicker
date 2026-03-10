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
	"github.com/AvneetSidhu/Flicker/internal/sfu/vm"
	"github.com/google/uuid"
)

// make thread safe
type RoomManager struct {
	rooms                  map[string]*Room
	logger                 *slog.Logger
	roomsCreatedLifetime   uint32
	mu                     sync.RWMutex
	controlEventMessageBus chan<- *control.CommandEvent
	ingestManager          *ingest.IngestManager
	vmService              *vm.VMService
}

func NewRoomManager(logger *slog.Logger, controlEventMessageBus chan<- *control.CommandEvent, ingestManager *ingest.IngestManager, vmService *vm.VMService) *RoomManager {
	return &RoomManager{
		rooms:                  make(map[string]*Room),
		logger:                 logger,
		roomsCreatedLifetime:   0,
		mu:                     sync.RWMutex{},
		controlEventMessageBus: controlEventMessageBus,
		ingestManager:          ingestManager,
		vmService:              vmService,
	}
}

func (rm *RoomManager) GetRoom(roomID string) (session.Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	room, ok := rm.rooms[roomID]
	return room, ok
}

func (rm *RoomManager) CreateRoom(ctx context.Context, roomName string, creator string) (string, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if len(rm.rooms) >= 10 {
		return "", fmt.Errorf("Maximum rooms created, unable to create new room")
	}
	audioPort, _, videoPort, _, err := allocateRoomPorts()
	if err != nil {
		return "", fmt.Errorf("error allocating room ports: %w", err)
	}

	roomID := uuid.New().String()
	rm.logger.Info("Creating room", slog.Any("roomID", roomID), slog.String("creator", creator))

	room := NewRoom(rm.logger, roomID, roomName, creator, rm.controlEventMessageBus)

	rm.ingestManager.AddRoom(roomID, audioPort, videoPort, room, room.SetVMReady, ctx)

	go func() {
		if err := rm.vmService.CreateVM(ctx, roomID, audioPort, videoPort); err != nil {
			rm.logger.Error("failed to provision VM for room", "roomID", roomID, "err", err)
		}
	}()

	rm.rooms[room.RoomID] = room
	return roomID, nil
}

func (rm *RoomManager) DeleteRoom(roomID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, ok := rm.rooms[roomID]; !ok {
		return nil
	}

	rm.ingestManager.RemoveRoom(roomID)

	if err := rm.vmService.DestroyVM(roomID); err != nil {
		rm.logger.Error("failed to destroy VM for room", "roomID", roomID, "err", err)
	}

	delete(rm.rooms, roomID)
	rm.logger.Info("Removed room", slog.Any("roomID", roomID))
	return nil
}

type RoomInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Creator string `json:"creator"`
	Viewers int    `json:"viewers"`
	IsLive  bool   `json:"isLive"`
}

func (rm *RoomManager) GetRooms() []RoomInfo {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	rooms := make([]RoomInfo, 0, len(rm.rooms))
	for _, r := range rm.rooms {
		rooms = append(rooms, RoomInfo{
			ID:      r.RoomID,
			Name:    r.Name,
			Creator: r.Creator,
			Viewers: r.MemberCount(),
			IsLive:  r.IsVMReady(),
		})
	}
	return rooms
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
