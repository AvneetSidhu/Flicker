package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/AvneetSidhu/Flicker/internal/sfu/room"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type SignalServer struct {
	roomManager *room.RoomManager
	logger      *slog.Logger
	upgrader    websocket.Upgrader
	sessions    map[string]*session
	webrtcAPI   *webrtc.API
	mu          sync.RWMutex
}

func NewSignalServer(roomManager *room.RoomManager, logger *slog.Logger, webrtcAPI *webrtc.API) *SignalServer {
	return &SignalServer{
		roomManager: roomManager,
		logger:      logger,
		upgrader: websocket.Upgrader{ReadBufferSize: 1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		sessions:  make(map[string]*session),
		webrtcAPI: webrtcAPI,
	}
}

func (s *SignalServer) WsConnectHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.URL.Query().Get("username")

	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade to WebSocket", http.StatusInternalServerError)
		return
	}

	session := &session{
		displayName: username,
		socketConn:  conn,
		sendChan:    make(chan []byte, 256),
		onClose:     s.cleanUp(),
		api:         s.webrtcAPI,
		logger:      s.logger,
	}

	s.sessions[username] = session

	go session.readPump(ctx)
	go session.writePump(ctx)
}

func (s *SignalServer) cleanUp() func(clientID string) {

	return func(clientID string) {
		s.mu.Lock()
		defer s.mu.Unlock()

		session, ok := s.sessions[clientID]

		if !ok || session.user == nil {
			return
		}

		room, _ := s.roomManager.GetRoom(session.roomId)
		removedMember := room.RemoveMember(session.user)

		if !removedMember {
			s.logger.Warn("Could not remove member from room", slog.String("clientID", clientID), slog.Any("roomId", session.roomId))
		}

		delete(s.sessions, clientID)
	}
}
