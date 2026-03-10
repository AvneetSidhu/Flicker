package ingest

import (
	"context"
	"log/slog"
	"sync"
)

type Room interface {
	Broadcast(pkt *PooledPacket, bufferType string)
}

type IngestManager struct {
	rooms   sync.Map
	logger  *slog.Logger
	bufPool sync.Pool
}

func NewIngestManager(logger *slog.Logger) *IngestManager {
	return &IngestManager{
		rooms:   sync.Map{},
		logger:  logger,
		bufPool: sync.Pool{New: func() interface{} { return make([]byte, 1500) }},
	}
}

func (i *IngestManager) AddRoom(roomID string, audioPort int, videoPort int, room Room, onReady func(), ctx context.Context) {
	ingestWorker := &ingestWorker{
		audioPort: audioPort,
		videoPort: videoPort,
		room:      room,
		onReady:   onReady,
		bufPool:   &i.bufPool,
		logger:    i.logger,
		roomID:    roomID,
	}

	go ingestWorker.StartIngest(ctx)
	i.rooms.Store(roomID, ingestWorker)
}

func (i *IngestManager) RemoveRoom(roomID string) {
	if val, ok := i.rooms.LoadAndDelete(roomID); ok {
		val.(*ingestWorker).Shutdown()
	}
}

func (i *IngestManager) Shutdown() {
	i.logger.Info("ingest service shutting down")
	i.rooms.Range(func(key, value any) bool {
		value.(*ingestWorker).Shutdown()
		return true
	})
}
