package signal

import (
	"encoding/json"
	"log/slog"

	"github.com/AvneetSidhu/Flicker/internal/sfu/user"
	"github.com/pion/webrtc/v3"
)

func (s *session) addIceCandidateToPeerConnection(pc *webrtc.PeerConnection, candidate ICECandidatePayload) error {
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

func (s *session) createPeerConnection(offer WebRTCOfferPayload) (*user.User, error) {
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
		return nil, newPeerConnectionErr
	}

	setRemoteDescriptionErr := pc.SetRemoteDescription(
		webrtc.SessionDescription{
			Type: webrtc.SDPTypeOffer,
			SDP:  offer.SDP,
		},
	)

	if setRemoteDescriptionErr != nil {
		return nil, setRemoteDescriptionErr
	}

	audioSender, audioErr := pc.AddTrack(audioTrack)
	if audioErr != nil {
		return nil, audioErr
	}

	videoSender, videoErr := pc.AddTrack(videoTrack)
	if videoErr != nil {
		return nil, videoErr

	}

	peer := user.NewUser(s.displayName, pc, audioTrack, videoTrack, nil, uint32(audioSender.GetParameters().Encodings[0].SSRC), uint32(videoSender.GetParameters().Encodings[0].SSRC), s.logger)
	s.defineCallbacks(pc)

	answer, err := s.createAnswer(offer.SDP)
	if err != nil {
		return nil, err
	}

	select {
	case s.sendChan <- []byte(answer):
	default:
		s.logger.Warn("Send channel is full, dropping answer message")
	}

	return peer, nil
}

func (s *session) createAnswer(offerSDP string) (string, error) {

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}
	if err := s.user.PeerConn.SetRemoteDescription(offer); err != nil {
		return "", err
	}

	answer, err := s.user.PeerConn.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	if err := s.user.PeerConn.SetLocalDescription(answer); err != nil {
		return "", err
	}
	return answer.SDP, nil
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
		if c == nil {
			return
		}
		candidateJSON := c.ToJSON()
		candidateJSONStr, candidateJsonErr := json.Marshal(candidateJSON)

		if candidateJsonErr != nil || candidateJSON.SDPMid == nil || *candidateJSON.SDPMid == "" || candidateJSON.SDPMLineIndex == nil {
			s.logger.Debug("Skipping ICE candidate with missing or empty sdpMid / sdpMLineIndex", slog.Any("candidate", candidateJSON), slog.String("error", candidateJsonErr.Error()))
			return
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
		default:
			s.logger.Warn("Send channel is full, dropping ICE candidate message")
		}
	})
}
