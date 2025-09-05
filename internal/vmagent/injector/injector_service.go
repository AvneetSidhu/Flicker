package injector

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
)

type InjectorService struct {
	inputChan <-chan *controlpb.InputMessage
	logger *slog.Logger
}

func NewInjectorService(ch <-chan *controlpb.InputMessage) *InjectorService {
	return &InjectorService{inputChan: ch, logger: slog.Default().With("service", "injector")}
}

func (s *InjectorService) StartReadingInput(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutdown signal received, shutting down injector service")
			return
		default:
			msg := <-s.inputChan
			if msg == nil {
				continue
			}
			s.handleInput(msg)
		}
	}
}

func (s *InjectorService) handleInput(msg *controlpb.InputMessage) {
	fmt.Println(msg)
}