package ingest

import (
	"testing"

	"github.com/pion/rtp"
)

func TestPooledPacketReleasesBufferOnceAfterAllConsumersFinish(t *testing.T) {
	released := 0
	packet := NewPooledPacket(&rtp.Packet{}, []byte("buffer"), func([]byte) {
		released++
	})

	packet.AcquireBulk(2)
	packet.Release()
	if released != 0 {
		t.Fatalf("buffer released too early: %d", released)
	}
	packet.Release()
	if released != 1 {
		t.Fatalf("buffer release count = %d, want 1", released)
	}
}
