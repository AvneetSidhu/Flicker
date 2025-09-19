package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/pion/rtp"
)

type ingestSession struct {
	port        int
	forwardChan chan<- *rtp.Packet
	logger      *slog.Logger
	roomID      int
}

func NewIngestSession(logger *slog.Logger, port int, roomID uint32, forwardChan chan<- *rtp.Packet) *ingestSession {
	return &ingestSession{
		port:        port,
		roomID:      roomID,
		logger:      logger,
		forwardChan: forwardChan,
	}
}

func (i *ingestSession) StartIngest(ctx context.Context) {
	conn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", i.port))
	if err != nil {
		i.logger.Error("Error starting UDP listener", "err", err)
		return
	}
	defer conn.Close()

	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			i.logger.Info("Ingest session context done, stopping ingest", slog.Int("roomID", i.roomID))
			return
		default:
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				i.logger.Error("Error reading from UDP", "err", err)
				continue
			}
			rtpPacket := &rtp.Packet{}
			if err := rtpPacket.Unmarshal(buf[:n]); err != nil {
				i.logger.Error("Error unmarshaling RTP packet", "err", err)
				continue
			}
			select {
			case i.forwardChan <- rtpPacket:
			default:
				continue
			}
		}
	}
}

func (i *ingestSession) cleanUp() {
	close(i.forwardChan)
}
