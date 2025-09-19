package ingest

import (
	"context"
	"log/slog"
	"sync"

	"github.com/pion/rtp"
)

type IngestManager struct {
	basePort  int
	vmCount   int
	ingestMap map[uint32]*ingestSession
	logger    *slog.Logger
	mu        sync.RWMutex
}

func NewIngestManager(logger *slog.Logger) *IngestManager {
	return &IngestManager{
		basePort:  50000,
		vmCount:   0,
		ingestMap: make(map[uint32]*ingestSession),
		logger:    logger,
		mu:        sync.RWMutex{},
	}
}

func (i *IngestManager) CreateIngestSession(ctx context.Context, roomID uint32, forwardChan chan<- *rtp.Packet) {
	session := NewIngestSession(
		i.logger,
		i.basePort,
		roomID,
		forwardChan,
	)
	i.addSession(session)
	session.StartIngest(ctx)
}

func (i *IngestManager) addSession(session *ingestSession) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.ingestMap[uint32(session.roomID)] = session
}

func (i *IngestManager) RemoveSession(roomID uint32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	session, ok := i.ingestMap[uint32(roomID)]
	if !ok {
		i.logger.Debug("could not get session", "roomID", roomID)
	}
	session.cleanUp()
	delete(i.ingestMap, uint32(roomID))
}
