package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var session *discordgo.Session

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Start HTTP server for Railway health checks
	go startHealthServer()

	// Get bot token
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN not set in environment variables")
	}

	// Create Discord session
	var err error
	session, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	// Set up intents
	session.Identify.Intents = discordgo.IntentsGuildMessages

	// Add event handlers
	session.AddHandler(onReady)
	session.AddHandler(onInteractionCreate)

	// Open Discord connection
	if err := session.Open(); err != nil {
		log.Fatalf("Failed to open Discord connection: %v", err)
	}
	defer session.Close()

	log.Println("Bot is now running. Press CTRL+C to exit.")

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop

	log.Println("Gracefully shutting down...")
}

func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Logged in as %s (ID: %s)", event.User.Username, event.User.ID)
	
	// Set bot presence
	if err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "Casino Games",
				Type: discordgo.ActivityTypeGame,
			},
		},
		Status: "online",
	}); err != nil {
		log.Printf("Failed to update status: %v", err)
	}

	log.Println("Bot is ready and online!")
}

func onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Basic interaction handler - will expand this later
	log.Printf("Received interaction from user: %s", i.Member.User.Username)
}

func startHealthServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		status := "offline"
		if session != nil && session.State != nil && session.State.User != nil {
			status = "online"
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Discord Bot Status: %s", status)))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"discord-bot"}`))
	})

	log.Printf("Health server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Printf("Health server error: %v", err)
	}
}