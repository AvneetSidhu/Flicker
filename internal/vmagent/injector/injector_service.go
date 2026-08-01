package injector

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
)

type InjectorService struct {
	inputChan <-chan *controlpb.InputMessage
	logger    *slog.Logger
	run       func(...string) error
}

func NewInjectorService(ch <-chan *controlpb.InputMessage, logger *slog.Logger) *InjectorService {
	return &InjectorService{inputChan: ch, logger: logger, run: runXdotool}
}

func ValidateEnvironment() error {
	path, err := exec.LookPath("xdotool")
	return validateEnvironment(path, err, os.Getenv("DISPLAY"))
}

func validateEnvironment(xdotoolPath string, xdotoolErr error, display string) error {
	if xdotoolErr != nil {
		return fmt.Errorf("xdotool is required: %w", xdotoolErr)
	}
	if xdotoolPath == "" {
		return fmt.Errorf("xdotool is required")
	}
	if display == "" {
		return fmt.Errorf("DISPLAY must be configured for input injection")
	}
	return nil
}

func (s *InjectorService) StartReadingInput(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("shutdown signal received: ending ingest in injector service")
			return
		case msg, ok := <-s.inputChan:
			if !ok {
				return
			}
			if msg != nil {
				s.handleInput(msg)
			}
		}
	}
}

func (s *InjectorService) handleInput(msg *controlpb.InputMessage) {
	var err error
	switch input := msg.Payload.(type) {
	case *controlpb.InputMessage_Keyboard:
		if input.Keyboard.Action == "down" {
			err = s.run("keydown", input.Keyboard.Key)
		} else if input.Keyboard.Action == "up" {
			err = s.run("keyup", input.Keyboard.Key)
		} else {
			err = fmt.Errorf("unsupported keyboard action %q", input.Keyboard.Action)
		}
	case *controlpb.InputMessage_Mouse:
		mouse := input.Mouse
		if mouse.X < 0 || mouse.X > 1 || mouse.Y < 0 || mouse.Y > 1 {
			err = fmt.Errorf("pointer coordinates must be normalized: x=%f y=%f", mouse.X, mouse.Y)
			break
		}
		if mouse.Action != "move" && mouse.Action != "mousedown" && mouse.Action != "mouseup" {
			err = fmt.Errorf("unsupported mouse action %q", mouse.Action)
			break
		}
		if mouse.Action == "move" || mouse.Action == "mousedown" || mouse.Action == "mouseup" {
			if width, height, geometryErr := displayGeometry(); geometryErr == nil {
				err = s.run("mousemove", strconv.Itoa(int(mouse.X*float64(width))), strconv.Itoa(int(mouse.Y*float64(height))))
			} else {
				err = geometryErr
			}
		}
		if err == nil && mouse.Action == "mousedown" {
			err = s.run("mousedown", "1")
		}
		if err == nil && mouse.Action == "mouseup" {
			err = s.run("mouseup", "1")
		}
	case *controlpb.InputMessage_Scroll:
		button := "5"
		if input.Scroll.Direction == "up" {
			button = "4"
		} else if input.Scroll.Direction != "down" {
			err = fmt.Errorf("unsupported scroll direction %q", input.Scroll.Direction)
			break
		}
		err = s.run("click", button)
	}
	if err != nil {
		s.logger.Error("failed to inject input", "err", err, "roomID", msg.RoomID)
	}
}

func runXdotool(args ...string) error { return exec.Command("xdotool", args...).Run() }

func displayGeometry() (int, int, error) {
	out, err := exec.Command("xdotool", "getdisplaygeometry").Output()
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return 0, 0, strconv.ErrSyntax
	}
	width, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, err
	}
	height, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}
