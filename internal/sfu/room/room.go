package room

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AvneetSidhu/Flicker/internal/sfu/control"
	"github.com/AvneetSidhu/Flicker/internal/sfu/ingest"
	"github.com/AvneetSidhu/Flicker/internal/sfu/session"
)

const (
	PayloadTypeOpus = 111
	PayloadTypeVP8  = 96
)

type VMState string

const (
	VMStateProvisioning VMState = "provisioning"
	VMStateReady        VMState = "ready"
	VMStateFailed       VMState = "failed"
)

// type PooledPacket interface {
// 	Acquire()
// 	Release()
// 	AcquireBulk(n int)
// }

type Room struct {
	logger *slog.Logger

	RoomID            string
	Name              string
	Creator           string
	OwnerID           string
	membersSnapshot   atomic.Value
	membersMu         sync.Mutex
	controllerMu      sync.Mutex
	CurrentController string
	inputForward      chan<- *control.CommandEvent
	vmReady           atomic.Bool
	vmState           atomic.Value
	vmFailureMu       sync.RWMutex
	vmFailure         string

	emptyMu sync.Mutex
	emptyAt time.Time
}

var ErrNotController = errors.New("input requires the active controller")

func (r *Room) RequestControl(userID string) bool {
	r.controllerMu.Lock()
	defer r.controllerMu.Unlock()
	if r.CurrentController != "" && r.CurrentController != userID {
		return false
	}
	r.CurrentController = userID
	return true
}

func (r *Room) ReleaseControl(userID string) bool {
	r.controllerMu.Lock()
	defer r.controllerMu.Unlock()
	if r.CurrentController != userID {
		return false
	}
	r.CurrentController = ""
	return true
}

func (r *Room) ForwardInput(userID string, event *control.CommandEvent) error {
	r.controllerMu.Lock()
	isController := r.CurrentController == userID
	r.controllerMu.Unlock()
	if !isController {
		return ErrNotController
	}
	event.RoomID = r.RoomID
	select {
	case r.inputForward <- event:
		return nil
	default:
		return fmt.Errorf("control queue is full")
	}
}

func (r *Room) SetVMReady() {
	r.vmReady.Store(true)
	r.vmState.Store(VMStateReady)
	r.logger.Info("VM ingest ready", "roomID", r.RoomID)
}

func (r *Room) SetVMFailed(err error) {
	r.vmReady.Store(false)
	r.vmState.Store(VMStateFailed)
	r.vmFailureMu.Lock()
	defer r.vmFailureMu.Unlock()
	if err == nil {
		r.vmFailure = "VM provisioning failed"
		return
	}
	r.vmFailure = err.Error()
}

func (r *Room) VMState() VMState {
	return r.vmState.Load().(VMState)
}

func (r *Room) VMFailure() string {
	r.vmFailureMu.RLock()
	defer r.vmFailureMu.RUnlock()
	return r.vmFailure
}

func (r *Room) IsVMReady() bool {
	return r.vmReady.Load()
}

// use atomic.Value for members snapshot / create on write
func NewRoom(logger *slog.Logger, roomID string, name string, creator string, ownerID string, inputForward chan<- *control.CommandEvent) *Room {
	r := &Room{
		RoomID:            roomID,
		Name:              name,
		Creator:           creator,
		OwnerID:           ownerID,
		CurrentController: "",
		inputForward:      inputForward,
		logger:            logger,
		emptyAt:           time.Now(),
	}

	r.membersSnapshot.Store([]*session.Session{})
	r.vmState.Store(VMStateProvisioning)
	return r
}

func (r *Room) AddMember(user *session.Session) bool {
	r.membersMu.Lock()
	defer r.membersMu.Unlock()

	oldList := r.membersSnapshot.Load().([]*session.Session)
	for _, member := range oldList {
		if member == user {
			return false
		}
	}
	newList := append([]*session.Session{}, oldList...)
	newList = append(newList, user)
	r.membersSnapshot.Store(newList)

	r.emptyMu.Lock()
	r.emptyAt = time.Time{}
	r.emptyMu.Unlock()
	r.broadcastPresence(newList)
	return true
}

func (r *Room) RemoveMember(user *session.Session) bool {
	r.membersMu.Lock()
	defer r.membersMu.Unlock()

	old := r.membersSnapshot.Load().([]*session.Session)
	newList := make([]*session.Session, 0, len(old))
	removed := false

	for _, mem := range old {
		if mem != user {
			newList = append(newList, mem)
		} else {
			removed = true
		}
	}
	if !removed {
		return false
	}

	r.membersSnapshot.Store(newList)
	r.ReleaseControl(user.UserID)
	r.broadcastPresence(newList)

	if len(newList) == 0 {
		r.emptyMu.Lock()
		if r.emptyAt.IsZero() {
			r.emptyAt = time.Now()
		}
		r.emptyMu.Unlock()
	}
	return true
}

func (r *Room) broadcastPresence(members []*session.Session) {
	viewers := make([]session.RoomViewer, 0, len(members))
	for _, member := range members {
		viewers = append(viewers, session.RoomViewer{ID: member.UserID, Name: member.DisplayName})
	}
	payload := session.RoomPresencePayload{RoomID: r.RoomID, Viewers: viewers}
	for _, member := range members {
		member.SendMessage("room-presence", payload)
	}
}

// EmptyDuration returns how long the room has had no members.
// Returns zero if the room currently has members.
func (r *Room) EmptyDuration() time.Duration {
	r.emptyMu.Lock()
	defer r.emptyMu.Unlock()
	if r.emptyAt.IsZero() {
		return 0
	}
	return time.Since(r.emptyAt)
}

func (r *Room) GetMember(userName string) *session.Session {
	oldList := r.membersSnapshot.Load().([]*session.Session)

	for _, mem := range oldList {
		if mem.DisplayName == userName {
			return mem
		}
	}

	return nil
}

func (r *Room) MemberCount() int {
	return len(r.membersSnapshot.Load().([]*session.Session))
}

func (r *Room) Broadcast(pkt *ingest.PooledPacket, bufferType string) {

	pkt.Acquire()

	snapshot := r.membersSnapshot.Load().([]*session.Session)
	activeUsers := int32(0)

	for _, u := range snapshot {
		if !u.IsActive() {
			continue
		}
		activeUsers++
	}

	if activeUsers == 0 {
		pkt.Release()
		return
	}

	pkt.AcquireBulk(activeUsers - 1)
	for _, u := range snapshot {
		if !u.IsActive() {
			continue
		}
		if bufferType == "audio" {
			if err := sendWithRetry(pkt, u.AudioBuf); err != nil {
				pkt.Release()
			}
		} else {
			if err := sendWithRetry(pkt, u.VideoBuf); err != nil {
				pkt.Release()
			}
		}
	}

}

func sendWithRetry(pkt *ingest.PooledPacket, ch chan *ingest.PooledPacket) error {
	select {
	case ch <- pkt:
		return nil
	default:
		dropped := <-ch
		dropped.Release()
		select {
		case ch <- pkt:
			return nil
		default:
			return fmt.Errorf("writing failed: buffer full")
		}
	}
}
