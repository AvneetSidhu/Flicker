package control

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type GRPCServer struct {
	controlpb.UnimplementedInputServiceServer
	logger      *slog.Logger
	controlRecv <-chan *CommandEvent
	streams     map[string]controlpb.InputService_StreamInputsServer
	mu          sync.RWMutex
	port        string
	authToken   string
}

func NewGRPCServer(logger *slog.Logger, port string, authToken string, commandCh <-chan *CommandEvent) *GRPCServer {
	return &GRPCServer{
		logger:      logger,
		controlRecv: commandCh,
		streams:     make(map[string]controlpb.InputService_StreamInputsServer),
		mu:          sync.RWMutex{},
		port:        port,
		authToken:   authToken,
	}
}

func (s *GRPCServer) Start(ctx context.Context) {
	s.logger.Info(fmt.Sprintf("started grpc server on port %s", s.port))
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Control Service shutting down...")
			return
		case controlMsg, ok := <-s.controlRecv:
			if !ok {
				return
			}
			s.logger.Debug("received controlMsg", "controlMSG", controlMsg)

			msgProto := serializeMessage(controlMsg)
			if msgProto == nil {
				s.logger.Warn("dropping unknown control message", "type", controlMsg.Type, "roomID", controlMsg.RoomID)
				continue
			}

			s.mu.RLock()
			stream := s.streams[controlMsg.RoomID]
			s.mu.RUnlock()
			if stream == nil {
				s.logger.Warn("dropping control message with no connected agent", "roomID", controlMsg.RoomID)
				continue
			}

			err := stream.Send(msgProto)

			if err != nil {
				s.logger.Error("error sending message", "err", err, "roomID", controlMsg.RoomID, "msg", controlMsg)
			}
		}
	}
}

func (s *GRPCServer) StreamInputs(_ *emptypb.Empty, stream controlpb.InputService_StreamInputsServer) error {
	ctx := stream.Context()
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		s.logger.Error("agent tried connecting with no header metadata")
		return fmt.Errorf("no metadata in grpc request header")
	}

	roomIDs := md["room-id"]
	if len(roomIDs) == 0 {
		s.logger.Error("agent sent no roomID when connecting")
		return fmt.Errorf("agent tried connecting with no roomID")
	}
	if authorization := md.Get("authorization"); len(authorization) != 1 || authorization[0] != "Bearer "+s.authToken {
		return fmt.Errorf("unauthorized control agent")
	}

	s.mu.Lock()
	s.streams[roomIDs[0]] = stream
	s.mu.Unlock()

	<-stream.Context().Done()
	s.mu.Lock()
	delete(s.streams, roomIDs[0])
	s.mu.Unlock()
	s.logger.Info("stream closed", "roomID", roomIDs[0])
	return nil
}

func serializeMessage(controlMessage *CommandEvent) *controlpb.InputMessage {
	var msg *controlpb.InputMessage

	switch controlMessage.Type {
	case "keyboard":
		msg = &controlpb.InputMessage{
			Payload: &controlpb.InputMessage_Keyboard{
				Keyboard: &controlpb.KeyboardInput{
					Key:    controlMessage.Key,
					Action: controlMessage.Action},
			},
			RoomID: controlMessage.RoomID,
		}
	case "mouse":
		msg = &controlpb.InputMessage{
			Payload: &controlpb.InputMessage_Mouse{
				Mouse: &controlpb.MouseInput{
					X:        controlMessage.X,
					Y:        controlMessage.Y,
					Action:   controlMessage.Action,
					Dragging: controlMessage.Dragging,
				},
			},
			RoomID: controlMessage.RoomID,
		}
	case "scroll":
		msg = &controlpb.InputMessage{
			Payload: &controlpb.InputMessage_Scroll{
				Scroll: &controlpb.ScrollInput{
					Direction: controlMessage.Direction,
				},
			},
			RoomID: controlMessage.RoomID,
		}
	}

	return msg
}
