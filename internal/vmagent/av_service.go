package vmagent

import (
	"fmt"
	"log/slog"
	"net"
	"time"
)

type AVService struct {
	sfuAddr string
	port    string
	sfu net.Conn
	logger *slog.Logger
}

func NewAVService(cfg *Config) *AVService {
	return &AVService{sfuAddr: cfg.SFUAddr, port: cfg.Port, sfu: nil, logger: slog.Default().With("service", "av")}
}

func (s *AVService) StartIngest() {
	conn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%s", s.port))
	if err != nil { 
		s.logger.Error("Failed to bind to ingestion port", "err", err)
		return 
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(time.Second))

	buf := make([]byte, 1500)
	for {
		conn.SetReadDeadline(time.Now().Add(time.Second))

		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			s.logger.Error("error ingesting RTP", "err", err)
			continue
		}

		err = s.forward(buf[:n])
		if err != nil {
			s.logger.Error("error forwarding rtp", "err", err)
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

func (s *AVService) ConnectToSFU() error {
	conn, err := net.Dial("udp", s.sfuAddr)
	if err != nil {
		return fmt.Errorf("error connecting to sfu at %s: %w", s.sfuAddr, err)
	}
	s.sfu = conn
	return nil
}
