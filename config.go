package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppID   int
	AppHash string
}

func LoadConfig() (*Config, error) {
	// Try loading .env file, but don't fail if it doesn't exist (env vars might be set otherwise)
	_ = godotenv.Load()

	appIDStr := os.Getenv("APP_ID")
	if appIDStr == "" {
		return nil, fmt.Errorf("APP_ID not set in environment or .env")
	}

	appID, err := strconv.Atoi(appIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid APP_ID: %w", err)
	}

	appHash := os.Getenv("APP_HASH")
	if appHash == "" {
		return nil, fmt.Errorf("APP_HASH not set in environment or .env")
	}

	return &Config{
		AppID:   appID,
		AppHash: appHash,
	}, nil
}
