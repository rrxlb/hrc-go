package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hrc-go/cogs"
	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
)

var session *discordgo.Session
var botStatus = "starting"

func main() {
	// Start HTTP server for Railway health checks
	go startHealthServer()

	// Initialize database
	if err := utils.SetupDatabase(); err != nil {
		log.Printf("Database setup failed: %v", err)
		log.Println("Bot will continue without database features")
	} else {
		log.Println("Database connected successfully")
		// Ensure database cleanup on shutdown
		defer utils.CloseDatabase()
	}

	// Get bot token from environment
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Println("BOT_TOKEN not set - Discord bot will not connect")
		botStatus = "no_token"
		// Keep HTTP server running
		select {}
	}

	// Create Discord session
	var err error
	session, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Printf("Failed to create Discord session: %v", err)
		botStatus = "error"
		select {}
	}

	// Set up basic intents
	session.Identify.Intents = discordgo.IntentsGuildMessages

	// Add event handlers
	session.AddHandler(onReady)
	session.AddHandler(onInteractionCreate)
	session.AddHandler(onButtonInteraction)

	// Open Discord connection
	if err := session.Open(); err != nil {
		log.Printf("Failed to open Discord connection: %v", err)
		botStatus = "connection_failed"
		select {}
	}
	defer session.Close()

	log.Println("Bot is now running. Press CTRL+C to exit.")
	botStatus = "running"

	// Wait for interrupt signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop

	log.Println("Gracefully shutting down...")
	botStatus = "shutting_down"
}

func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("✅ Discord Bot logged in as %s (ID: %s)", event.User.Username, event.User.ID)
	botStatus = "online"
	
	// Set bot presence
	if err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "Casino Games - Go Version",
				Type: discordgo.ActivityTypeGame,
			},
		},
		Status: "online",
	}); err != nil {
		log.Printf("Failed to update status: %v", err)
	}
	
	// Register slash commands
	if err := registerSlashCommands(s); err != nil {
		log.Printf("Failed to register slash commands: %v", err)
	}
}

func registerSlashCommands(s *discordgo.Session) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "ping",
			Description: "Check bot latency and status",
		},
		{
			Name:        "info",
			Description: "Get information about the bot",
		},
		{
			Name:        "profile",
			Description: "View your casino profile and stats",
		},
		{
			Name:        "balance",
			Description: "Check your current chip balance",
		},
		cogs.RegisterBlackjackCommands(),
	}

	for _, command := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", command)
		if err != nil {
			return fmt.Errorf("failed to create command %s: %w", command.Name, err)
		}
	}
	
	log.Printf("Successfully registered %d slash commands", len(commands))
	return nil
}

func onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	
	switch i.ApplicationCommandData().Name {
	case "ping":
		handlePingCommand(s, i)
	case "info":
		handleInfoCommand(s, i)
	case "profile":
		handleProfileCommand(s, i)
	case "balance":
		handleBalanceCommand(s, i)
	case "blackjack":
		cogs.HandleBlackjackCommand(s, i)
	}
}

func onButtonInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}
	
	customID := i.MessageComponentData().CustomID
	
	// Route button interactions to appropriate handlers
	if strings.HasPrefix(customID, "blackjack_") {
		cogs.HandleBlackjackInteraction(s, i)
	}
}

func handlePingCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	startTime := time.Now()
	
	// Calculate approximate latency
	latency := s.HeartbeatLatency()
	
	embed := &discordgo.MessageEmbed{
		Title: "🏓 Pong!",
		Color: 0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Latency",
				Value:  fmt.Sprintf("%dms", latency.Milliseconds()),
				Inline: true,
			},
			{
				Name:   "Status",
				Value:  "✅ Online",
				Inline: true,
			},
			{
				Name:   "Response Time",
				Value:  fmt.Sprintf("%dms", time.Since(startTime).Milliseconds()),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func handleInfoCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🎰 High Rollers Club Bot",
		Description: "A Discord casino bot built with Go",
		Color:       0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Version",
				Value:  "2.0.0 (Go Rewrite)",
				Inline: true,
			},
			{
				Name:   "Language",
				Value:  "Go",
				Inline: true,
			},
			{
				Name:   "Framework",
				Value:  "DiscordGo",
				Inline: true,
			},
			{
				Name:   "Features",
				Value:  "✅ User System & Profiles\n✅ Blackjack Game\n🔜 More Casino Games & Achievements",
				Inline: false,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "High Rollers Club",
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func handleProfileCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	username := i.Member.User.Username

	// Get or create user
	user, err := utils.GetUser(userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Error accessing user data. Database may be unavailable.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	// Get rank information
	rankName, rankIcon, rankColor, nextRankXP := utils.GetRank(user.TotalXP)
	
	// Calculate win rate
	totalGames := user.Wins + user.Losses
	winRate := 0.0
	if totalGames > 0 {
		winRate = float64(user.Wins) / float64(totalGames) * 100
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("🎰 %s's Casino Profile", username),
		Color: rankColor,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "💰 Chips",
				Value:  fmt.Sprintf("%d <:chips:1396988413151940629>", user.Chips),
				Inline: true,
			},
			{
				Name:   "🏆 Rank",
				Value:  fmt.Sprintf("%s %s", rankIcon, rankName),
				Inline: true,
			},
			{
				Name:   "⭐ Total XP",
				Value:  fmt.Sprintf("%d", user.TotalXP),
				Inline: true,
			},
			{
				Name:   "🎯 Games Won",
				Value:  fmt.Sprintf("%d", user.Wins),
				Inline: true,
			},
			{
				Name:   "💔 Games Lost",
				Value:  fmt.Sprintf("%d", user.Losses),
				Inline: true,
			},
			{
				Name:   "📊 Win Rate",
				Value:  fmt.Sprintf("%.1f%%", winRate),
				Inline: true,
			},
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Casino Profile",
		},
	}

	// Add prestige if > 0
	if user.Prestige > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🌟 Prestige",
			Value:  fmt.Sprintf("Level %d", user.Prestige),
			Inline: true,
		})
	}

	// Add next rank progress if not max rank
	if nextRankXP > user.TotalXP {
		xpNeeded := nextRankXP - user.TotalXP
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🚀 Next Rank",
			Value:  fmt.Sprintf("%d XP needed", xpNeeded),
			Inline: true,
		})
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func handleBalanceCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, _ := strconv.ParseInt(i.Member.User.ID, 10, 64)
	username := i.Member.User.Username

	// Get or create user
	user, err := utils.GetUser(userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "❌ Error accessing user data. Database may be unavailable.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("💰 %s's Balance", username),
		Color: 0x5865F2,
		Description: fmt.Sprintf("You currently have **%d** <:chips:1396988413151940629> chips", user.Chips),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func startHealthServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Discord Bot Status: %s", botStatus)))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := fmt.Sprintf(`{"status":"healthy","service":"discord-bot","bot_status":"%s"}`, botStatus)
		w.Write([]byte(response))
	})

	log.Printf("Health server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Printf("Health server error: %v", err)
	}
}