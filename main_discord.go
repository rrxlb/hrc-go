package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	godotenv.Load()

	// Get bot token
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN not set in environment variables")
	}

	// Create Discord session
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Failed to create Discord session: %v", err)
	}

	// Set up basic event handler
	session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as %s", r.User.Username)
	})

	// Start HTTP server for Railway health checks
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Bot is running"))
		})

		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
		})

		log.Printf("HTTP server starting on port %s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Open Discord connection
	if err := session.Open(); err != nil {
		log.Fatalf("Failed to open Discord connection: %v", err)
	}
	defer session.Close()

	fmt.Println("Bot is running. Press Ctrl+C to exit.")
	select {} // Block forever
}