package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"github.com/AvneetSidhu/Flicker/internal/vmagent"
	av_service "github.com/AvneetSidhu/Flicker/internal/vmagent/avservice"
	control_service "github.com/AvneetSidhu/Flicker/internal/vmagent/control"
	"github.com/AvneetSidhu/Flicker/internal/vmagent/injector"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)


func loadConfig() (*vmagent.Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env file, %v", err)
	}

	listenPortStr := os.Getenv("LISTEN_PORT")
	if listenPortStr == "" {
		return nil, fmt.Errorf("missing LISTEN_PORT")
	}

	sfuForwardAddr := os.Getenv("SFU_FORWARD_ADDR")
	if sfuForwardAddr == "" {
		return nil, fmt.Errorf("missing SFU_FORWARD_ADDR")
	}

	sfuGrpcAddr := os.Getenv("SFU_GRPC_ADDR")
	if sfuGrpcAddr == "" {
		return nil, fmt.Errorf("missing SFU_GRPC_ADDR")
	}

	return &vmagent.Config{
		SFUForwardAddr: sfuForwardAddr,
		AvListenPort: listenPortStr,
		SFUgRPCAddr: sfuGrpcAddr,
	}, nil 
}

func runAVService(ctx context.Context, cfg *vmagent.Config, logger *slog.Logger) error {
	listenConnection, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%s", cfg.AvListenPort))
	if err != nil {
		return err
	}

	sfuConnection, err := net.Dial("udp", cfg.SFUForwardAddr)
	if err != nil {
		return err
	}

	avService := av_service.NewAVService(sfuConnection, listenConnection, logger)

	go func() {
		defer listenConnection.Close()
		defer sfuConnection.Close()
		avService.StartIngest(ctx)
	}()

	return nil
}

func runControlService(ctx context.Context, cfg *vmagent.Config, logger *slog.Logger, ch chan<- *controlpb.InputMessage) error {
	gRPCConnection, err := grpc.NewClient(cfg.SFUgRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	client := controlpb.NewInputServiceClient(gRPCConnection)
	controlService := control_service.NewControlService(client, ch, logger)

	go func() {
		defer gRPCConnection.Close()
		controlService.StartControlStream(ctx)
	}()
	return nil
}

func runInjectorService(ctx context.Context, logger *slog.Logger, ch <-chan *controlpb.InputMessage) error {
	injectorService :=  injector.NewInjectorService(ch, logger)

	go func() {
		injectorService.StartReadingInput(ctx)
	}()
	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("error loading env variables", slog.Any("err", err))
		os.Exit(1)
	}

	if err := runAVService(ctx, cfg, logger); err != nil {
		logger.Error("error starting av service", slog.Any("err", err))
	}

	ch := make(chan *controlpb.InputMessage, 1000)

	if err := runControlService(ctx, cfg, logger, ch); err != nil {
		logger.Error("error starting control service", slog.Any("err", err))
		os.Exit(1)
	}

	if err := runInjectorService(ctx, logger, ch); err != nil {
		logger.Error("error starting injector service", slog.Any("err", err))
		os.Exit(1)
	}
	
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	logger.Info("Shutting down...")
	cancel()
	time.Sleep(time.Second)
}
