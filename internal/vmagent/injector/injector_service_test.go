package injector

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
)

func TestValidateEnvironment(t *testing.T) {
	if err := validateEnvironment("", errors.New("not found"), ":0"); err == nil {
		t.Fatal("expected missing xdotool error")
	}
	if err := validateEnvironment("/usr/bin/xdotool", nil, ""); err == nil {
		t.Fatal("expected missing display error")
	}
	if err := validateEnvironment("/usr/bin/xdotool", nil, ":0"); err != nil {
		t.Fatalf("expected valid environment, got %v", err)
	}
}

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

func TestHandleInputRunsExpectedXdotoolCommands(t *testing.T) {
	service := NewInjectorService(make(chan *controlpb.InputMessage), slog.Default())
	var commands [][]string
	service.run = func(args ...string) error {
		commands = append(commands, args)
		return nil
	}

	service.handleInput(&controlpb.InputMessage{Payload: &controlpb.InputMessage_Keyboard{Keyboard: &controlpb.KeyboardInput{Key: "A", Action: "down"}}})
	service.handleInput(&controlpb.InputMessage{Payload: &controlpb.InputMessage_Keyboard{Keyboard: &controlpb.KeyboardInput{Key: "A", Action: "up"}}})
	service.handleInput(&controlpb.InputMessage{Payload: &controlpb.InputMessage_Scroll{Scroll: &controlpb.ScrollInput{Direction: "up"}}})

	if len(commands) != 3 || commands[0][0] != "keydown" || commands[1][0] != "keyup" || commands[2][0] != "click" || commands[2][1] != "4" {
		t.Fatalf("unexpected xdotool commands: %#v", commands)
	}
}

func TestHandleInputRejectsInvalidCommands(t *testing.T) {
	service := NewInjectorService(make(chan *controlpb.InputMessage), slog.Default())
	called := false
	service.run = func(args ...string) error {
		called = true
		return nil
	}

	service.handleInput(&controlpb.InputMessage{Payload: &controlpb.InputMessage_Keyboard{Keyboard: &controlpb.KeyboardInput{Key: "A", Action: "press"}}})
	service.handleInput(&controlpb.InputMessage{Payload: &controlpb.InputMessage_Mouse{Mouse: &controlpb.MouseInput{X: 2, Y: 0, Action: "move"}}})
	service.handleInput(&controlpb.InputMessage{Payload: &controlpb.InputMessage_Scroll{Scroll: &controlpb.ScrollInput{Direction: "sideways"}}})

	if called {
		t.Fatal("invalid input invoked xdotool")
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
