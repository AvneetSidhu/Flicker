package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/AvneetSidhu/Flicker/internal/vmagent"
	"github.com/joho/godotenv"
)


func loadConfig() (*vmagent.Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env file, %v", err)
	}

	portStr := os.Getenv("PORT")
	if portStr == "" {
		return nil, fmt.Errorf("missing PORT")
	}

	sfuAddr := os.Getenv("SFU_ADDR")
	if sfuAddr == "" {
		return nil, fmt.Errorf("missing SFU_ADDR")
	}

	return &vmagent.Config{
		SFUAddr: sfuAddr,
		Port: portStr,
	}, nil 
}

func runAVService(cfg *vmagent.Config) error {
	avService := vmagent.NewAVService(cfg)
	err := avService.ConnectToSFU()
	if err != nil {
		return err
	}

	conn, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%s", cfg.Port))
	if err != nil {
		return fmt.Errorf("error while listening to ingestion port at %s: %w",cfg.Port, err)
	}
	conn.Close()

	go avService.StartIngest()
	return nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		slog.Error("Error loading env variables", "err", err)
		os.Exit(1)
	}

	if err := runAVService(cfg); err != nil {
		slog.Error("error starting av service", "err", err)
	}
	
	select {}
}
