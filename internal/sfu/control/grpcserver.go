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
}

func NewGRPCServer(logger *slog.Logger, port string, commandCh <-chan *CommandEvent) *GRPCServer {
	return &GRPCServer{
		logger:      logger,
		controlRecv: commandCh,
		streams:     make(map[string]controlpb.InputService_StreamInputsServer),
		mu:          sync.RWMutex{},
		port:        port,
	}
}

func (s *GRPCServer) Start(ctx context.Context) {
	s.logger.Info(fmt.Sprintf("started grpc server on port %s", s.port))
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Control Service shutting down...")
			return
		case controlMsg := <-s.controlRecv:
			s.logger.Debug("received controlMsg", "controlMSG", controlMsg)

			msgProto := serializeMessage(controlMsg)

			s.mu.RLock()
			err := s.streams[controlMsg.RoomID].Send(msgProto)
			s.mu.RUnlock()

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

	s.mu.Lock()
	s.streams[roomIDs[0]] = stream
	s.mu.Unlock()

	go func(roomID string) {
		<-stream.Context().Done()
		s.mu.Lock()
		delete(s.streams, roomID)
		s.mu.Unlock()
		s.logger.Info("stream closed", "roomID", roomID)
	}(roomIDs[0])

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
