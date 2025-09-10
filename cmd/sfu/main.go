package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/AvneetSidhu/Flicker/internal/sfu"
	"github.com/joho/godotenv"
)

func loadConfig() (*sfu.Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env, %v", err)
	}
	return &sfu.Config{
		Port:        os.Getenv("PORT"),
	}, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := loadConfig() 
	if err != nil {
		slog.Error("error loading env variables", "err", err)
		os.Exit(1)
	}
	fmt.Println(ctx, cfg)

}