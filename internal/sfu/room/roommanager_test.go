package room

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AvneetSidhu/Flicker/internal/sfu/control"
	"github.com/AvneetSidhu/Flicker/internal/sfu/ingest"
)

type fakeIngestManager struct {
	mu      sync.Mutex
	added   map[string]struct{}
	removed []string
}

func (m *fakeIngestManager) AddRoom(roomID string, audioListener net.PacketConn, videoListener net.PacketConn, _ ingest.Room, onReady func(), _ context.Context) {
	audioListener.Close()
	videoListener.Close()
	m.mu.Lock()
	if m.added == nil {
		m.added = make(map[string]struct{})
	}
	m.added[roomID] = struct{}{}
	m.mu.Unlock()
	if onReady != nil {
		onReady()
	}
}

func (m *fakeIngestManager) RemoveRoom(roomID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, roomID)
}

type fakeVMService struct {
	created chan string
	destroy chan string
}

func (s *fakeVMService) CreateVM(_ context.Context, roomID string, _ int, _ int) error {
	s.created <- roomID
	return nil
}

func (s *fakeVMService) DestroyVM(roomID string) error {
	s.destroy <- roomID
	return nil
}

func TestRoomManagerCreatesListsAndDeletesRoom(t *testing.T) {
	ingestManager := &fakeIngestManager{}
	vmService := &fakeVMService{created: make(chan string, 1), destroy: make(chan string, 1)}
	manager := newRoomManager(slog.Default(), make(chan *control.CommandEvent), ingestManager, vmService, false)

	roomID, err := manager.CreateRoom(context.Background(), "Demo", "owner", "owner-id")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	select {
	case created := <-vmService.created:
		if created != roomID {
			t.Fatalf("VM created for %q, want %q", created, roomID)
		}
	case <-time.After(time.Second):
		t.Fatal("VM was not created")
	}

	rooms := manager.GetRooms()
	if len(rooms) != 1 || rooms[0].ID != roomID || rooms[0].Name != "Demo" || rooms[0].OwnerID != "owner-id" || !rooms[0].IsLive {
		t.Fatalf("unexpected listed rooms: %#v", rooms)
	}

	if err := manager.DeleteRoom(roomID); err != nil {
		t.Fatalf("DeleteRoom returned error: %v", err)
	}
	select {
	case destroyed := <-vmService.destroy:
		if destroyed != roomID {
			t.Fatalf("VM destroyed for %q, want %q", destroyed, roomID)
		}
	case <-time.After(time.Second):
		t.Fatal("VM was not destroyed")
	}
	if len(manager.GetRooms()) != 0 {
		t.Fatal("room remained listed after deletion")
	}

	ingestManager.mu.Lock()
	defer ingestManager.mu.Unlock()
	if len(ingestManager.removed) != 1 || ingestManager.removed[0] != roomID {
		t.Fatalf("unexpected ingest removals: %#v", ingestManager.removed)
	}
}

func TestRoomManagerDeleteMissingRoomReturnsNotFound(t *testing.T) {
	manager := newRoomManager(slog.Default(), make(chan *control.CommandEvent), &fakeIngestManager{}, &fakeVMService{created: make(chan string), destroy: make(chan string)}, false)
	if err := manager.DeleteRoom("missing"); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("deleting missing room error = %v, want ErrRoomNotFound", err)
	}
}

func TestRoomManagerOnlyAllowsOwnerDeletion(t *testing.T) {
	ingestManager := &fakeIngestManager{}
	vmService := &fakeVMService{created: make(chan string, 1), destroy: make(chan string, 1)}
	manager := newRoomManager(slog.Default(), make(chan *control.CommandEvent), ingestManager, vmService, false)
	roomID, err := manager.CreateRoom(context.Background(), "Demo", "owner", "owner-id")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	<-vmService.created

	if err := manager.DeleteRoomForOwner(roomID, "viewer-id"); !errors.Is(err, ErrNotRoomOwner) {
		t.Fatalf("non-owner deletion error = %v, want ErrNotRoomOwner", err)
	}
	if _, ok := manager.GetRoom(roomID); !ok {
		t.Fatal("non-owner deletion removed the room")
	}

	if err := manager.DeleteRoomForOwner(roomID, "owner-id"); err != nil {
		t.Fatalf("owner deletion returned error: %v", err)
	}
	<-vmService.destroy
}

type blockingVMService struct {
	started   chan struct{}
	canceled  chan struct{}
	destroyed chan string
}

type failingVMService struct{}

func (failingVMService) CreateVM(context.Context, string, int, int) error {
	return errors.New("clone failed")
}

func (failingVMService) DestroyVM(string) error {
	return nil
}

func (s *blockingVMService) CreateVM(ctx context.Context, _ string, _ int, _ int) error {
	close(s.started)
	<-ctx.Done()
	close(s.canceled)
	return ctx.Err()
}

func (s *blockingVMService) DestroyVM(roomID string) error {
	s.destroyed <- roomID
	return nil
}

func TestRoomManagerCancelsProvisioningBeforeDeletion(t *testing.T) {
	vmService := &blockingVMService{
		started:   make(chan struct{}),
		canceled:  make(chan struct{}),
		destroyed: make(chan string, 1),
	}
	manager := newRoomManager(slog.Default(), make(chan *control.CommandEvent), &fakeIngestManager{}, vmService, false)
	roomID, err := manager.CreateRoom(context.Background(), "Demo", "owner", "owner-id")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}
	<-vmService.started

	if err := manager.DeleteRoomForOwner(roomID, "owner-id"); err != nil {
		t.Fatalf("DeleteRoomForOwner returned error: %v", err)
	}
	select {
	case <-vmService.canceled:
	case <-time.After(time.Second):
		t.Fatal("VM provisioning context was not canceled")
	}
	select {
	case destroyed := <-vmService.destroyed:
		if destroyed != roomID {
			t.Fatalf("destroyed room = %q, want %q", destroyed, roomID)
		}
	case <-time.After(time.Second):
		t.Fatal("VM cleanup was not requested")
	}
}

func TestRoomManagerMarksFailedProvisioningAndStopsIngest(t *testing.T) {
	ingestManager := &fakeIngestManager{}
	manager := newRoomManager(slog.Default(), make(chan *control.CommandEvent), ingestManager, failingVMService{}, false)
	roomID, err := manager.CreateRoom(context.Background(), "Demo", "owner", "owner-id")
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		rooms := manager.GetRooms()
		if len(rooms) == 1 && rooms[0].VMState == VMStateFailed {
			if rooms[0].Failure != "clone failed" {
				t.Fatalf("failure reason = %q", rooms[0].Failure)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("room %q did not enter failed state", roomID)
		}
		time.Sleep(time.Millisecond)
	}

	ingestManager.mu.Lock()
	defer ingestManager.mu.Unlock()
	if len(ingestManager.removed) != 1 || ingestManager.removed[0] != roomID {
		t.Fatalf("ingest was not stopped for failed room: %#v", ingestManager.removed)
	}
}

func TestAllocateRoomListenersReservesDistinctPorts(t *testing.T) {
	audioListener, videoListener, err := allocateRoomListeners()
	if err != nil {
		t.Fatalf("allocateRoomListeners returned error: %v", err)
	}
	defer audioListener.Close()
	defer videoListener.Close()

	audioPort := audioListener.LocalAddr().(*net.UDPAddr).Port
	videoPort := videoListener.LocalAddr().(*net.UDPAddr).Port
	if audioPort == 0 || videoPort == 0 || audioPort == videoPort {
		t.Fatalf("invalid reserved ports: audio=%d video=%d", audioPort, videoPort)
	}
	if duplicate, err := net.ListenPacket("udp", audioListener.LocalAddr().String()); err == nil {
		duplicate.Close()
		t.Fatal("audio port was not reserved")
	}
}
