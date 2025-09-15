package signal

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/AvneetSidhu/Flicker/internal/sfu/user"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type session struct {
	displayName string
	socketConn  *websocket.Conn
	sendChan    chan []byte
	roomId      uint32
	api         *webrtc.API
	logger      *slog.Logger
	onClose     func(clientID string)
	user        *user.User
	closeOnce   sync.Once
}

func (s *session) readPump(ctx context.Context) {
	defer func() {
		s.socketConn.Close()
	}()

	for {

		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := s.socketConn.ReadMessage()
			if err != nil {
				s.logger.Error("readPump: error reading message", slog.String("error", err.Error()), slog.String("user", s.displayName))
				break
			}
			wsMessage, err := parseWSMessage(message)
			if err != nil {
				s.logger.Error("readPump: error parsing ws message", slog.String("error", err.Error()), slog.String("user", s.displayName))
				continue
			}

			switch wsMessage.Type {
			case "offer":
				offer, err := parseWSMessagePayload[WebRTCOfferPayload](wsMessage.Payload)
				if err != nil {
					s.logger.Error("readPump: error parsing offer payload", slog.String("error", err.Error()), slog.String("user", s.displayName))
					continue
				}

				peer, createPeerErr := s.createPeerConnection(*offer)
				if createPeerErr != nil {
					s.logger.Error("readPump: error creating peer connection", slog.String("error", createPeerErr.Error()), slog.String("user", s.displayName))
					continue
				}
				s.user = peer

			case "ice-candidate":
				candidate, parseCandidateErr := parseWSMessagePayload[ICECandidatePayload](wsMessage.Payload)
				if parseCandidateErr != nil {
					s.logger.Error("readPump: error parsing ice candidate payload", slog.String("error", parseCandidateErr.Error()), slog.String("user", s.displayName))
					continue
				}

				iceCandidateAddErr := s.addIceCandidateToPeerConnection(s.user.PeerConn, *candidate)
				if iceCandidateAddErr != nil {
					s.logger.Error("readPump: error adding ice candidate", slog.String("error", iceCandidateAddErr.Error()), slog.String("user", s.displayName))
					continue
				}

			default:
				s.logger.Error("readPump: unknown message type", slog.String("type", wsMessage.Type), slog.String("user", s.displayName))
				continue
			}
		}
	}
}

func (s *session) writePump(ctx context.Context) {
	defer func() {
		s.socketConn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-s.sendChan:
			if !ok {
				return
			}
			writeErr := s.socketConn.WriteMessage(websocket.TextMessage, message)
			if writeErr != nil {
				s.logger.Error("writePump: error writing message", slog.String("error", writeErr.Error()), slog.String("user", s.displayName))
				return
			}
		default:
			continue
		}
	}
}

func parseWSMessage(msg []byte) (*WSMessage, error) {
	var decodedMsg WSMessage
	err := json.Unmarshal(msg, &decodedMsg)
	if err != nil {
		return nil, err
	}
	return &decodedMsg, nil
}

func parseWSMessagePayload[T any](payload []byte) (*T, error) {
	var out T
	err := json.Unmarshal(payload, out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *session) CleanUpSession() {
	s.closeOnce.Do(func() {
		if s.onClose != nil {
			s.onClose(s.displayName)
		}

		s.socketConn.Close()
		close(s.sendChan)

		s.user.CleanUpUser()

		s.logger.Info("Session closed", "user", s.displayName)
	})
}
