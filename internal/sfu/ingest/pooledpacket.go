package ingest

import (
	"sync/atomic"

	"github.com/pion/rtp"
)

type PooledPacket struct {
	Packet *rtp.Packet
	Buffer []byte
	refs   int32
	done   func([]byte)
	Time   uint32
}

func (p *PooledPacket) Acquire() {
	atomic.AddInt32(&p.refs, 1)
}

func (p *PooledPacket) AcquireBulk(refs int32) {
	atomic.AddInt32(&p.refs, refs)
}

func (p *PooledPacket) Release() {
	if atomic.AddInt32(&p.refs, -1) <= 0 {
		p.done(p.Buffer)
	}
}
