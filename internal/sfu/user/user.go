package user

import (
	"context"
	"log/slog"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

type User struct {
	Name       string
	peerConn *webrtc.PeerConnection

	VideoBuf chan *rtp.Packet
	AudioBuf chan *rtp.Packet

	audioTrack *webrtc.TrackLocalStaticRTP
	videoTrack *webrtc.TrackLocalStaticRTP

	dataChan *webrtc.DataChannel

	VideoBytesSent uint32
	VideoPacketSent uint32

	AudioBytesSent uint32
	AudioPacketsSent uint32

	LatestAudioPacketTime uint32
	LatestVideoPacketTime uint32

	AudioSSRC uint32
	VideoSSRC uint32

	roomID uint32

	logger *slog.Logger
}

func (u *User) StartWriting(ctx context.Context){
	go func() {
		for {
			select{
			case <-ctx.Done():
				u.logger.Info("ending video write loop", "user", u.Name)
			case pkt := <-u.AudioBuf:
				u.audioTrack.WriteRTP(pkt)
			default:
				continue
			}
	}
	}()

	go func() {
		for {
		select{
		case <-ctx.Done():
			u.logger.Info("ending audio write loop", "user", u.Name)
		case pkt := <-u.VideoBuf:
			u.videoTrack.WriteRTP(pkt)
		default:
			continue
		}
	}
	}()
}


func (u *User) CleanUpUser() {
	u.dataChan.Close()
	u.peerConn.Close()
}
