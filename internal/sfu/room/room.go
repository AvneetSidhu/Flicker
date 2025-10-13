package room

import (
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/AvneetSidhu/Flicker/internal/sfu/control"
	"github.com/AvneetSidhu/Flicker/internal/sfu/session"
	"github.com/pion/rtp"
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
	membersSnapshot   atomic.Value
	CurrentController string
	inputForward      chan<- *control.CommandEvent
}

// use atomic.Value for members snapshot / create on write
func NewRoom(logger *slog.Logger, roomID string, inputForward chan<- *control.CommandEvent) *Room {
	r := &Room{
		RoomID:            roomID,
		CurrentController: "",
		inputForward:      inputForward,
		logger:            logger,
	}

	r.membersSnapshot.Store([]*session.Session{})
	return r
}

func (r *Room) AddMember(user *session.Session) bool {
	oldList := r.membersSnapshot.Load().([]*session.Session)
	newList := append([]*session.Session{}, oldList...)
	newList = append(newList, user)
	r.membersSnapshot.Store(newList)
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
	return true
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

func (r *Room) Broadcast(pkt *rtp.Packet, bufferType string) {

	// pkt.Acquire()

	snapshot := r.membersSnapshot.Load().([]*session.Session)
	activeUsers := int32(0)

	for _, u := range snapshot {
		if !u.IsActive() {
			continue
		}
		activeUsers++
	}

	// if activeUsers == 0 {
	// 	pkt.Release()
	// 	return
	// }

	// pkt.AcquireBulk(activeUsers - 1)
	for _, u := range snapshot {
		if !u.IsActive() {
			continue
		}
		if bufferType == "audio" {
			if err := sendWithRetry(pkt, u.AudioBuf); err != nil {
				// pkt.Release()
			}
		} else {
			if err := sendWithRetry(pkt, u.VideoBuf); err != nil {
				// pkt.Release()
			}
		}
	}

}

func sendWithRetry(pkt *rtp.Packet, ch chan *rtp.Packet) error {
	select {
	case ch <- pkt:
		return nil
	default:
		<-ch
		select {
		case ch <- pkt:
			return nil
		default:
			return fmt.Errorf("writing failed: buffer full")
		}
	}
}
