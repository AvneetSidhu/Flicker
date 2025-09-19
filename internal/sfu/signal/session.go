package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/AvneetSidhu/Flicker/internal/sfu/room"
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
	roomManager *room.RoomManager
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
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					s.logger.Error("readPump: unexpected close error", slog.String("error", err.Error()), slog.String("user", s.displayName))
				} else {
					s.logger.Error("readPump: error reading message", slog.String("error", err.Error()), slog.String("user", s.displayName))
				}

				return
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

				createPeerErr := s.createPeerConnection(*offer)
				if createPeerErr != nil {
					s.logger.Error("readPump: error creating peer connection", slog.String("error", createPeerErr.Error()), slog.String("user", s.displayName))
					continue
				}

			case "ice-candidate":
				candidate, parseCandidateErr := parseWSMessagePayload[ICECandidatePayload](wsMessage.Payload)
				if parseCandidateErr != nil {
					s.logger.Error("readPump: error parsing ice candidate payload", slog.String("error", parseCandidateErr.Error()), slog.String("user", s.displayName))
					continue
				}

				// s.logger.Debug("Received ICE candidate", slog.String("candidate", candidate.Candidate), slog.String("user", s.displayName))
				iceCandidateAddErr := s.addIceCandidateToPeerConnection(*candidate)
				if iceCandidateAddErr != nil {
					s.logger.Error("readPump: error adding ice candidate", slog.String("error", iceCandidateAddErr.Error()), slog.String("user", s.displayName))
					continue
				}
				// s.logger.Debug("Added ICE candidate", slog.String("candidate", candidate.Candidate), slog.String("user", s.displayName))
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
	s.logger.Info("writePump: started", slog.String("user", s.displayName))
	for {
		select {
		case <-ctx.Done():
			return

		case message, ok := <-s.sendChan:
			if !ok {
				return
			}
			s.logger.Debug("writePump: sending message", slog.Int("length", len(message)), slog.String("user", s.displayName), slog.String("msg", string(message)))
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
	err := json.Unmarshal(payload, &out)
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

		if s.roomId != 0 && s.user != nil {
			room, _ := s.roomManager.GetRoom(s.roomId)
			removedMember := room.RemoveMember(s.user)

			if !removedMember {
				s.logger.Warn("Could not remove member from room", slog.String("clientID", s.displayName), slog.Any("roomId", s.roomId))
			}
		}

		s.socketConn.Close()
		close(s.sendChan)

		s.user.CleanUpUser()

		s.logger.Info("Session closed", "user", s.displayName)
	})
}
