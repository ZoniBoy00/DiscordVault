package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DiscordToken  string
	ChannelID     string
	AllowedUsers  []string
	EncryptionKey []byte
}

func Load() (*Config, error) {
	cfg := &Config{}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN environment variable not set")
	}
	cfg.DiscordToken = token

	channelID := os.Getenv("DISCORD_CHANNEL_ID")
	if channelID == "" {
		return nil, fmt.Errorf("DISCORD_CHANNEL_ID environment variable not set")
	}
	cfg.ChannelID = channelID

	allowedUsersStr := os.Getenv("ALLOWED_USERS")
	if allowedUsersStr != "" {
		parts := strings.Split(allowedUsersStr, ",")
		for _, part := range parts {
			cfg.AllowedUsers = append(cfg.AllowedUsers, strings.TrimSpace(part))
		}
	}

	key := os.Getenv("ENCRYPTION_KEY")
	if len(key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be exactly 32 bytes (got %d)", len(key))
	}
	cfg.EncryptionKey = []byte(key)

	return cfg, nil
}
