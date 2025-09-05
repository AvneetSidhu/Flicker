package injector

import (
	"testing"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
)

func TestNewInjectorService(t *testing.T) {
	ch := make(chan *controlpb.InputMessage, 1000)

	injectorService := NewInjectorService(ch)
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
