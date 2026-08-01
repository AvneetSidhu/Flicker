package control

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeInputClient struct {
	ctx    context.Context
	stream controlpb.InputService_StreamInputsClient
}

func (c *fakeInputClient) StreamInputs(ctx context.Context, _ *emptypb.Empty, _ ...grpc.CallOption) (controlpb.InputService_StreamInputsClient, error) {
	c.ctx = ctx
	return c.stream, nil
}

type fakeInputClientStream struct {
	grpc.ClientStream
	messages []*controlpb.InputMessage
	index    int
}

func (s *fakeInputClientStream) Recv() (*controlpb.InputMessage, error) {
	if s.index >= len(s.messages) {
		return nil, io.EOF
	}
	message := s.messages[s.index]
	s.index++
	return message, nil
}

func TestStartControlStreamForwardsMessagesAndSendsRoomMetadata(t *testing.T) {
	stream := &fakeInputClientStream{messages: []*controlpb.InputMessage{{RoomID: "room", Payload: &controlpb.InputMessage_Keyboard{Keyboard: &controlpb.KeyboardInput{Key: "A", Action: "down"}}}}}
	client := &fakeInputClient{stream: stream}
	output := make(chan *controlpb.InputMessage, 1)
	service := NewControlService(client, output, slog.Default(), "room", "test-token")
	service.retryDelay = time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		service.StartControlStream(ctx)
		close(done)
	}()

	message := <-output
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("control service did not stop after cancellation")
	}

	metadata, ok := metadata.FromOutgoingContext(client.ctx)
	if !ok || len(metadata.Get("room-id")) != 1 || metadata.Get("room-id")[0] != "room" || metadata.Get("authorization")[0] != "Bearer test-token" {
		t.Fatalf("missing room metadata: %#v", metadata)
	}
	if message.GetKeyboard().Key != "A" {
		t.Fatalf("unexpected forwarded message: %#v", message)
	}
}
