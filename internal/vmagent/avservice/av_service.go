package av_service

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/pion/rtp"
)

type AVService struct {
	sfu    net.Conn
	listen net.PacketConn
	logger *slog.Logger
	roomID []byte
}

func NewAVService(sfuConnection net.Conn, listenConnection net.PacketConn, logger *slog.Logger, roomID string) *AVService {
	return &AVService{sfu: sfuConnection, listen: listenConnection, logger: logger, roomID: []byte(roomID)}
}

func (s *AVService) StartIngest(ctx context.Context) {
	buf := make([]byte, 1500)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutdown signal received: ending ingest in av service")
			return
		default:
			s.listen.SetReadDeadline(time.Now().Add(time.Second))

			n, _, err := s.listen.ReadFrom(buf)
			if err != nil {
				s.logger.Error("error ingesting RTP", slog.Any("error", err))
				continue
			}

			taggedPacket, err := addHeaderExtension(buf[:n], s.roomID)
			if err != nil {
				s.logger.Error("error tagging RTP", "err", err)
			}

			err = s.forward(taggedPacket)
			if err != nil {
				s.logger.Error("error forwarding RTP", slog.Any("error", err))
			}
		}
	}
}

func (s *AVService) forward(packet []byte) error {
	if s.sfu == nil {
		return fmt.Errorf("sfu address undefined when writing")
	}

	if _, err := s.sfu.Write(packet); err != nil {
		return err
	}
	return nil
}

func addHeaderExtension(packet []byte, roomID []byte) ([]byte, error) {
	var rtpPacket rtp.Packet
	if err := rtpPacket.Unmarshal(packet); err != nil {
		return nil, fmt.Errorf("failed to parse RTP packet %w", err)
	}

	rtpPacket.Header.SetExtension(1, roomID)

	raw, err := rtpPacket.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RTP packet on re-mashal: %w", err)
	}

	return raw, nil
}
