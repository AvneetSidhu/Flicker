package room

import (
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
	membersSnapshot   atomic.Value
	CurrentController string
	inputForward      chan<- *control.CommandEvent
	vmReady           atomic.Bool

	emptyMu sync.Mutex
	emptyAt time.Time // zero means the room currently has members
}

func (r *Room) SetVMReady() {
	r.vmReady.Store(true)
	r.logger.Info("VM ingest ready", "roomID", r.RoomID)
}

func (r *Room) IsVMReady() bool {
	return r.vmReady.Load()
}

// use atomic.Value for members snapshot / create on write
func NewRoom(logger *slog.Logger, roomID string, name string, creator string, inputForward chan<- *control.CommandEvent) *Room {
	r := &Room{
		RoomID:            roomID,
		Name:              name,
		Creator:           creator,
		CurrentController: "",
		inputForward:      inputForward,
		logger:            logger,
		emptyAt:           time.Now(), // starts empty
	}

	r.membersSnapshot.Store([]*session.Session{})
	return r
}

func (r *Room) AddMember(user *session.Session) bool {
	oldList := r.membersSnapshot.Load().([]*session.Session)
	newList := append([]*session.Session{}, oldList...)
	newList = append(newList, user)
	r.membersSnapshot.Store(newList)

	r.emptyMu.Lock()
	r.emptyAt = time.Time{} // room has members — clear the empty timer
	r.emptyMu.Unlock()
	return true
}

func (r *Room) RemoveMember(user *session.Session) bool {
	old := r.membersSnapshot.Load().([]*session.Session)
	newList := make([]*session.Session, 0, len(old)-1)

	for _, mem := range old {
		if mem != user {
			newList = append(newList, mem)
		}
	}

	r.membersSnapshot.Store(newList)

	if len(newList) == 0 {
		r.emptyMu.Lock()
		if r.emptyAt.IsZero() {
			r.emptyAt = time.Now()
		}
		r.emptyMu.Unlock()
	}
	return true
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
