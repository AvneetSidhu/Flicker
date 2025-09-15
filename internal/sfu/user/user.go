package user

import (
	"context"
	"log/slog"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type User struct {
	Name string

	PeerConn *webrtc.PeerConnection

	VideoBuf chan *rtp.Packet
	AudioBuf chan *rtp.Packet

	audioTrack *webrtc.TrackLocalStaticRTP
	videoTrack *webrtc.TrackLocalStaticRTP

	DataChan *webrtc.DataChannel

	VideoBytesSent  uint32
	VideoPacketSent uint32

	AudioBytesSent   uint32
	AudioPacketsSent uint32

	LatestAudioPacketTime uint32
	LatestVideoPacketTime uint32

	AudioSSRC uint32
	VideoSSRC uint32

	logger *slog.Logger
}

func NewUser(name string, pc *webrtc.PeerConnection, audioTrack, videoTrack *webrtc.TrackLocalStaticRTP, dataChan *webrtc.DataChannel, audioSSRC uint32, videoSSRC uint32, logger *slog.Logger) *User {
	return &User{
		Name:       name,
		PeerConn:   pc,
		audioTrack: audioTrack,
		videoTrack: videoTrack,
		AudioBuf:   make(chan *rtp.Packet, 1000),
		VideoBuf:   make(chan *rtp.Packet, 1000),
		DataChan:   dataChan,
		logger:     logger,
		AudioSSRC:  audioSSRC,
		VideoSSRC:  videoSSRC,
	}
}

func (u *User) StartWritingRTP(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pkt := <-u.AudioBuf:
				u.audioTrack.WriteRTP(pkt)
			default:
				continue
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pkt := <-u.VideoBuf:
				u.videoTrack.WriteRTP(pkt)
			default:
				continue
			}
		}
	}()
}

func (u *User) CleanUpUser() {
	u.DataChan.Close()
	u.PeerConn.Close()
}
