package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// HandData represents a player's hand data for blackjack embeds
type HandData struct {
	Hand     []string
	Score    int
	IsActive bool
}

// CreateBrandedEmbed creates a basic embed with bot branding (matches Python _create_branded_embed)
func CreateBrandedEmbed(title, description string, color int) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text:    "High Roller Club",
			IconURL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753043816/HRC-final_ymqwfy.png",
		},
	}

	return embed
}

// InsufficientChipsEmbed creates an embed for insufficient chips (matches Python insufficient_chips_embed)
func InsufficientChipsEmbed(requiredChips, currentBalance int64, betDescription string) *discordgo.MessageEmbed {
	embed := CreateBrandedEmbed(
		"Not Enough Chips",
		fmt.Sprintf("You don't have enough chips for %s.\n**Your balance:** %s %s\n**Required:** %s %s",
			betDescription,
			FormatChips(currentBalance), ChipsEmoji,
			FormatChips(requiredChips), ChipsEmoji),
		0xFF0000, // Red color
	)

	// Set thumbnail to match Python
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
		URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753046175/ER2_fwidxb.png",
	}

	// Add help text field like in Python
	embed.Fields = []*discordgo.MessageEmbedField{
		{
			Name:   "How to Get More Chips",
			Value:  "💰 **Get more chips:**\n• Use `/claimall` to claim daily, hourly, and weekly bonuses\n• Use `/vote` to vote for the bot on Top.gg for extra chips\n• Play lower stakes games to build your balance",
			Inline: false,
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

// BlackjackGameEmbed creates an embed for blackjack game state (matches Python create_game_embed)
func BlackjackGameEmbed(playerHands []HandData, dealerHand []string, dealerValue int, bet int64, gameOver bool, outcomeText string, newBalance int64, profit int64, xpGain int64, hasAces bool) *discordgo.MessageEmbed {
	// Set color based on game state and outcome (matches Python logic)
	var color int
	if gameOver {
		if strings.Contains(strings.ToLower(outcomeText), "win") || strings.Contains(strings.ToLower(outcomeText), "pays") {
			color = 0xFFD700 // Gold
		} else if strings.Contains(strings.ToLower(outcomeText), "lost") || strings.Contains(strings.ToLower(outcomeText), "bust") || strings.Contains(strings.ToLower(outcomeText), "dealer wins") {
			color = 0xFF0000 // Red
		} else { // Push
			color = 0xD3D3D3 // Light grey
		}
	} else {
		color = 0x1E5631 // Casino Green
	}

	embed := CreateBrandedEmbed("Blackjack", "", color)

	// Set thumbnail (matches Python)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
		URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753042166/3_vxurig.png",
	}

	// Player Hands
	for i, handData := range playerHands {
		handStr := strings.Join(handData.Hand, " ")
		score := handData.Score
		isActive := handData.IsActive

		var title string
		if len(playerHands) > 1 {
			if isActive {
				title = fmt.Sprintf("▶ Your Hand (%d/%d) - %d", i+1, len(playerHands), score)
			} else {
				title = fmt.Sprintf("Your Hand (%d/%d) - %d", i+1, len(playerHands), score)
			}
		} else {
			title = fmt.Sprintf("Your Hand - %d", score)
		}

		// Removed Ace clarification text per request

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   title,
			Value:  fmt.Sprintf("`%s`", handStr),
			Inline: false,
		})
	}

	// Dealer Hand
	dealerHandStr := strings.Join(dealerHand, " ")
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   fmt.Sprintf("Dealer's Hand - %d", dealerValue),
		Value:  fmt.Sprintf("`%s`", dealerHandStr),
		Inline: false,
	})

	// Preserve original footer
	originalFooterText := embed.Footer.Text
	originalFooterIcon := embed.Footer.IconURL

	if gameOver {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "Outcome",
			Value:  outcomeText,
			Inline: false,
		})

		if profit > 0 {
			winnningsText := fmt.Sprintf("%s %s", FormatChips(profit), ChipsEmoji)
			if strings.Contains(strings.ToLower(outcomeText), "blackjack") {
				winnningsText += " `(1.5x)`"
			} else if strings.Contains(strings.ToLower(outcomeText), "5-card charlie") {
				winnningsText += " `(1.75x)`"
			}
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "Winnings",
				Value:  winnningsText,
				Inline: true,
			})

			// XP Gained would be handled in Python with premium settings check - simplified here
			if xpGain > 0 {
				embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
					Name:   "XP Gained",
					Value:  fmt.Sprintf("%s XP", FormatChips(xpGain)),
					Inline: true,
				})
			}
		} else if profit < 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "Losses",
				Value:  fmt.Sprintf("%s %s", FormatChips(-profit), ChipsEmoji),
				Inline: true,
			})
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "New Balance",
			Value:  fmt.Sprintf("%s %s", FormatChips(newBalance), ChipsEmoji),
			Inline: false,
		})

		// Update footer for game over
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("%s | Game Over", originalFooterText),
			IconURL: originalFooterIcon,
		}
	} else {
		// Update footer with bet info
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("%s | Bet: %s chips", originalFooterText, FormatChips(bet)),
			IconURL: originalFooterIcon,
		}
	}

	return embed
}

// RouletteGameEmbed builds the roulette game embed for different states
// (Primary RouletteGameEmbed defined later in file)

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

// Note: Rank type is defined in constants.go
// (Removed duplicate Rank struct - using the one from constants.go)

// UserProfileEmbed creates an embed for user profile display
// showWinLoss controls whether wins/losses/win rate stats are shown (premium feature)
func UserProfileEmbed(user *User, discordUser *discordgo.User, showWinLoss bool) *discordgo.MessageEmbed {
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
	if showWinLoss {
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
		}...)
	}

	embed.Fields = append(embed.Fields, []*discordgo.MessageEmbedField{
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
	for level := len(Ranks) - 1; level >= 0; level-- {
		rank, exists := Ranks[level]
		if exists && totalXP >= int64(rank.XPRequired) {
			return rank
		}
	}
	return Ranks[0]
}

func getNextRank(totalXP int64) *Rank {
	currentLevel := -1
	for level := len(Ranks) - 1; level >= 0; level-- {
		rank, exists := Ranks[level]
		if exists && totalXP >= int64(rank.XPRequired) {
			currentLevel = level
			break
		}
	}

	nextLevel := currentLevel + 1
	if nextRank, exists := Ranks[nextLevel]; exists {
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

// RouletteGameEmbed builds the roulette game embed for different states
// state: betting | spinning | final
func RouletteGameEmbed(state string, bets map[string]int64, resultNumber int, resultColor string, profit int64, newBalance int64, xpGain int64) *discordgo.MessageEmbed {
	title := "🎡 Roulette"
	var description string
	color := BotColor
	switch state {
	case "betting":
		description = "Place your bets! Use the buttons below then press Spin."
		color = 0x1E8449
	case "spinning":
		description = "The wheel is spinning... ⏳"
		color = 0xF1C40F
	case "final":
		description = fmt.Sprintf("Result: **%d** (%s)", resultNumber, strings.Title(resultColor))
		if profit > 0 {
			description += fmt.Sprintf("\nYou won **%s** %s", FormatChips(profit), ChipsEmoji)
			color = 0x2ECC71
		} else if profit < 0 {
			description += fmt.Sprintf("\nYou lost **%s** %s", FormatChips(-profit), ChipsEmoji)
			color = 0xE74C3C
		} else {
			description += "\nIt's a push."
			color = 0x95A5A6
		}
	}
	embed := CreateBrandedEmbed(title, description, color)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1753042166/3_vxurig.png"}
	// Bets
	if len(bets) > 0 {
		var lines []string
		var total int64
		for k, v := range bets {
			name := strings.ReplaceAll(k, "_", " ")
			// Normalize casing: split words and title case each
			parts := strings.Split(name, " ")
			for i, p := range parts {
				if len(p) > 0 {
					parts[i] = strings.ToUpper(p[:1]) + p[1:]
				}
			}
			name = strings.Join(parts, " ")
			lines = append(lines, fmt.Sprintf("**%s**: %s", name, FormatChips(v)))
			total += v
		}
		lines = append(lines, fmt.Sprintf("Total: %s %s", FormatChips(total), ChipsEmoji))
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Current Bets", Value: strings.Join(lines, "\n"), Inline: false})
	} else if state == "betting" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Current Bets", Value: "No bets placed yet.", Inline: false})
	}
	if state == "final" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", FormatChips(newBalance), ChipsEmoji), Inline: true})
		if xpGain > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "XP Gained", Value: fmt.Sprintf("%s XP", FormatChips(xpGain)), Inline: true})
		}
		embed.Footer.Text += " | Game Over"
	}
	return embed
}

// ThreeCardPokerEmbed builds embeds for Three Card Poker states
// state: initial | final | forced_end
func ThreeCardPokerEmbed(state string, playerHand []string, dealerHand []string, playerEval string, dealerEval string, ante int64, pairPlus int64, playBet int64, outcome string, payoutLines []string, finalBalance int64, profit int64, xpGain int64) *discordgo.MessageEmbed {
	title := "🃏 Three Card Poker"
	color := BotColor
	description := ""
	if state == "initial" {
		description = "Decide to Play or Fold."
	}
	if state == "final" || state == "forced_end" {
		// Keep description blank; outcome will appear only in dedicated field below
		if profit > 0 {
			color = 0x2ECC71
		} else if profit < 0 {
			color = 0xE74C3C
		} else {
			color = 0x95A5A6
		}
	}
	embed := CreateBrandedEmbed(title, description, color)
	embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: "https://res.cloudinary.com/dfoeiotel/image/upload/v1754083114/TC2_ugnpqd.png"}
	// Card fields (side-by-side) with greyed evaluator using block quote style (> )
	playerVal := fmt.Sprintf("`%s`\n> %s", strings.Join(playerHand, " "), playerEval)
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Your Hand", Value: playerVal, Inline: true})
	if state == "initial" {
		masked := "?? ?? ??"
		dealerVal := fmt.Sprintf("`%s`\n> %s", masked, "??")
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Dealer's Hand", Value: dealerVal, Inline: true})
	} else {
		dealerVal := fmt.Sprintf("`%s`\n> %s", strings.Join(dealerHand, " "), dealerEval)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Dealer's Hand", Value: dealerVal, Inline: true})
	}

	// Bets only shown during initial decision phase
	if state == "initial" {
		betLines := []string{fmt.Sprintf("Ante: %s %s", FormatChips(ante), ChipsEmoji)}
		if pairPlus > 0 {
			betLines = append(betLines, fmt.Sprintf("Pair+: %s %s", FormatChips(pairPlus), ChipsEmoji))
		}
		if playBet > 0 {
			betLines = append(betLines, fmt.Sprintf("Play: %s %s", FormatChips(playBet), ChipsEmoji))
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Bets", Value: strings.Join(betLines, "\n"), Inline: false})
	}

	if state == "final" || state == "forced_end" {
		// Outcome field
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Outcome", Value: outcome, Inline: false})
		// Profit & Balance fields
		var profitFieldName string
		var profitVal string
		if profit > 0 {
			profitFieldName = "Profit"
			profitVal = fmt.Sprintf("+%s %s", FormatChips(profit), ChipsEmoji)
		} else if profit < 0 {
			profitFieldName = "Loss"
			profitVal = fmt.Sprintf("%s %s", FormatChips(abs(profit)), ChipsEmoji)
		} else { // push
			profitFieldName = "Result"
			profitVal = "Push"
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: profitFieldName, Value: profitVal, Inline: true})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "New Balance", Value: fmt.Sprintf("%s %s", FormatChips(finalBalance), ChipsEmoji), Inline: true})
		if len(payoutLines) > 0 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Payouts", Value: strings.Join(payoutLines, "\n"), Inline: false})
		}
		embed.Footer.Text += " | Game Over"
	}
	return embed
}
