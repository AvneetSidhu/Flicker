package room

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/AvneetSidhu/Flicker/internal/sfu/control"
	"github.com/AvneetSidhu/Flicker/internal/sfu/ingest"
	"github.com/AvneetSidhu/Flicker/internal/sfu/session"
	"github.com/google/uuid"
)

const emptyRoomTimeout = 2 * time.Minute

var (
	ErrRoomNotFound = errors.New("room not found")
	ErrNotRoomOwner = errors.New("room deletion requires owner")
)

// make thread safe
type RoomManager struct {
	rooms                  map[string]*Room
	logger                 *slog.Logger
	roomsCreatedLifetime   uint32
	mu                     sync.RWMutex
	controlEventMessageBus chan<- *control.CommandEvent
	ingestManager          ingestManager
	vmService              vmService
	vmCancels              map[string]context.CancelFunc
}

type ingestManager interface {
	AddRoom(roomID string, audioListener net.PacketConn, videoListener net.PacketConn, room ingest.Room, onReady func(), ctx context.Context)
	RemoveRoom(roomID string)
}

type vmService interface {
	CreateVM(ctx context.Context, roomID string, audioPort int, videoPort int) error
	DestroyVM(roomID string) error
}

func NewRoomManager(logger *slog.Logger, controlEventMessageBus chan<- *control.CommandEvent, ingestManager ingestManager, vmService vmService) *RoomManager {
	return newRoomManager(logger, controlEventMessageBus, ingestManager, vmService, true)
}

func newRoomManager(logger *slog.Logger, controlEventMessageBus chan<- *control.CommandEvent, ingestManager ingestManager, vmService vmService, startReaper bool) *RoomManager {
	rm := &RoomManager{
		rooms:                  make(map[string]*Room),
		logger:                 logger,
		roomsCreatedLifetime:   0,
		mu:                     sync.RWMutex{},
		controlEventMessageBus: controlEventMessageBus,
		ingestManager:          ingestManager,
		vmService:              vmService,
		vmCancels:              make(map[string]context.CancelFunc),
	}
	if startReaper {
		go rm.runEmptyRoomReaper()
	}
	return rm
}

func (rm *RoomManager) runEmptyRoomReaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		rm.mu.RLock()
		var toDelete []string
		for id, r := range rm.rooms {
			if r.EmptyDuration() >= emptyRoomTimeout {
				toDelete = append(toDelete, id)
			}
		}
		rm.mu.RUnlock()

		for _, id := range toDelete {
			rm.logger.Info("reaping empty room", "roomID", id)
			rm.DeleteRoom(id)
		}
	}
}

func (rm *RoomManager) GetRoom(roomID string) (session.Room, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	room, ok := rm.rooms[roomID]
	return room, ok
}

func (rm *RoomManager) CreateRoom(ctx context.Context, roomName string, creator string, ownerID string) (string, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if len(rm.rooms) >= 10 {
		return "", fmt.Errorf("Maximum rooms created, unable to create new room")
	}
	audioListener, videoListener, err := allocateRoomListeners()
	if err != nil {
		return "", fmt.Errorf("error allocating room ports: %w", err)
	}
	audioPort := audioListener.LocalAddr().(*net.UDPAddr).Port
	videoPort := videoListener.LocalAddr().(*net.UDPAddr).Port

	roomID := uuid.New().String()
	rm.logger.Info("Creating room", slog.Any("roomID", roomID), slog.String("creator", creator))

	room := NewRoom(rm.logger, roomID, roomName, creator, ownerID, rm.controlEventMessageBus)

	rm.ingestManager.AddRoom(roomID, audioListener, videoListener, room, room.SetVMReady, ctx)
	vmCtx, vmCancel := context.WithCancel(ctx)
	rm.vmCancels[roomID] = vmCancel

	go func() {
		if err := rm.vmService.CreateVM(vmCtx, roomID, audioPort, videoPort); err != nil {
			if vmCtx.Err() == nil {
				rm.markProvisioningFailed(roomID, err)
			}
		}
		rm.mu.Lock()
		delete(rm.vmCancels, roomID)
		rm.mu.Unlock()
	}()

	rm.rooms[room.RoomID] = room
	return roomID, nil
}

func (rm *RoomManager) markProvisioningFailed(roomID string, err error) {
	rm.mu.RLock()
	room, ok := rm.rooms[roomID]
	rm.mu.RUnlock()
	if !ok {
		return
	}

	rm.logger.Error("failed to provision VM for room", "roomID", roomID, "err", err)
	rm.ingestManager.RemoveRoom(roomID)

	rm.mu.Lock()
	if currentRoom, ok := rm.rooms[roomID]; ok && currentRoom == room {
		room.SetVMFailed(err)
	}
	rm.mu.Unlock()
}

func (rm *RoomManager) DeleteRoom(roomID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.deleteRoomLocked(roomID)
}

func (rm *RoomManager) DeleteRoomForOwner(roomID string, ownerID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	room, ok := rm.rooms[roomID]
	if !ok {
		return ErrRoomNotFound
	}
	if ownerID == "" || room.OwnerID != ownerID {
		return ErrNotRoomOwner
	}
	return rm.deleteRoomLocked(roomID)
}

func (rm *RoomManager) deleteRoomLocked(roomID string) error {
	if _, ok := rm.rooms[roomID]; !ok {
		return ErrRoomNotFound
	}

	rm.logger.Info("DeleteRoom: removing ingest", "roomID", roomID)
	if cancel, ok := rm.vmCancels[roomID]; ok {
		cancel()
		delete(rm.vmCancels, roomID)
	}
	rm.ingestManager.RemoveRoom(roomID)

	rm.logger.Info("DeleteRoom: destroying VM", "roomID", roomID)
	if err := rm.vmService.DestroyVM(roomID); err != nil {
		rm.logger.Error("failed to destroy VM for room", "roomID", roomID, "err", err)
	}

	delete(rm.rooms, roomID)
	rm.logger.Info("DeleteRoom: done", "roomID", roomID)
	return nil
}

type RoomInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Creator string  `json:"creator"`
	OwnerID string  `json:"ownerId"`
	Viewers int     `json:"viewers"`
	IsLive  bool    `json:"isLive"`
	VMState VMState `json:"vmState"`
	Failure string  `json:"failure,omitempty"`
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
			OwnerID: r.OwnerID,
			Viewers: r.MemberCount(),
			IsLive:  r.IsVMReady(),
			VMState: r.VMState(),
			Failure: r.VMFailure(),
		})
	}
	return rooms
}

func allocateRoomListeners() (net.PacketConn, net.PacketConn, error) {
	audioListener, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to reserve audio RTP port: %w", err)
	}

	videoListener, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		audioListener.Close()
		return nil, nil, fmt.Errorf("failed to reserve video RTP port: %w", err)
	}
	return audioListener, videoListener, nil
}
