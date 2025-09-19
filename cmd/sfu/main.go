package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"syscall"
	"time"

	"github.com/AvneetSidhu/Flicker/internal/sfu"
	server "github.com/AvneetSidhu/Flicker/internal/sfu/signal"
	"github.com/joho/godotenv"
	"github.com/pion/webrtc/v3"
)

func loadConfig() (*sfu.Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env, %v", err)
	}
	return &sfu.Config{
		Port:     os.Getenv("PORT"),
		Tailnet:  os.Getenv("TAILNET_IP"),
		LogLevel: os.Getenv("LOG_LEVEL"),
	}, nil
}

func startWebSocketServer(ctx context.Context, cfg *sfu.Config, logger *slog.Logger) error {
	me := &webrtc.MediaEngine{}
	me.RegisterDefaultCodecs()
	se := webrtc.SettingEngine{}
	se.SetNAT1To1IPs([]string{cfg.Tailnet}, webrtc.ICECandidateTypeHost)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithSettingEngine(se))

	sfuServer := server.NewSignalServer(logger, api)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { sfuServer.WsConnectHandler(ctx, w, r) })
	go func() error {
		err := http.ListenAndServe(fmt.Sprintf(":%s", cfg.Port), nil)
		if err != nil {
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

	if err := startWebSocketServer(ctx, cfg, logger); err != nil {
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
