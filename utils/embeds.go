package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// CreateBrandedEmbed creates a basic embed with bot branding
func CreateBrandedEmbed(title, description string, color int) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text:    "High Rollers Casino • discord.gg/RK4K8tDsHB",
			IconURL: "https://cdn.discordapp.com/attachments/1396632945481838652/1396988413151940629/chips.png",
		},
	}
	
	return embed
}

// InsufficientChipsEmbed creates an embed for insufficient chips
func InsufficientChipsEmbed(requiredChips, currentBalance int64, betDescription string) *discordgo.MessageEmbed {
	embed := CreateBrandedEmbed(
		"💰 Insufficient Chips",
		fmt.Sprintf("You need **%s** %s to place %s, but you only have **%s** %s.\n\n💡 **Tip**: Use `/daily` to claim your daily chips or try a smaller bet!",
			FormatChips(requiredChips), ChipsEmoji,
			betDescription,
			FormatChips(currentBalance), ChipsEmoji),
		0xE74C3C, // Red color
	)
	
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:   "Your Balance",
			Value:  fmt.Sprintf("%s %s", FormatChips(currentBalance), ChipsEmoji),
			Inline: true,
		},
		{
			Name:   "Required",
			Value:  fmt.Sprintf("%s %s", FormatChips(requiredChips), ChipsEmoji),
			Inline: true,
		},
		{
			Name:   "Shortfall",
			Value:  fmt.Sprintf("%s %s", FormatChips(requiredChips-currentBalance), ChipsEmoji),
			Inline: true,
		},
	}
	
	return embed
}

// GameTimeoutEmbed creates an embed for game timeout
func GameTimeoutEmbed(betAmount int64) *discordgo.MessageEmbed {
	return CreateBrandedEmbed(
		"⏰ Game Timeout",
		fmt.Sprintf(GameTimeoutMessage, betAmount),
		0xF39C12, // Orange color
	)
}

// GameCleanupEmbed creates an embed for game cleanup
func GameCleanupEmbed(betAmount int64) *discordgo.MessageEmbed {
	return CreateBrandedEmbed(
		"🧹 Game Cleanup",
		fmt.Sprintf(GameCleanupMessage, betAmount),
		0xF39C12, // Orange color
	)
}

// CreateTimeoutEmbed creates a generic timeout embed
func CreateTimeoutEmbed() *discordgo.MessageEmbed {
	return CreateBrandedEmbed(
		"⏰ Timeout",
		TimeoutMessage,
		0xF39C12, // Orange color
	)
}

// BlackjackGameEmbed creates an embed for blackjack game state
func BlackjackGameEmbed(playerHands [][]Card, dealerHand []Card, playerScores []int, dealerScore int, currentHandIndex int, gameOver bool) *discordgo.MessageEmbed {
	embed := CreateBrandedEmbed("🃏 Blackjack", "", BotColor)
	
	// Dealer section
	dealerCardsStr := ""
	if gameOver {
		for _, card := range dealerHand {
			dealerCardsStr += card.String() + " "
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("🎯 Dealer (%d)", dealerScore),
			Value:  strings.TrimSpace(dealerCardsStr),
			Inline: false,
		})
	} else {
		// Show first card, hide second
		if len(dealerHand) > 0 {
			dealerCardsStr += dealerHand[0].String() + " 🂠"
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "🎯 Dealer",
			Value:  strings.TrimSpace(dealerCardsStr),
			Inline: false,
		})
	}
	
	// Player hands
	for i, hand := range playerHands {
		handCardsStr := ""
		for _, card := range hand {
			handCardsStr += card.String() + " "
		}
		
		handName := "👤 Your Hand"
		if len(playerHands) > 1 {
			handName = fmt.Sprintf("👤 Hand %d", i+1)
		}
		if i == currentHandIndex && !gameOver {
			handName += " ⭐"
		}
		
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s (%d)", handName, playerScores[i]),
			Value:  strings.TrimSpace(handCardsStr),
			Inline: false,
		})
	}
	
	return embed
}

// GameResultEmbed creates an embed showing game results
func GameResultEmbed(gameType string, bet, profit int64, userBefore, userAfter *User) *discordgo.MessageEmbed {
	var title, description string
	var color int
	
	if profit > 0 {
		title = "🎉 You Won!"
		description = fmt.Sprintf("Congratulations! You won **%s** %s", FormatChips(profit), ChipsEmoji)
		color = 0x2ECC71 // Green
	} else if profit < 0 {
		title = "😔 You Lost"
		description = fmt.Sprintf("Better luck next time! You lost **%s** %s", FormatChips(-profit), ChipsEmoji)
		color = 0xE74C3C // Red
	} else {
		title = "🤝 Push"
		description = "It's a tie! Your bet has been returned."
		color = 0xF39C12 // Orange
	}
	
	embed := CreateBrandedEmbed(title, description, color)
	
	// Add game details
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:   "Game",
			Value:  strings.Title(gameType),
			Inline: true,
		},
		{
			Name:   "Bet Amount",
			Value:  fmt.Sprintf("%s %s", FormatChips(bet), ChipsEmoji),
			Inline: true,
		},
		{
			Name:   "Result",
			Value:  fmt.Sprintf("%s%s %s", getProfitPrefix(profit), FormatChips(abs(profit)), ChipsEmoji),
			Inline: true,
		},
	}
	
	// Add balance information
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "Balance",
		Value:  fmt.Sprintf("%s %s → %s %s", FormatChips(userBefore.Chips), ChipsEmoji, FormatChips(userAfter.Chips), ChipsEmoji),
		Inline: false,
	})
	
	// Add XP information if gained
	if userAfter.TotalXP > userBefore.TotalXP {
		xpGained := userAfter.TotalXP - userBefore.TotalXP
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "XP Gained",
			Value:  fmt.Sprintf("+%d XP", xpGained),
			Inline: true,
		})
		
		// Check for rank up
		beforeRank := getUserRank(userBefore.TotalXP)
		afterRank := getUserRank(userAfter.TotalXP)
		if beforeRank != afterRank {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "🎊 Rank Up!",
				Value:  fmt.Sprintf("%s %s → %s %s", beforeRank.Icon, beforeRank.Name, afterRank.Icon, afterRank.Name),
				Inline: true,
			})
		}
	}
	
	return embed
}

// Rank represents a user rank for embeds
type Rank struct {
	Name       string
	Icon       string
	XPRequired int
	Color      int
}

// UserProfileEmbed creates an embed for user profile display
func UserProfileEmbed(user *User, discordUser *discordgo.User) *discordgo.MessageEmbed {
	rank := getUserRank(user.TotalXP)
	nextRank := getNextRank(user.TotalXP)
	
	embed := CreateBrandedEmbed("👤 Player Profile", "", BotColor)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
		URL: discordUser.AvatarURL(""),
	}
	
	// User info
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:   "Player",
			Value:  discordUser.Mention(),
			Inline: true,
		},
		{
			Name:   "Balance",
			Value:  fmt.Sprintf("%s %s", FormatChips(user.Chips), ChipsEmoji),
			Inline: true,
		},
		{
			Name:   "Rank",
			Value:  fmt.Sprintf("%s %s", rank.Icon, rank.Name),
			Inline: true,
		},
	}
	
	// Stats
	totalGames := user.Wins + user.Losses
	winRate := 0.0
	if totalGames > 0 {
		winRate = (float64(user.Wins) / float64(totalGames)) * 100
	}
	
	embed.Fields = append(embed.Fields, []*discordgo.MessageEmbedField{
		{
			Name:   "Games Won",
			Value:  strconv.Itoa(user.Wins),
			Inline: true,
		},
		{
			Name:   "Games Lost",
			Value:  strconv.Itoa(user.Losses),
			Inline: true,
		},
		{
			Name:   "Win Rate",
			Value:  fmt.Sprintf("%.1f%%", winRate),
			Inline: true,
		},
		{
			Name:   "Total XP",
			Value:  FormatNumber(user.TotalXP),
			Inline: true,
		},
		{
			Name:   "Net Profit",
			Value:  fmt.Sprintf("%s%s %s", getProfitPrefix(user.Chips-StartingChips), FormatChips(abs(user.Chips-StartingChips)), ChipsEmoji),
			Inline: true,
		},
	}...)
	
	// Next rank progress
	if nextRank != nil {
		xpNeeded := int64(nextRank.XPRequired) - user.TotalXP
		progressBar := createProgressBar(user.TotalXP, int64(rank.XPRequired), int64(nextRank.XPRequired), 10)
		
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("Progress to %s %s", nextRank.Icon, nextRank.Name),
			Value:  fmt.Sprintf("%s\n**%s** / **%s** XP (**%s** needed)", progressBar, FormatNumber(user.TotalXP), FormatNumber(int64(nextRank.XPRequired)), FormatNumber(xpNeeded)),
			Inline: false,
		})
	}
	
	// Account age
	accountAge := time.Since(user.CreatedAt)
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "Account Age",
		Value:  formatDuration(accountAge),
		Inline: true,
	})
	
	return embed
}

// Helper functions
func FormatChips(amount int64) string {
	return FormatNumber(amount)
}

func FormatNumber(num int64) string {
	str := strconv.FormatInt(num, 10)
	if len(str) <= 3 {
		return str
	}
	
	// Add commas for thousands
	var result strings.Builder
	for i, r := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(r)
	}
	
	return result.String()
}

func getProfitPrefix(profit int64) string {
	if profit > 0 {
		return "+"
	}
	return ""
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func getUserRank(totalXP int64) Rank {
	ranks := map[int]Rank{
		0: {"Novice", "🥉", 0, 0xcd7f32},
		1: {"Apprentice", "🥈", 10000, 0xc0c0c0},
		2: {"Gambler", "🥇", 40000, 0xffd700},
		3: {"High Roller", "💰", 125000, 0x22a7f0},
		4: {"Card Shark", "🦈", 350000, 0x1f3a93},
		5: {"Pit Boss", "👑", 650000, 0x9b59b6},
		6: {"Legend", "🌟", 2000000, 0xf1c40f},
		7: {"Casino Elite", "💎", 4500000, 0x1abc9c},
	}
	
	for level := len(ranks) - 1; level >= 0; level-- {
		rank, exists := ranks[level]
		if exists && totalXP >= int64(rank.XPRequired) {
			return rank
		}
	}
	return ranks[0]
}

func getNextRank(totalXP int64) *Rank {
	ranks := map[int]Rank{
		0: {"Novice", "🥉", 0, 0xcd7f32},
		1: {"Apprentice", "🥈", 10000, 0xc0c0c0},
		2: {"Gambler", "🥇", 40000, 0xffd700},
		3: {"High Roller", "💰", 125000, 0x22a7f0},
		4: {"Card Shark", "🦈", 350000, 0x1f3a93},
		5: {"Pit Boss", "👑", 650000, 0x9b59b6},
		6: {"Legend", "🌟", 2000000, 0xf1c40f},
		7: {"Casino Elite", "💎", 4500000, 0x1abc9c},
	}
	
	currentLevel := -1
	for level := len(ranks) - 1; level >= 0; level-- {
		rank, exists := ranks[level]
		if exists && totalXP >= int64(rank.XPRequired) {
			currentLevel = level
			break
		}
	}
	
	nextLevel := currentLevel + 1
	if nextRank, exists := ranks[nextLevel]; exists {
		return &nextRank
	}
	
	return nil
}

func createProgressBar(current, min, max int64, length int) string {
	if max <= min {
		return strings.Repeat("█", length)
	}
	
	progress := float64(current-min) / float64(max-min)
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	
	filled := int(progress * float64(length))
	empty := length - filled
	
	return strings.Repeat("█", filled) + strings.Repeat("░", empty)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	
	if days > 0 {
		return fmt.Sprintf("%d days, %d hours", days, hours)
	}
	return fmt.Sprintf("%d hours", hours)
}