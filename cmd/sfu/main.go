package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"

	"syscall"
	"time"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"github.com/AvneetSidhu/Flicker/internal/sfu"
	"github.com/AvneetSidhu/Flicker/internal/sfu/control"
	"github.com/AvneetSidhu/Flicker/internal/sfu/ingest"
	"github.com/AvneetSidhu/Flicker/internal/sfu/room"
	"github.com/AvneetSidhu/Flicker/internal/sfu/server"
	"github.com/AvneetSidhu/Flicker/internal/sfu/vm"
	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v3"
	"google.golang.org/grpc"
)

func loadConfig() (*sfu.Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env, %v", err)
	}

	wsPort := os.Getenv("PORT")
	if wsPort == "" {
		return nil, fmt.Errorf("missing PORT")
	}

	tailnet := os.Getenv("TAILNET_IP")
	if tailnet == "" {
		return nil, fmt.Errorf("missing TAILNET_IP")
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		return nil, fmt.Errorf("missing LOG_LEVEL")
	}

	grpcPort := os.Getenv("SFU_GRPC_PORT")
	if grpcPort == "" {
		return nil, fmt.Errorf("missing SFU_GRPC_PORT")
	}

	vmBaseName := os.Getenv("VM_BASE_NAME")
	if vmBaseName == "" {
		return nil, fmt.Errorf("missing VM_BASE_NAME")
	}

	return &sfu.Config{
		Port:       wsPort,
		Tailnet:    tailnet,
		LogLevel:   logLevel,
		GRPCPort:   grpcPort,
		VMBaseName: vmBaseName,
	}, nil
}

func startGRPCServer(ctx context.Context, cfg *sfu.Config, logger *slog.Logger, controlMessageEventBus <-chan *control.CommandEvent) error {
	server := control.NewGRPCServer(logger, cfg.GRPCPort, controlMessageEventBus)
	grpcServer := grpc.NewServer()

	controlpb.RegisterInputServiceServer(grpcServer, server)

	go server.Start(ctx)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.GRPCPort))
	if err != nil {
		logger.Error("failed to listen: %v", err)
		return err
	}

	go func() error {
		grpcServer.Serve(lis)
		return nil
	}()
	return nil
}

func startWebSocketServer(ctx context.Context, cfg *sfu.Config, logger *slog.Logger, roomManager *room.RoomManager) error {
	me := &webrtc.MediaEngine{}
	me.RegisterDefaultCodecs()
	se := webrtc.SettingEngine{}
	se.SetNAT1To1IPs([]string{cfg.Tailnet}, webrtc.ICECandidateTypeHost)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se))

	sfuServer := server.NewSignalServer(logger, api, roomManager)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { sfuServer.WsConnectHandler(ctx, w, r, api) })
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	http.HandleFunc("/rooms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(roomManager.GetRooms())
	})
	go func() error {
		err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%s", cfg.Port), nil)
		if err != nil {
			logger.Error("error starting server", "err", err)
			os.Exit(1)
			return err
		}
		return nil
	}()

	logger.Info("WebSocket server started on port", "port", cfg.Port)
	return nil
}

func parsedLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("error loading env variables", "err", err)
		os.Exit(1)
	}

	logLevel := parsedLogLevel(cfg.LogLevel)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	logger.Debug("logger initialized")

	controlMessageEventBus := make(chan *control.CommandEvent, 5000)

	if err := startGRPCServer(ctx, cfg, logger, controlMessageEventBus); err != nil {
		logger.Error("error starting grpc server", "err", err)
		os.Exit(1)
	}

	ingestService := ingest.NewIngestManager(logger)

	vmService := vm.NewVMService(logger, vm.VMServiceConfig{
		BaseDomain: cfg.VMBaseName,
		SFUHost:    cfg.Tailnet,
		GRPCPort:   cfg.GRPCPort,
	})

	roomManager := room.NewRoomManager(logger, controlMessageEventBus, ingestService, vmService)

	if err := startWebSocketServer(ctx, cfg, logger, roomManager); err != nil {
		slog.Error("error starting web socket server", "err", err)
		os.Exit(1)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, os.Interrupt, syscall.SIGTERM)

	<-stop
	logger.Info("shutting down...")
	cancel()
	time.Sleep(time.Second)
}
