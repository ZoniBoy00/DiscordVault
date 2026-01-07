package main

import (
	"discordvault/internal/bot"
	"discordvault/internal/config"
	"discordvault/internal/database"
	"discordvault/internal/server"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not found, using system environment variables.")
	}

	// Load Configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[CRITICAL] Config load failed: %v", err)
	}

	// Initialize Database
	db, err := database.Initialize("./metadata.db")
	if err != nil {
		log.Fatalf("[CRITICAL] Database init failed: %v", err)
	}
	defer db.Conn.Close()

	// Initialize Bot
	vaultBot, err := bot.New(cfg, db)
	if err != nil {
		log.Fatalf("[CRITICAL] Bot init failed: %v", err)
	}

	// Initialize Server
	srv := server.New(cfg, db, vaultBot)

	// Start Server in background
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("[CRITICAL] Server failed: %v", err)
		}
	}()

	// Start Bot
	if err := vaultBot.Start(); err != nil {
		log.Fatalf("[CRITICAL] Bot failed: %v", err)
	}

	log.Println("Discord Vault is fully operational.")

	// Wait for termination signal
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc

	log.Println("Shutting down gracefully...")
	vaultBot.Session.Close()
}
