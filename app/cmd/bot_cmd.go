package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/pocketbase/pocketbase"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/bot"
)

func Run() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Check required environment variables
	if os.Getenv("BOT_TOKEN") == "" {
		log.Fatal("BOT_TOKEN environment variable is required")
	}

	// Initialize PocketBase
	pb := pocketbase.New()

	// Initialize and start the bot
	telegramBot, err := bot.NewBot(pb)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal...")
		telegramBot.Stop()
	}()

	// Start the bot
	log.Println("🚀 Starting Telegram bot...")
	if err := telegramBot.Start(); err != nil {
		log.Fatalf("Bot error: %v", err)
	}
}
