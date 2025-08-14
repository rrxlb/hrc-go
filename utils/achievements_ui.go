package utils

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	AchievementsPerPage = 5
)

// AchievementDisplayData holds achievement info for UI display
type AchievementDisplayData struct {
	Achievement *Achievement
	IsCompleted bool
	Progress    int
}

// GetCategorizedAchievements returns achievements organized by category
func GetCategorizedAchievements(userID int64) (map[AchievementCategory][]*AchievementDisplayData, error) {
	if AchievementMgr == nil {
		return nil, fmt.Errorf("achievement manager not initialized")
	}

	// Get all achievements
	allAchievements := AchievementMgr.GetAllAchievements()
	if allAchievements == nil {
		return nil, fmt.Errorf("no achievements available")
	}

	// Get user's earned achievements
	userAchievements, err := AchievementMgr.GetUserAchievements(userID)
	if err != nil {
		// Continue without user achievements (all will show as not completed)
		userAchievements = []*UserAchievement{}
	}

	// Create a map of earned achievement IDs
	earnedMap := make(map[int]bool)
	for _, ua := range userAchievements {
		earnedMap[ua.AchievementID] = true
	}

	// Get current user stats for progress calculation
	user, err := GetUser(userID)
	if err != nil {
		// Continue without user data
		user = &User{}
	}

	// Organize achievements by category
	categorized := make(map[AchievementCategory][]*AchievementDisplayData)

	for _, achievement := range allAchievements {
		// Skip hidden achievements that haven't been earned
		if achievement.Hidden && !earnedMap[achievement.ID] {
			continue
		}

		category := AchievementCategory(achievement.Category)

		// Calculate current progress
		currentProgress := 0
		if !earnedMap[achievement.ID] && AchievementMgr.checker != nil {
			// Calculate progress based on current stats
			currentProgress = calculateAchievementProgress(user, achievement)
		}

		displayData := &AchievementDisplayData{
			Achievement: achievement,
			IsCompleted: earnedMap[achievement.ID],
			Progress:    currentProgress,
		}

		categorized[category] = append(categorized[category], displayData)
	}

	// Sort achievements within each category by ID
	for category := range categorized {
		sort.Slice(categorized[category], func(i, j int) bool {
			return categorized[category][i].Achievement.ID < categorized[category][j].Achievement.ID
		})
	}

	return categorized, nil
}

// calculateAchievementProgress calculates current progress for an achievement
func calculateAchievementProgress(user *User, achievement *Achievement) int {
	switch RequirementType(achievement.RequirementType) {
	case RequirementChips:
		return int(user.Chips)
	case RequirementWins:
		return user.Wins
	case RequirementTotalXP:
		return int(user.TotalXP)
	case RequirementPrestige:
		return user.Prestige
	case RequirementGamesPlayed:
		return user.Wins + user.Losses
	case RequirementDailyBonuses:
		return user.DailyBonusesClaimed
	case RequirementVotes:
		return user.VotesCount
	default:
		return 0
	}
}

// CreateAchievementCategoryEmbed creates an embed for a specific category page
func CreateAchievementCategoryEmbed(category AchievementCategory, achievements []*AchievementDisplayData, page int, totalPages int, userID int64) *discordgo.MessageEmbed {
	startIdx := page * AchievementsPerPage
	endIdx := startIdx + AchievementsPerPage
	if endIdx > len(achievements) {
		endIdx = len(achievements)
	}

	pageAchievements := achievements[startIdx:endIdx]

	embed := CreateBrandedEmbed(
		fmt.Sprintf("🏆 %s Achievements", string(category)),
		fmt.Sprintf("Page %d/%d", page+1, totalPages),
		BotColor,
	)

	// Add achievements to embed
	for _, data := range pageAchievements {
		achievement := data.Achievement
		var statusIcon string
		var progressText string

		if data.IsCompleted {
			statusIcon = "✅"
			progressText = "**COMPLETED**"
		} else {
			statusIcon = "⭕"
			// Show progress based on requirement type
			maxValue := int(achievement.RequirementValue)
			currentProgress := data.Progress

			if maxValue > 0 {
				percentage := (float64(currentProgress) / float64(maxValue)) * 100
				if percentage > 100 {
					percentage = 100
				}
				progressText = fmt.Sprintf("Progress: %d/%d (%.1f%%)", currentProgress, maxValue, percentage)
			} else {
				progressText = "Not Started"
			}
		}

		rewardText := ""
		if achievement.ChipsReward > 0 || achievement.XPReward > 0 {
			var rewards []string
			if achievement.ChipsReward > 0 {
				rewards = append(rewards, fmt.Sprintf("💰 %s chips", FormatNumber(achievement.ChipsReward)))
			}
			if achievement.XPReward > 0 {
				rewards = append(rewards, fmt.Sprintf("⭐ %s XP", FormatNumber(achievement.XPReward)))
			}
			rewardText = fmt.Sprintf("\n**Rewards:** %s", strings.Join(rewards, ", "))
		}

		fieldValue := fmt.Sprintf("%s %s\n%s%s",
			achievement.Description,
			progressText,
			rewardText,
			"",
		)

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s %s %s", statusIcon, achievement.Icon, achievement.Name),
			Value:  fieldValue,
			Inline: false,
		})
	}

	// Add navigation info
	if totalPages > 1 {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Use the buttons below to navigate • %d achievements in this category", len(achievements)),
		}
	} else {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%d achievements in this category", len(achievements)),
		}
	}

	return embed
}

// CreateAchievementOverviewEmbed creates an overview embed showing all categories
func CreateAchievementOverviewEmbed(categorized map[AchievementCategory][]*AchievementDisplayData, userID int64) *discordgo.MessageEmbed {
	embed := CreateBrandedEmbed("🏆 Achievements Overview", "Select a category to view detailed achievements", BotColor)

	// Define category order
	categoryOrder := []AchievementCategory{
		CategoryFirstSteps,
		CategoryWins,
		CategoryWealth,
		CategoryExperience,
		CategoryPrestige,
		CategoryGaming,
		CategoryLoyalty,
		CategorySpecial,
	}

	// Calculate total achievements and completed
	totalAchievements := 0
	totalCompleted := 0

	for _, category := range categoryOrder {
		achievements, exists := categorized[category]
		if !exists || len(achievements) == 0 {
			continue
		}

		completed := 0
		for _, data := range achievements {
			if data.IsCompleted {
				completed++
			}
		}

		totalAchievements += len(achievements)
		totalCompleted += completed

		// Create progress bar
		percentage := 0.0
		if len(achievements) > 0 {
			percentage = (float64(completed) / float64(len(achievements))) * 100
		}

		progressBar := createAchievementProgressBar(percentage, 10)

		fieldValue := fmt.Sprintf("%s %d/%d completed (%.1f%%)\n%s",
			getCategoryIcon(category),
			completed,
			len(achievements),
			percentage,
			progressBar,
		)

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   string(category),
			Value:  fieldValue,
			Inline: true,
		})
	}

	// Add overall progress
	overallPercentage := 0.0
	if totalAchievements > 0 {
		overallPercentage = (float64(totalCompleted) / float64(totalAchievements)) * 100
	}

	embed.Description = fmt.Sprintf("**Overall Progress:** %d/%d completed (%.1f%%)\n%s\n\nSelect a category to view detailed achievements",
		totalCompleted,
		totalAchievements,
		overallPercentage,
		createAchievementProgressBar(overallPercentage, 15),
	)

	return embed
}

// getCategoryIcon returns an icon for each category
func getCategoryIcon(category AchievementCategory) string {
	switch category {
	case CategoryFirstSteps:
		return "👶"
	case CategoryWins:
		return "🏆"
	case CategoryWealth:
		return "💰"
	case CategoryExperience:
		return "⭐"
	case CategoryPrestige:
		return "👑"
	case CategoryGaming:
		return "🎮"
	case CategoryLoyalty:
		return "❤️"
	case CategorySpecial:
		return "🌟"
	default:
		return "🏅"
	}
}

// createAchievementProgressBar creates a visual progress bar for achievements
func createAchievementProgressBar(percentage float64, length int) string {
	if percentage > 100 {
		percentage = 100
	}
	if percentage < 0 {
		percentage = 0
	}

	filled := int((percentage / 100) * float64(length))
	empty := length - filled

	var bar strings.Builder
	bar.WriteString("`")
	for i := 0; i < filled; i++ {
		bar.WriteString("█")
	}
	for i := 0; i < empty; i++ {
		bar.WriteString("░")
	}
	bar.WriteString("`")

	return bar.String()
}

// CreateAchievementButtons creates navigation buttons for achievements
func CreateAchievementButtons(currentCategory AchievementCategory, currentPage int, totalPages int, userID int64, isOverview bool) []discordgo.MessageComponent {
	var components []discordgo.MessageComponent

	if isOverview {
		// Overview mode - show category selection buttons
		row1 := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategoryFirstSteps), userID),
					Label:    "First Steps",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "👶"},
				},
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategoryWins), userID),
					Label:    "Wins",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "🏆"},
				},
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategoryWealth), userID),
					Label:    "Wealth",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "💰"},
				},
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategoryExperience), userID),
					Label:    "Experience",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "⭐"},
				},
			},
		}

		row2 := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategoryPrestige), userID),
					Label:    "Prestige",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "👑"},
				},
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategoryGaming), userID),
					Label:    "Gaming",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "🎮"},
				},
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategoryLoyalty), userID),
					Label:    "Loyalty",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "❤️"},
				},
				discordgo.Button{
					CustomID: fmt.Sprintf("achievements_category_%s_%d", string(CategorySpecial), userID),
					Label:    "Special",
					Style:    discordgo.SecondaryButton,
					Emoji:    &discordgo.ComponentEmoji{Name: "🌟"},
				},
			},
		}

		components = append(components, row1, row2)
	} else {
		// Category view mode - show navigation buttons
		var buttons []discordgo.MessageComponent

		// Previous page button
		if currentPage > 0 {
			buttons = append(buttons, discordgo.Button{
				CustomID: fmt.Sprintf("achievements_page_%s_%d_%d", string(currentCategory), currentPage-1, userID),
				Label:    "Previous",
				Style:    discordgo.SecondaryButton,
				Emoji:    &discordgo.ComponentEmoji{Name: "◀️"},
			})
		}

		// Back to overview button
		buttons = append(buttons, discordgo.Button{
			CustomID: fmt.Sprintf("achievements_overview_%d", userID),
			Label:    "Back to Overview",
			Style:    discordgo.PrimaryButton,
			Emoji:    &discordgo.ComponentEmoji{Name: "🏠"},
		})

		// Next page button
		if currentPage < totalPages-1 {
			buttons = append(buttons, discordgo.Button{
				CustomID: fmt.Sprintf("achievements_page_%s_%d_%d", string(currentCategory), currentPage+1, userID),
				Label:    "Next",
				Style:    discordgo.SecondaryButton,
				Emoji:    &discordgo.ComponentEmoji{Name: "▶️"},
			})
		}

		if len(buttons) > 0 {
			components = append(components, discordgo.ActionsRow{Components: buttons})
		}
	}

	return components
}
