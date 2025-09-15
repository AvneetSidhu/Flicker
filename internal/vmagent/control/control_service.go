package control

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ControlService struct {
	client controlpb.InputServiceClient
	logger *slog.Logger
	ch     chan<- *controlpb.InputMessage
}

func NewControlService(client controlpb.InputServiceClient, inputForwardingChan chan<- *controlpb.InputMessage, logger *slog.Logger) *ControlService {
	return &ControlService{client: client, ch: inputForwardingChan, logger: logger}
}

func (s *ControlService) StartControlStream(ctx context.Context) {
	stream, err := s.client.StreamInputs(context.Background(), &emptypb.Empty{})
	if err != nil {
		s.logger.Error("error initiating gRPC stream", slog.Any("error", err))
		return
	}

	defer close(s.ch)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutdown signal received: shutting down input stream in control service")
			return
		default:
			msg, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					s.logger.Info("stream ended by server", slog.Any("info", err))
					return
				}
				s.logger.Error("error receiving from stream in control service", slog.Any("error", err))
				continue
			}
			s.ch <- msg
		}
	}
}
