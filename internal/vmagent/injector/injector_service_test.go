package injector

import (
	"log/slog"
	"testing"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
)

func TestNewInjectorService(t *testing.T) {
	ch := make(chan *controlpb.InputMessage, 1000)

	injectorService := NewInjectorService(ch, slog.Default().With("service", "av"))
	if injectorService.logger == nil {
		t.Error("expected injectorService to not nil")
	}

	if injectorService.inputChan == nil {
		t.Error("expected inputChan to not be nil")
	}

}

// func TestStartReadingInput(t *testing.T) {
// 	ch := make(chan *controlpb.InputMessage, 1000)

// 	injectorService := NewInjectorService(ch)
// 	ctx, cancel := context.WithCancel(context.Background())

// 	injectorService.StartReadingInput(ctx)

// 	ch <- &controlpb.InputMessage{
//             Payload: &controlpb.InputMessage_Keyboard{
//                 Keyboard: &controlpb.KeyboardInput{
//                     Key:    "a",
//                     Action: "press",
//                 	},
//             	},
//         	}

// }
