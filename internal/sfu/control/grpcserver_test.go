package control

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeInputStream struct {
	grpc.ServerStream
	ctx context.Context

	mu       sync.Mutex
	messages []*controlpb.InputMessage
}

func (s *fakeInputStream) Context() context.Context {
	return s.ctx
}

func (s *fakeInputStream) Send(message *controlpb.InputMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return nil
}

func TestSerializeMessagePreservesAllPayloadFields(t *testing.T) {
	tests := []struct {
		name  string
		event *CommandEvent
		check func(*testing.T, *controlpb.InputMessage)
	}{
		{
			name:  "keyboard",
			event: &CommandEvent{Type: KeyboardEvent, RoomID: "room", Key: "A", Action: "down"},
			check: func(t *testing.T, message *controlpb.InputMessage) {
				input := message.GetKeyboard()
				if input == nil || input.Key != "A" || input.Action != "down" {
					t.Fatalf("unexpected keyboard message: %#v", message)
				}
			},
		},
		{
			name:  "mouse",
			event: &CommandEvent{Type: MouseEvent, RoomID: "room", X: 12.5, Y: 20.25, Action: "click", Dragging: true},
			check: func(t *testing.T, message *controlpb.InputMessage) {
				input := message.GetMouse()
				if input == nil || input.X != 12.5 || input.Y != 20.25 || input.Action != "click" || !input.Dragging {
					t.Fatalf("unexpected mouse message: %#v", message)
				}
			},
		},
		{
			name:  "scroll",
			event: &CommandEvent{Type: ScrollEvent, RoomID: "room", Direction: "down"},
			check: func(t *testing.T, message *controlpb.InputMessage) {
				input := message.GetScroll()
				if input == nil || input.Direction != "down" {
					t.Fatalf("unexpected scroll message: %#v", message)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := serializeMessage(test.event)
			if message == nil || message.RoomID != "room" {
				t.Fatalf("unexpected message: %#v", message)
			}
			test.check(t, message)
		})
	}

	if message := serializeMessage(&CommandEvent{Type: "unknown"}); message != nil {
		t.Fatalf("expected unknown event to be dropped, got %#v", message)
	}
}

func TestStartDropsMessagesWithoutConnectedAgent(t *testing.T) {
	commands := make(chan *CommandEvent, 1)
	server := NewGRPCServer(testLogger(), "0", "test-token", commands)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		server.Start(ctx)
		close(done)
	}()

	commands <- &CommandEvent{Type: KeyboardEvent, RoomID: "missing", Key: "A", Action: "down"}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

func TestStartForwardsToConnectedAgent(t *testing.T) {
	commands := make(chan *CommandEvent, 1)
	server := NewGRPCServer(testLogger(), "0", "test-token", commands)
	stream := &fakeInputStream{ctx: context.Background()}
	server.streams["room"] = stream

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		server.Start(ctx)
		close(done)
	}()

	commands <- &CommandEvent{Type: MouseEvent, RoomID: "room", Action: "drag", X: 4, Y: 8, Dragging: true}
	deadline := time.Now().Add(time.Second)
	for {
		stream.mu.Lock()
		count := len(stream.messages)
		stream.mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("message was not forwarded")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.messages[0].GetMouse().Action != "drag" {
		t.Fatalf("mouse action was lost: %#v", stream.messages[0])
	}
}

func TestStreamInputsRequiresRoomMetadata(t *testing.T) {
	server := NewGRPCServer(testLogger(), "0", "test-token", make(chan *CommandEvent))
	if err := server.StreamInputs(&emptypb.Empty{}, &fakeInputStream{ctx: context.Background()}); err == nil {
		t.Fatal("expected missing metadata error")
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("other", "value"))
	if err := server.StreamInputs(&emptypb.Empty{}, &fakeInputStream{ctx: ctx}); err == nil {
		t.Fatal("expected missing room-id error")
	}

	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("room-id", "room", "authorization", "Bearer wrong-token"))
	if err := server.StreamInputs(&emptypb.Empty{}, &fakeInputStream{ctx: ctx}); err == nil {
		t.Fatal("expected invalid authorization error")
	}
}
