package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/AvneetSidhu/Flicker/api/controlpb"
	"github.com/AvneetSidhu/Flicker/internal/vmagent"
	control_service "github.com/AvneetSidhu/Flicker/internal/vmagent/control"
	"github.com/AvneetSidhu/Flicker/internal/vmagent/injector"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func loadConfig() (*vmagent.Config, error) {
	err := godotenv.Load("/etc/vmagent.env")
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

	roomID := os.Getenv("ROOM_ID")
	if roomID == "" {
		return nil, fmt.Errorf("missing ROOM_ID")
	}

	audioPort := os.Getenv("AUDIO_PORT")
	if audioPort == "" {
		return nil, fmt.Errorf("missing AUDIO_PORT")
	}

	videoPort := os.Getenv("VIDEO_PORT")
	if videoPort == "" {
		return nil, fmt.Errorf("missing VIDEO_PORT")
	}

	return &vmagent.Config{
		SFUForwardAddr: sfuForwardAddr,
		AvListenPort:   listenPortStr,
		SFUgRPCAddr:    sfuGrpcAddr,
		RoomID:         roomID,
		AudioPort:      audioPort,
		VideoPort:      videoPort,
	}, nil
}

func spawnGStreamer(ctx context.Context, cfg *vmagent.Config, logger *slog.Logger) error {
	fullShellCommand := "/usr/bin/gst-launch-1.0 -v" +
		" pulsesrc ! audio/x-raw,rate=48000,channels=2 ! audioconvert ! opusenc bitrate=32000 ! rtpopuspay pt=111 ! rtpbin.send_rtp_sink_0" +
		" rtpbin.send_rtp_src_0 ! udpsink host=$SFU_IP port=$AUDIO_RTP_PORT" +
		" ximagesrc display-name=$DISPLAY use-damage=0 ! video/x-raw,framerate=30/1 ! videoconvert ! vp8enc error-resilient=partitions keyframe-max-dist=10 cpu-used=5 deadline=1 ! rtpvp8pay pt=96 ! rtpbin.send_rtp_sink_1" +
		" rtpbin.send_rtp_src_1 ! udpsink host=$SFU_IP port=$VIDEO_RTP_PORT" +
		" rtpbin name=rtpbin"

	cmd := exec.CommandContext(
		ctx,
		"/bin/sh",
		"-c",
		fullShellCommand,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("SFU_IP=%s", cfg.SFUForwardAddr),
		fmt.Sprintf("AUDIO_RTP_PORT=%s", cfg.AudioPort),
		fmt.Sprintf("VIDEO_RTP_PORT=%s", cfg.VideoPort),
	)

	logger.Info("Starting GStreamer ingest pipeline using ENV vars", "SFU_IP", cfg.SFUForwardAddr, "AudioPort", cfg.AudioPort, "VideoPort", cfg.VideoPort)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start GStreamer: %w", err)
	}

	logger.Info("Starting GStreamer ingest pipeline", "cmd", cmd.String())

	go func() {
		<-ctx.Done()
		fmt.Println("Context canceled, killing GStreamer...")
		if err := cmd.Process.Kill(); err != nil {
			fmt.Println("Failed to kill GStreamer:", err)
		}
	}()

	go func() {
		if err := cmd.Wait(); err != nil {
			if ctx.Err() == nil {
				logger.Error("GStreamer exited with error", "err", err)
			}
		}
	}()

	return nil
}

// func runAVService(ctx context.Context, cfg *vmagent.Config, logger *slog.Logger) error {
// 	listenConnection, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%s", cfg.AvListenPort))
// 	if err != nil {
// 		return err
// 	}

// 	sfuConnection, err := net.Dial("udp", cfg.SFUForwardAddr)
// 	if err != nil {
// 		return err
// 	}

// 	avService := av_service.NewAVService(sfuConnection, listenConnection, logger, cfg.RoomID)

// 	go func() {
// 		defer listenConnection.Close()
// 		defer sfuConnection.Close()
// 		avService.StartIngest(ctx)
// 	}()

// 	return nil
// }

func runControlService(ctx context.Context, cfg *vmagent.Config, logger *slog.Logger, ch chan<- *controlpb.InputMessage) error {
	gRPCConnection, err := grpc.NewClient(cfg.SFUgRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	client := controlpb.NewInputServiceClient(gRPCConnection)
	controlService := control_service.NewControlService(client, ch, logger, cfg.RoomID)

	go func() {
		defer gRPCConnection.Close()
		controlService.StartControlStream(ctx)
	}()
	return nil
}

func runInjectorService(ctx context.Context, logger *slog.Logger, ch <-chan *controlpb.InputMessage) error {
	injectorService := injector.NewInjectorService(ch, logger)

	go func() {
		injectorService.StartReadingInput(ctx)
	}()
	return nil
}

// Injected at build time via -ldflags.
var (
	buildDate = "unknown"
	buildTime = "unknown"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("vmagent starting", "buildDate", buildDate, "buildTime", buildTime)

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("error loading env variables", slog.Any("err", err))
		os.Exit(1)
	}

	if err := spawnGStreamer(ctx, cfg, logger); err != nil {
		logger.Error("error starting GStreamer", "err", err)
		os.Exit(1)
	}

	// if err := runAVService(ctx, cfg, logger); err != nil {
	// 	logger.Error("error starting av service", slog.Any("err", err))
	// 	os.Exit(1)
	// }

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
