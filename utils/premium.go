package utils

import (
	"strconv"

	"github.com/bwmarrin/discordgo"
)

// Premium feature keys (aligned with Python)
const (
	PremiumFeatureXPDisplay  = "xp_display"
	PremiumFeatureWinsLosses = "wins_losses_display"
)

// HasPremiumRole checks if the member has the premium role
func HasPremiumRole(member *discordgo.Member) bool {
	for _, r := range member.Roles {
		if r == strconv.FormatInt(PremiumRoleID, 10) { // role IDs in discordgo are strings
			return true
		}
	}
	return false
}

// GetPremiumSetting returns a bool flag from user's PremiumSettings JSONB
func GetPremiumSetting(user *User, key string) bool {
	if user == nil || user.PremiumSettings == nil {
		return false
	}
	if v, ok := user.PremiumSettings[key]; ok {
		if b, ok2 := v.(bool); ok2 {
			return b
		}
		if s, ok2 := v.(string); ok2 {
			return s == "true" || s == "1"
		}
		if f, ok2 := v.(float64); ok2 {
			return f != 0
		}
	}
	return false
}

// ShouldShowXPGained returns whether XP should be shown in game result embeds
// Rule: show only if user has premium role and xp_display=true
func ShouldShowXPGained(member *discordgo.Member, user *User) bool {
	if !HasPremiumRole(member) {
		return false
	}
	return GetPremiumSetting(user, PremiumFeatureXPDisplay)
}
