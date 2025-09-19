package server

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/AvneetSidhu/Flicker/internal/sfu/user"
	"github.com/pion/webrtc/v3"
)

func (s *session) addIceCandidateToPeerConnection(candidate ICECandidatePayload) error {
	if s.user == nil {
		return nil
	}

	iceCandidate := webrtc.ICECandidateInit{
		Candidate:     candidate.Candidate,
		SDPMid:        &candidate.SDPMid,
		SDPMLineIndex: &candidate.SDPMLineIndex,
	}
	return s.user.PeerConn.AddICECandidate(iceCandidate)
}

func (s *session) createPeerConnection(offer WebRTCOfferPayload) error {
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

	user := user.NewUser(s.displayName, pc, audioTrack, videoTrack, nil, uint32(audioSender.GetParameters().Encodings[0].SSRC), uint32(videoSender.GetParameters().Encodings[0].SSRC), s.logger)
	s.user = user
	s.defineCallbacks(pc)

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		s.logger.Error("Error setting local description", "err", err)
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
		s.logger.Error("Error marshalling answer payload", "err", marshalErr)
		return marshalErr
	}
	msg := WSMessage{
		Type:    "answer",
		Payload: answerPayloadBytes,
		UserID:  s.displayName,
	}
	msgBytes, marshalErr := json.Marshal(msg)
	if marshalErr != nil {
		s.logger.Error("Error marshalling answer message", "err", marshalErr)
		return marshalErr
	}
	select {
	case s.sendChan <- []byte(msgBytes):
		s.logger.Debug("Sent answer", slog.String("user", s.displayName), slog.Any("msg", msg))
	default:
		s.logger.Warn("Send channel is full, dropping answer message")
	}

	return nil
}

func (s *session) defineCallbacks(pc *webrtc.PeerConnection) {
	if s.user == nil {
		s.logger.Error("PeerConnection is nil, cannot define callbacks")
		return
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.logger.Info("Peer Connection State has changed", "state", state.String())
		if state == webrtc.PeerConnectionStateFailed {
			s.logger.Info("Peer Connection has gone to failed exiting")
			s.onClose(s.displayName)
		}
	})

	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		s.logger.Info("New DataChannel has been created", "label", d.Label(), "id", d.ID())
		s.user.DataChan = d
	})

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		s.logger.Debug("New ICE candidate found", slog.String("user", s.displayName))
		if c == nil {
			return
		}
		candidateJSON := c.ToJSON()
		candidateJSONStr, candidateJsonErr := json.Marshal(candidateJSON)

		if candidateJsonErr != nil || candidateJSON.SDPMid == nil || *candidateJSON.SDPMid == "" || candidateJSON.SDPMLineIndex == nil {
			s.logger.Debug("Skipping ICE candidate with missing or empty sdpMid / sdpMLineIndex",
				slog.Any("candidate", candidateJSON),
				slog.String("error", fmt.Sprint(candidateJsonErr)))
		}

		signalMsg, wsMessageJsonErr := json.Marshal(WSMessage{
			Type:    "ice-candidate",
			Payload: candidateJSONStr,
			UserID:  s.displayName,
		})
		if wsMessageJsonErr != nil {
			s.logger.Error("Error marshalling ICE candidate", "err", wsMessageJsonErr)
			return
		}

		select {
		case s.sendChan <- signalMsg:
			s.logger.Debug("Sent ICE candidate", slog.String("candidate", candidateJSON.Candidate), slog.String("user", s.displayName))
		default:
			s.logger.Warn("Send channel is full, dropping ICE candidate message")
		}
	})
}
