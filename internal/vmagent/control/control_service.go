package control

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ControlService struct {
	client controlpb.InputServiceClient
	logger *slog.Logger
	ch     chan<- *controlpb.InputMessage
	roomID string
}

func NewControlService(client controlpb.InputServiceClient, inputForwardingChan chan<- *controlpb.InputMessage, logger *slog.Logger, roomID string) *ControlService {
	return &ControlService{client: client, ch: inputForwardingChan, logger: logger, roomID: roomID}
}

func (s *ControlService) StartControlStream(ctx context.Context) {
	md := metadata.Pairs("room-id", s.roomID)
	streamctx := metadata.NewOutgoingContext(ctx, md)

	stream, err := s.client.StreamInputs(streamctx, &emptypb.Empty{})
	if err != nil {
		s.logger.Error("error initiating gRPC stream", slog.Any("error", err))
		return
	}
	defer close(s.ch)

	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.logger.Info("stream ended by server", slog.Any("info", err))
				return
			}

			select {
			case <-ctx.Done():
				s.logger.Info("shutdown signal received: shutting down input stream in control service")
				return
			default:
			}

			s.logger.Error("error receiving from stream in control service", slog.Any("error", err))
			continue
		}

		select {
		case s.ch <- msg:
		case <-ctx.Done():
			s.logger.Info("shutdown signal received while sending message to channel")
			return
		}
	}
}
