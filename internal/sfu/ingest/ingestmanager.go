package ingest

import (
	"context"
	"log/slog"
	"net"
	"sync"

	"github.com/pion/rtp"
)

type Room interface {
	Broadcast(pkt *rtp.Packet, bufferType string)
}

type IngestManager struct {
	rooms   sync.Map
	logger  *slog.Logger
	port    string
	bufPool sync.Pool
	listen  net.PacketConn
}

func NewIngestManager(logger *slog.Logger, port string) *IngestManager {
	return &IngestManager{
		rooms:   sync.Map{},
		logger:  logger,
		port:    port,
		bufPool: sync.Pool{New: func() interface{} { return make([]byte, 1500) }},
	}
}

func (i *IngestManager) AddRoom(roomID string, audioPort int, videoPort int, room Room, ctx context.Context) {
	ingestWorker := &ingestWorker{
		audioPort: 50005,
		videoPort: 50007,
		room:      room,
		bufPool:   &i.bufPool,
		logger:    i.logger,
		roomID:    roomID,
	}

	go ingestWorker.StartIngest(ctx)
	i.rooms.Store(roomID, ingestWorker)
}

func (i *IngestManager) RemoveRoom(roomID string) {
	if val, ok := i.rooms.LoadAndDelete(roomID); ok {
		close(val.(chan *PooledPacket))
	}
}

func (i *IngestManager) Shutdown() {
	i.logger.Info("ingest service shutting down")
	i.listen.Close()
	i.rooms.Range(func(key, value any) bool {
		value.(*ingestWorker).Shutdown()
		return true
	})
}
