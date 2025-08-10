package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"hrc-go/games/blackjack"
	"hrc-go/utils"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var (
	session *discordgo.Session
)

func init() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}
}

func main() {
	// Get bot token from environment
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
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	// Initialize database
	if err := utils.SetupDatabase(); err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}
	defer utils.CloseDatabase()

	// Initialize cache with 30 minute TTL
	utils.InitializeCache(30 * time.Minute)
	defer utils.CloseCache()

	// Add event handlers
	session.AddHandler(onReady)
	session.AddHandler(onInteractionCreate)

	// Start HTTP server for health checks (Railway requirement)
	go startHealthServer()

	// Register slash commands
	if err := registerCommands(); err != nil {
		log.Fatalf("Failed to register commands: %v", err)
	}

	// Open connection to Discord
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

	// Cleanup expired games periodically
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				utils.Games.CleanupExpiredGames(30 * time.Minute)
			case <-stop:
				return
			}
		}
	}()
}

func onReady(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Logged in as %s (ID: %s)", event.User.Username, event.User.ID)
	
	// Set bot presence
	if err := s.UpdateStatusComplex(discordgo.UpdateStatusData{
		Activities: []*discordgo.Activity{
			{
				Name: "/help for commands",
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
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		handleSlashCommand(s, i)
	case discordgo.InteractionMessageComponent:
		handleComponentInteraction(s, i)
	}
}

func handleSlashCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	commandName := i.ApplicationCommandData().Name
	
	log.Printf("Received slash command: %s from user: %s", commandName, i.Member.User.Username)
	
	switch commandName {
	case "blackjack":
		blackjack.HandleBlackjackCommand(s, i)
	case "help":
		handleHelpCommand(s, i)
	case "profile":
		handleProfileCommand(s, i)
	case "daily":
		handleDailyCommand(s, i)
	default:
		respondWithError(s, i, "Unknown command")
	}
}

func handleComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	customID := i.MessageComponentData().CustomID
	
	log.Printf("Received component interaction: %s from user: %s", customID, i.Member.User.Username)
	
	// Route component interactions to appropriate game handlers
	switch {
	case customID == "blackjack_hit" || customID == "blackjack_stand" || 
		 customID == "blackjack_double" || customID == "blackjack_split":
		blackjack.HandleBlackjackInteraction(s, i)
	default:
		respondWithError(s, i, "Unknown interaction")
	}
}

func registerCommands() error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "blackjack",
			Description: "Play a game of blackjack",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "bet",
					Description: "Your bet amount (e.g., 100, 1k, 50%, all)",
					Required:    true,
				},
			},
		},
		{
			Name:        "help",
			Description: "Show available commands and game information",
		},
		{
			Name:        "profile",
			Description: "View your casino profile and statistics",
		},
		{
			Name:        "daily",
			Description: "Claim your daily chip bonus",
		},
	}

	for _, command := range commands {
		_, err := session.ApplicationCommandCreate(session.State.User.ID, "", command)
		if err != nil {
			return err
		}
		log.Printf("Registered slash command: %s", command.Name)
	}

	return nil
}

func handleHelpCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := utils.CreateBrandedEmbed(
		"🎰 High Rollers Casino - Commands",
		"Welcome to the High Rollers Casino! Here are the available commands:",
		utils.BotColor,
	)

	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:   "🃏 Games",
			Value:  "`/blackjack <bet>` - Play blackjack\n`/baccarat <bet>` - Coming soon!\n`/roulette <bet>` - Coming soon!",
			Inline: false,
		},
		{
			Name:   "💰 Economy",
			Value:  "`/daily` - Claim daily chips\n`/profile` - View your statistics",
			Inline: false,
		},
		{
			Name:   "ℹ️ Information",
			Value:  "`/help` - Show this help message",
			Inline: false,
		},
		{
			Name:   "💡 Betting Tips",
			Value:  "• Use numbers: `100`, `1000`\n• Use shortcuts: `1k` (1,000), `1m` (1,000,000)\n• Use percentages: `50%`, `25%`\n• Use keywords: `all`, `half`, `max`",
			Inline: false,
		},
	}

	embed.Footer = &discordgo.MessageEmbedFooter{
		Text: "Join our community: " + utils.HighRollersClubLink,
	}

	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	}

	if err := s.InteractionRespond(i.Interaction, response); err != nil {
		log.Printf("Failed to respond to help command: %v", err)
	}
}

func handleProfileCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, err := parseUserID(i.Member.User.ID)
	if err != nil {
		respondWithError(s, i, "Failed to parse user ID")
		return
	}

	user, err := utils.GetCachedUser(userID)
	if err != nil {
		respondWithError(s, i, "Failed to get user profile")
		return
	}

	embed := utils.UserProfileEmbed(user, i.Member.User)

	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	}

	if err := s.InteractionRespond(i.Interaction, response); err != nil {
		log.Printf("Failed to respond to profile command: %v", err)
	}
}

func handleDailyCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, err := parseUserID(i.Member.User.ID)
	if err != nil {
		respondWithError(s, i, "Failed to parse user ID")
		return
	}

	user, err := utils.GetCachedUser(userID)
	if err != nil {
		respondWithError(s, i, "Failed to get user data")
		return
	}

	// Check if user can claim daily
	if !canClaimDaily(user) {
		timeUntilNext := getTimeUntilNextDaily(user)
		embed := utils.CreateBrandedEmbed(
			"⏰ Daily Reward",
			"You've already claimed your daily reward! Come back in " + formatDuration(timeUntilNext),
			0xF39C12, // Orange
		)
		
		response := &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{embed},
			},
		}
		
		s.InteractionRespond(i.Interaction, response)
		return
	}

	// Award daily chips
	now := time.Now()
	updates := utils.UserUpdateData{
		ChipsIncrement: utils.DailyReward,
		LastDaily:      &now,
	}

	updatedUser, err := utils.UpdateCachedUser(userID, updates)
	if err != nil {
		respondWithError(s, i, "Failed to claim daily reward")
		return
	}

	embed := utils.CreateBrandedEmbed(
		"💰 Daily Reward Claimed!",
		"You've received your daily chip bonus!",
		0x2ECC71, // Green
	)

	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:   "Reward",
			Value:  fmt.Sprintf("%s %s", utils.FormatChips(utils.DailyReward), utils.ChipsEmoji),
			Inline: true,
		},
		{
			Name:   "New Balance",
			Value:  fmt.Sprintf("%s %s", utils.FormatChips(updatedUser.Chips), utils.ChipsEmoji),
			Inline: true,
		},
		{
			Name:   "Next Daily",
			Value:  "Available in 24 hours",
			Inline: true,
		},
	}

	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	}

	if err := s.InteractionRespond(i.Interaction, response); err != nil {
		log.Printf("Failed to respond to daily command: %v", err)
	}
}

func respondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	embed := utils.CreateBrandedEmbed(
		"❌ Error",
		message,
		0xFF0000, // Red
	)

	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}

	if err := s.InteractionRespond(i.Interaction, response); err != nil {
		log.Printf("Failed to send error response: %v", err)
	}
}

// Helper functions
func parseUserID(discordID string) (int64, error) {
	return strconv.ParseInt(discordID, 10, 64)
}

func canClaimDaily(user *utils.User) bool {
	if user.LastDaily == nil {
		return true
	}
	return time.Since(*user.LastDaily) >= 24*time.Hour
}

func getTimeUntilNextDaily(user *utils.User) time.Duration {
	if user.LastDaily == nil {
		return 0
	}
	nextDaily := user.LastDaily.Add(24 * time.Hour)
	return time.Until(nextDaily)
}

func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// startHealthServer starts a simple HTTP server for Railway health checks
func startHealthServer() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("High Rollers Casino Bot - Online"))
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