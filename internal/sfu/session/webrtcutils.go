package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

func (s *Session) addIceCandidateToPeerConnection(candidate ICECandidatePayload) error {
	if s.PeerConn == nil {
		return nil
	}

	iceCandidate := webrtc.ICECandidateInit{
		Candidate:     candidate.Candidate,
		SDPMid:        &candidate.SDPMid,
		SDPMLineIndex: &candidate.SDPMLineIndex,
	}
	return s.PeerConn.AddICECandidate(iceCandidate)
}

func (s *Session) createPeerConnection(offer WebRTCOfferPayload, ctx context.Context) error {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	videoTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"video", "pion",
	)

	audioTrack, _ := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio", "pion",
	)

	pc, newPeerConnectionErr := s.api.NewPeerConnection(config)
	if newPeerConnectionErr != nil {
		return newPeerConnectionErr
	}

	setRemoteDescriptionErr := pc.SetRemoteDescription(
		webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  offer.SDP,
		},
	)

	if setRemoteDescriptionErr != nil {
		return setRemoteDescriptionErr
	}

	audioSender, audioErr := pc.AddTrack(audioTrack)
	if audioErr != nil {
		return audioErr
	}

	videoSender, videoErr := pc.AddTrack(videoTrack)
	if videoErr != nil {
		return videoErr

	}

	// user := user.NewUser(s.DisplayName, pc, audioTrack, videoTrack, nil, uint32(audioSender.GetParameters().Encodings[0].SSRC), uint32(videoSender.GetParameters().Encodings[0].SSRC), s.Logger)

	s.PeerConn = pc
	s.audioTrack = audioTrack
	s.videoTrack = videoTrack
	s.audioSSRC = uint32(audioSender.GetParameters().Encodings[0].SSRC)
	s.videoSSRC = uint32(videoSender.GetParameters().Encodings[0].SSRC)

	s.defineCallbacks(pc, ctx)

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		s.Logger.Error("Error setting local description", "err", err)
		return err
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	<-gatherComplete

	finalLocalDesc := pc.LocalDescription()

	answerPayload := WebRTCAnswerPayload{
		SDP: finalLocalDesc.SDP,
	}

	answerPayloadBytes, marshalErr := json.Marshal(answerPayload)
	if marshalErr != nil {
		s.Logger.Error("Error marshalling answer payload", "err", marshalErr)
		return marshalErr
	}
	msg := WSMessage{
		Type:    "answer",
		Payload: answerPayloadBytes,
		UserID:  s.DisplayName,
	}
	msgBytes, marshalErr := json.Marshal(msg)
	if marshalErr != nil {
		s.Logger.Error("Error marshalling answer message", "err", marshalErr)
		return marshalErr
	}
	select {
	case s.SendChan <- []byte(msgBytes):
		s.Logger.Debug("Sent answer", slog.String("user", s.DisplayName), slog.Any("msg", msg))
	default:
		s.Logger.Warn("Send channel is full, dropping answer message")
	}

	return nil
}

func (s *Session) defineCallbacks(pc *webrtc.PeerConnection, ctx context.Context) {
	if s.PeerConn == nil {
		s.Logger.Error("PeerConnection is nil, cannot define callbacks")
		return
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.Logger.Info("Peer Connection State has changed", "state", state.String())
		if state == webrtc.PeerConnectionStateConnected {
			s.SetActiveStream(true)
			ctx, cancel := context.WithCancel(ctx)
			s.rtpWriteCancel = cancel
			s.StartWritingRTP(ctx)
		}

		if state == webrtc.PeerConnectionStateFailed {
			s.Logger.Info("Peer Connection has gone to failed exiting")
			s.SetActiveStream(false)
		}
	})

	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		s.Logger.Info("New DataChannel has been created", "label", d.Label(), "id", d.ID())
		s.dataChan = d
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		s.Logger.Debug("New ICE candidate found", slog.String("user", s.DisplayName))
		if c == nil {
			return
		}
		candidateJSON := c.ToJSON()
		candidateJSONStr, candidateJsonErr := json.Marshal(candidateJSON)

		if candidateJsonErr != nil || candidateJSON.SDPMid == nil || *candidateJSON.SDPMid == "" || candidateJSON.SDPMLineIndex == nil {
			s.Logger.Debug("Skipping ICE candidate with missing or empty sdpMid / sdpMLineIndex",
				slog.Any("candidate", candidateJSON),
				slog.String("error", fmt.Sprint(candidateJsonErr)))
		}

		signalMsg, wsMessageJsonErr := json.Marshal(WSMessage{
			Type:    "ice-candidate",
			Payload: candidateJSONStr,
			UserID:  s.DisplayName,
		})
		if wsMessageJsonErr != nil {
			s.Logger.Error("Error marshalling ICE candidate", "err", wsMessageJsonErr)
			return
		}

		select {
		case s.SendChan <- signalMsg:
			s.Logger.Debug("Sent ICE candidate", slog.String("candidate", candidateJSON.Candidate), slog.String("user", s.DisplayName))
		default:
			s.Logger.Warn("Send channel is full, dropping ICE candidate message")
		}
	})
}

func toNTPTime(t time.Time) uint64 {
	const ntpEpoch = 2208988800
	seconds := uint64(t.Unix()) + ntpEpoch
	fraction := uint64(t.Nanosecond()) * 0xFFFFFFFF / 1e9
	return (seconds << 32) | fraction
}

func (s *Session) sendRTCPSenderReport(ssrc uint32, rtpTimeStamp uint32, packetCount uint32, byteCount uint32) {
	now := time.Now()
	ntpTimeStamp := toNTPTime(now)

	senderReport := &rtcp.SenderReport{
		SSRC:        ssrc,
		NTPTime:     ntpTimeStamp,
		RTPTime:     rtpTimeStamp,
		PacketCount: packetCount,
		OctetCount:  byteCount,
	}

	_ = s.PeerConn.WriteRTCP([]rtcp.Packet{senderReport})
}

func (s *Session) StartWritingRTP(ctx context.Context) {
	s.Logger.Debug("started rtp write pump for user", "userID", s.UserID)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pooled := <-s.AudioBuf:
				if pooled == nil {
					return
				}
				pkt := pooled.Packet
				if s.audioPacketsSent == 0 {
					s.masterRTPStartTime = pkt.Timestamp
				}
				// s.Logger.Debug("writing audio packet", "userID", s.UserID)
				atomic.AddUint32(&s.audioPacketsSent, 1)
				atomic.AddUint32(&s.audioBytesSent, uint32(len(pkt.Payload)))
				atomic.StoreUint32(&s.latestAudioPacketTime, pkt.Timestamp)
				if s.audioTrack == nil {
					s.Logger.Warn("audio track is nil, cannot write RTP")
					pooled.Release()
					continue
				}
				s.audioTrack.WriteRTP(pkt)
				pooled.Release()
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case pooled := <-s.VideoBuf:
				if pooled == nil {
					return
				}
				pkt := pooled.Packet
				if s.videoPacketsSent == 0 {
					s.nonMasterRTPStartTime = pkt.Timestamp
				}
				// s.Logger.Debug("writing video packet", "userID", s.UserID)
				atomic.AddUint32(&s.videoPacketsSent, 1)
				atomic.AddUint32(&s.videoBytesSent, uint32(len(pkt.Payload)))
				atomic.StoreUint32(&s.latestVideoPacketTime, pkt.Timestamp)
				if s.videoTrack == nil {
					s.Logger.Warn("video track is nil, cannot write RTP")
					pooled.Release()
					continue
				}
				s.videoTrack.WriteRTP(pkt)
				pooled.Release()
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sendRTCPSenderReport(s.videoSSRC, s.nonMasterRTPStartTime, s.videoPacketsSent, uint32(s.videoBytesSent))
				s.sendRTCPSenderReport(s.audioSSRC, s.masterRTPStartTime, s.audioPacketsSent, uint32(s.audioBytesSent))
			}
		}
	}()
}
