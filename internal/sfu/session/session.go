package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AvneetSidhu/Flicker/internal/sfu/ingest"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

const (
	pingInterval = 30 * time.Second
	pongWait     = 60 * time.Second
	writeWait    = 10 * time.Second
)

type Room interface {
	RemoveMember(s *Session) bool
	AddMember(s *Session) bool
}

type roomManager interface {
	GetRoom(roomID string) (Room, bool)
	CreateRoom(ctx context.Context, roomName string, creator string) (string, error)
}

type Session struct {
	DisplayName string
	UserID      string

	socketConn *websocket.Conn
	SendChan   chan []byte

	api    *webrtc.API
	Logger *slog.Logger

	roomID      string
	roomManager roomManager

	//webrtc
	PeerConn *webrtc.PeerConnection

	VideoBuf chan *ingest.PooledPacket
	AudioBuf chan *ingest.PooledPacket

	audioTrack *webrtc.TrackLocalStaticRTP
	videoTrack *webrtc.TrackLocalStaticRTP

	dataChan *webrtc.DataChannel

	videoBytesSent   uint32
	videoPacketsSent uint32

	audioBytesSent   uint32
	audioPacketsSent uint32

	latestAudioPacketTime uint32
	latestVideoPacketTime uint32

	audioSSRC uint32
	videoSSRC uint32

	nonMasterRTPStartTime uint32
	masterRTPStartTime    uint32

	activeStream atomic.Bool

	cleanUpOnce    sync.Once
	rtpWriteCancel context.CancelFunc
	cancel         context.CancelFunc
	mu             sync.RWMutex
}

func NewSession(displayName string, userID string, conn *websocket.Conn, api *webrtc.API, logger *slog.Logger, roomManager roomManager, cancelFunc context.CancelFunc) *Session {
	return &Session{
		DisplayName: displayName,
		UserID:      userID,

		AudioBuf: make(chan *ingest.PooledPacket, 1000),
		VideoBuf: make(chan *ingest.PooledPacket, 1000),

		socketConn: conn,
		SendChan:   make(chan []byte, 256),

		api:    api,
		Logger: logger,

		roomManager: roomManager,

		cleanUpOnce: sync.Once{},
		cancel:      cancelFunc,
		mu:          sync.RWMutex{},
	}
}

func (s *Session) ReadPump(ctx context.Context) {
	defer func() {
		s.CleanUpSession()
	}()
	s.Logger.Debug("readPump: started", slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
	s.socketConn.SetReadDeadline(time.Now().Add(pongWait))
	s.socketConn.SetPongHandler(func(string) error {
		s.socketConn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {

		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := s.socketConn.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					s.Logger.Error("readPump: unexpected close error", slog.String("error", err.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
				} else {
					s.Logger.Error("readPump: error reading message", slog.String("error", err.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
				}

				return
			}

			wsMessage, err := parseWSMessage(message)
			if err != nil {
				s.Logger.Error("readPump: error parsing ws message", slog.String("error", err.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
				continue
			}

			switch wsMessage.Type {
			case "offer":
				offer, err := parseWSMessagePayload[WebRTCOfferPayload](wsMessage.Payload)
				if err != nil {
					s.Logger.Error("readPump: error parsing offer payload", slog.String("error", err.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
					continue
				}

				createPeerErr := s.createPeerConnection(*offer, ctx)
				if createPeerErr != nil {
					s.Logger.Error("readPump: error creating peer connection", slog.String("error", createPeerErr.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
					continue
				}

			case "ice-candidate":
				candidate, parseCandidateErr := parseWSMessagePayload[ICECandidatePayload](wsMessage.Payload)
				if parseCandidateErr != nil {
					s.Logger.Error("readPump: error parsing ice candidate payload", slog.String("error", parseCandidateErr.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
					continue
				}

				// s.logger.Debug("Received ICE candidate", slog.String("candidate", candidate.Candidate), slog.String("user", s.DisplayName))
				iceCandidateAddErr := s.addIceCandidateToPeerConnection(*candidate)
				if iceCandidateAddErr != nil {
					s.Logger.Error("readPump: error adding ice candidate", slog.String("error", iceCandidateAddErr.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
					continue
				}
				// s.logger.Debug("   ICE candidate", slog.String("candidate", candidate.Candidate), slog.String("user", s.DisplayName))
			case "join-room":
				joinRoom, err := parseWSMessagePayload[JoinRoomPayload](wsMessage.Payload)
				if err != nil {
					s.Logger.Error("readPump: error parsing join room payload", slog.String("error", err.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
				}

				err = s.joinRoom(joinRoom.RoomID)

				if err != nil {
					s.Logger.Error("error joining room", "err", err, "display name", s.DisplayName)
				}

				s.Logger.Info("user has joined a room", "userID", s.UserID, "displayName", s.DisplayName, "roomID", s.roomID)
			case "create-room":
				createRoom, err := parseWSMessagePayload[CreateRoomPayload](wsMessage.Payload)
				s.Logger.Info("create room request received", "userID", s.UserID, "displayName", s.DisplayName)
				if err != nil {
					s.Logger.Error("readPump: error parsing create room payload", slog.String("error", err.Error()), slog.String("DisplayName", s.DisplayName), slog.String("userID", s.UserID))
					continue
				}

				roomID, err := s.createRoom(ctx, createRoom.RoomName)
				if err != nil {
					s.Logger.Error("error creating room", "err", err, "display name", s.DisplayName)
					continue
				}

				s.sendRoomCreated(roomID)

			default:
				s.Logger.Error("readPump: unknown message type", slog.String("type", wsMessage.Type), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
				continue
			}
		}
	}
}

func (s *Session) WritePump(ctx context.Context) {
	s.Logger.Debug("writePump: started", slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			s.socketConn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.socketConn.WriteMessage(websocket.PingMessage, nil); err != nil {
				s.Logger.Error("writePump: ping failed", slog.String("error", err.Error()), slog.String("DisplayName", s.DisplayName))
				return
			}

		case message, ok := <-s.SendChan:
			if !ok {
				return
			}
			s.Logger.Debug("writePump: sending message", slog.Int("length", len(message)), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID), slog.String("msg", string(message)))
			s.socketConn.SetWriteDeadline(time.Now().Add(writeWait))
			writeErr := s.socketConn.WriteMessage(websocket.TextMessage, message)
			if writeErr != nil {
				s.Logger.Error("writePump: error writing message", slog.String("error", writeErr.Error()), slog.String("DisplayName", s.DisplayName), slog.String("UserID", s.UserID))
				return
			}
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

func (s *Session) joinRoom(roomID string) error {
	s.roomID = roomID

	room, ok := s.roomManager.GetRoom(roomID)
	if !ok {
		return fmt.Errorf("could not retrieve room with roomID %s", roomID)
	}

	room.AddMember(s)
	return nil
}

func (s *Session) createRoom(ctx context.Context, roomName string) (string, error) {
	roomID, err := s.roomManager.CreateRoom(ctx, roomName, s.DisplayName)
	if err != nil {
		return "", err
	}
	return roomID, nil
}

func (s *Session) SetActiveStream(b bool) {
	s.activeStream.Store(b)
}

func (s *Session) IsActive() bool {
	return s.activeStream.Load()
}

func (s *Session) sendRoomCreated(roomID string) {
	payload, err := json.Marshal(RoomCreatedPayload{RoomID: roomID})
	if err != nil {
		s.Logger.Error("error marshalling room-created payload", "err", err)
		return
	}
	msg, err := json.Marshal(WSMessage{Type: "room-created", Payload: payload})
	if err != nil {
		s.Logger.Error("error marshalling room-created message", "err", err)
		return
	}
	select {
	case s.SendChan <- msg:
	default:
		s.Logger.Warn("SendChan full, dropping room-created message")
	}
}

func (s *Session) CleanUpSession() {
	s.cleanUpOnce.Do(func() {
		if s.IsActive() { //currently forwarding rtp
			s.rtpWriteCancel()
		}

		if s.roomID != "" { // if in a room, remove from room first
			room, ok := s.roomManager.GetRoom(s.roomID)
			if !ok {
				return
			}
			removed := room.RemoveMember(s)

			if !removed {
				s.Logger.Error("error removing member from room", "userID", s.UserID, "roomID", s.roomID)
			}
		}

		if s.PeerConn != nil {
			s.PeerConn.Close()
		}

		close(s.AudioBuf)
		close(s.VideoBuf)
		close(s.SendChan)

		s.socketConn.Close()
	})
}
