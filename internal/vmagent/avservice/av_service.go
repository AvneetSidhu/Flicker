package av_service

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"
)

type AVService struct {
	sfu    net.Conn
	listen net.PacketConn
	logger *slog.Logger
}

func NewAVService(sfuConnection net.Conn, listenConnection net.PacketConn, logger *slog.Logger) *AVService {
	return &AVService{sfu: sfuConnection, listen: listenConnection, logger: logger}
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

			err = s.forward(buf[:n])
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
		return fmt.Errorf("error forwarding RTP packet of size %d", len(packet))
	}
	return nil
}
