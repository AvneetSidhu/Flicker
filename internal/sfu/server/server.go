package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/AvneetSidhu/Flicker/internal/sfu/room"
	"github.com/AvneetSidhu/Flicker/internal/sfu/session"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

type SignalServer struct {
	roomManager *room.RoomManager
	logger      *slog.Logger
	upgrader    websocket.Upgrader
	mu          sync.RWMutex
}

func NewSignalServer(logger *slog.Logger, webrtcAPI *webrtc.API, roomManager *room.RoomManager) *SignalServer {
	return &SignalServer{
		roomManager: roomManager,
		logger:      logger,
		upgrader: websocket.Upgrader{ReadBufferSize: 1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (s *SignalServer) WsConnectHandler(ctx context.Context, w http.ResponseWriter, r *http.Request, api *webrtc.API) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	displayName := r.URL.Query().Get("displayname")
	userID := r.URL.Query().Get("userid")

	s.logger.Info("New WebSocket connection", slog.String("username", displayName))

	if displayName == "" {
		s.logger.Info("no username in url, connection failed")
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade to WebSocket", http.StatusInternalServerError)
		return
	}

	sessCtx, cancel := context.WithCancel(ctx)

	session := session.NewSession(displayName, userID, conn, api, s.logger, s.roomManager, cancel)

	go session.WritePump(sessCtx)
	go session.ReadPump(sessCtx)
}
