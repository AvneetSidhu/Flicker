package main

import (
	"log"
	"net"
	"time"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)


type mockInputService struct {
    controlpb.UnimplementedInputServiceServer
}

func (s *mockInputService) StreamInputs(_ *emptypb.Empty, stream controlpb.InputService_StreamInputsServer) error {
    for i := 0; i < 10; i++ {
        msg := &controlpb.InputMessage{
            Payload: &controlpb.InputMessage_Keyboard{
                Keyboard: &controlpb.KeyboardInput{
                    Key:    "a",
                    Action: "press",
                },
            },
        }
        if err := stream.Send(msg); err != nil {
            return err
        }
        time.Sleep(500 * time.Millisecond)
    }
    return nil
}

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    server := grpc.NewServer()
    controlpb.RegisterInputServiceServer(server, &mockInputService{})

    log.Println("Mock gRPC server running on :50051")
    if err := server.Serve(lis); err != nil {
        log.Fatalf("server exited with error: %v", err)
    }
}
