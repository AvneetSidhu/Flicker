package injector

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
)

type InjectorService struct {
	inputChan <-chan *controlpb.InputMessage
	logger    *slog.Logger
}

func NewInjectorService(ch <-chan *controlpb.InputMessage, logger *slog.Logger) *InjectorService {
	return &InjectorService{inputChan: ch, logger: logger}
}

func (s *InjectorService) StartReadingInput(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutdown signal received: ending ingest in injector service")
			return
		case msg := <-s.inputChan:
			if msg != nil {
				s.handleInput(msg)
			}
		}
	}
}

func (s *InjectorService) handleInput(msg *controlpb.InputMessage) {
	fmt.Println(msg)
}
