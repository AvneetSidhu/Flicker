package room

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/AvneetSidhu/Flicker/internal/sfu/user"
	"github.com/pion/rtp"
)

const (
	PayloadTypeOpus = 111
	PayloadTypeVP8  = 96
)

type Room struct {
	logger *slog.Logger

	RoomId            uint32
	membersSnapshot   []*user.User
	mediaChan         <-chan *rtp.Packet
	CurrentController string
	mu                sync.RWMutex
}

// use atomic.Value for members snapshot / create on write
func NewRoom(logger *slog.Logger, roomId uint32, mediaChan <-chan *rtp.Packet) *Room {
	return &Room{
		RoomId:            roomId,
		mediaChan:         mediaChan,
		CurrentController: "",
	}
}

func (r *Room) AddMember(user *user.User) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.membersSnapshot = append(r.membersSnapshot, user)
}

func (r *Room) RemoveMember(user *user.User) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for idx, mem := range r.membersSnapshot {
		if mem == user {
			if mem.Name == r.CurrentController {
				r.CurrentController = ""
			}

			if idx != len(r.membersSnapshot)-1 {
				lastUser := r.membersSnapshot[len(r.membersSnapshot)-1]
				r.membersSnapshot[idx] = lastUser
			}

			r.membersSnapshot = r.membersSnapshot[:len(r.membersSnapshot)-1]
			return true
		}
	}
	return false
}

func (r *Room) GetMember(userName string) *user.User {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, mem := range r.membersSnapshot {
		if mem.Name == userName {
			return mem
		}
	}

	return nil
}

func (r *Room) Broadcast(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			pkt, ok := <-r.mediaChan
			if !ok {
				break
			}
			if pkt == nil {
				continue
			}

			for _, u := range r.membersSnapshot {
				go func() {
					pktCopy := *pkt
					pktCopy.Payload = append([]byte(nil), pkt.Payload...)

					err := demuxAndSend(&pktCopy, u)
					if err != nil {
						r.logger.Error("error sending packet", "err", err, "client", u.Name)
					}
				}()
			}
		}
	}
}

func demuxAndSend(pkt *rtp.Packet, user *user.User) error {
	var err error = nil
	if pkt.PayloadType == PayloadTypeVP8 {
		err = sendWithRetry(pkt, user.VideoBuf)
	} else if pkt.PayloadType == PayloadTypeOpus {
		err = sendWithRetry(pkt, user.AudioBuf)
	}

	if err != nil {
		return fmt.Errorf("error sending packet (payloadType=%d): %w", pkt.PayloadType, err)
	}

	return nil
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
