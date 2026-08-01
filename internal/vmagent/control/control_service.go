package control

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ControlService struct {
	client     controlpb.InputServiceClient
	logger     *slog.Logger
	ch         chan<- *controlpb.InputMessage
	roomID     string
	authToken  string
	retryDelay time.Duration
}

func NewControlService(client controlpb.InputServiceClient, inputForwardingChan chan<- *controlpb.InputMessage, logger *slog.Logger, roomID string, authToken string) *ControlService {
	return &ControlService{client: client, ch: inputForwardingChan, logger: logger, roomID: roomID, authToken: authToken, retryDelay: time.Second}
}

func (s *ControlService) StartControlStream(ctx context.Context) {
	for {
		if err := s.receiveStream(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("control stream disconnected; retrying", "err", err, "roomID", s.roomID)
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.retryDelay):
		}
	}
}

func (s *ControlService) receiveStream(ctx context.Context) error {
	streamctx := metadata.NewOutgoingContext(ctx, metadata.Pairs("room-id", s.roomID, "authorization", "Bearer "+s.authToken))
	stream, err := s.client.StreamInputs(streamctx, &emptypb.Empty{})
	if err != nil {
		return err
	}
	s.logger.Info("control stream connected", "roomID", s.roomID)
	for {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		select {
		case s.ch <- msg:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
