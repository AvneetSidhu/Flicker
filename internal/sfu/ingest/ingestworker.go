package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"

	"github.com/pion/rtp"
)

const (
	PayloadTypeOpus = 111
	PayloadTypeVP8  = 96
)

type ingestWorker struct {
	videoPort int
	audioPort int

	room Room

	audioListener net.PacketConn
	videoListener net.PacketConn

	bufPool *sync.Pool
	logger  *slog.Logger
	roomID  string
}

func (w *ingestWorker) StartIngest(ctx context.Context) {
	go w.startRTPIngest(ctx, w.audioPort, "audio")
	go w.startRTPIngest(ctx, w.videoPort, "video")
	go w.startRTCPIngest(ctx, w.audioPort+1)
	go w.startRTCPIngest(ctx, w.videoPort+1)
}

func (w *ingestWorker) startRTPIngest(ctx context.Context, port int, packetType string) {
	conn, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%s", strconv.Itoa(port)))
	if err != nil {
		w.logger.Error("error starting udp listener", "err", err)
		return
	}

	if packetType == "audio" {
		w.audioListener = conn
	} else {
		w.videoListener = conn
	}

	w.logger.Info("listening for udp", "port", port)

	go func() {
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				buf := w.bufPool.Get().([]byte)
				n, _, err := conn.ReadFrom(buf)

				if err != nil {
					w.returnBuffer(buf)
					w.logger.Error("error reading from udp connection", "err", err)
					continue
				}

				if n == 0 {
					w.returnBuffer(buf)
					w.logger.Debug("received nothing on ingest")
					continue
				}

				pkt := &rtp.Packet{}

				if err := pkt.Unmarshal(buf[:n]); err != nil {
					w.returnBuffer(buf)
					w.logger.Error("error unmarshalling packet into rtp packet")
					continue
				}

				// wrappedPkt := &PooledPacket{
				// 	Packet: pkt,
				// 	Buffer: buf,
				// 	done:   func(buf []byte) { w.returnBuffer(buf) },
				// 	Time:   pkt.Timestamp,
				// }

				w.room.Broadcast(pkt, packetType)
			}
		}
	}()
}

func (w *ingestWorker) startRTCPIngest(ctx context.Context, port int) {
}

func (w *ingestWorker) Shutdown() {
	w.logger.Info("shutting down ingest worker", "roomID", w.roomID)

	if w.audioListener != nil {
		w.audioListener.Close()
	}

	if w.videoListener != nil {
		w.videoListener.Close()
	}
}

func (w *ingestWorker) returnBuffer(buf []byte) {
	w.bufPool.Put(buf)
}
