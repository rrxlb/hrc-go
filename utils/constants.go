package utils

// General Configuration
const (
	GuildID             int64 = 1396567190102347776
	HighRollersClubLink       = "https://discord.gg/RK4K8tDsHB"
	DeveloperRoleID     int64 = 1399860810574073978
	BotColor                  = 0x5865F2
	PremiumRoleID       int64 = 1333193975947071488
)

// Economy & XP
const (
	StartingChips = 1000
	DailyReward   = 500
	XPPerProfit   = 2
)

// Ranks with XP requirements and colors
type Rank struct {
	Name       string
	Icon       string
	XPRequired int
	Color      int
}

var Ranks = map[int]Rank{
	0: {"Novice", "🥉", 0, 0xcd7f32},
	1: {"Apprentice", "🥈", 10000, 0xc0c0c0},
	2: {"Gambler", "🥇", 40000, 0xffd700},
	3: {"High Roller", "💰", 125000, 0x22a7f0},
	4: {"Card Shark", "🦈", 350000, 0x1f3a93},
	5: {"Pit Boss", "👑", 650000, 0x9b59b6},
	6: {"Legend", "🌟", 2000000, 0xf1c40f},
	7: {"Casino Elite", "💎", 4500000, 0x1abc9c},
}

// Card System
var (
	CardSuits = []string{"♠️", "♥️", "♦️", "♣️"}
	CardRanks = map[string]int{
		"2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "10": 10,
		"J": 10, "Q": 10, "K": 10, "A": 11,
	}
)

// Blackjack Game Constants
const (
	DeckCount             = 6    // Standard shoe size
	ShuffleThreshold      = 0.25 // Shuffle when 25% of the shoe remains
	DealerStandValue      = 17
	BlackjackPayout       = 1.5
	FiveCardCharliePayout = 1.75
)

// Baccarat Game Constants
const (
	BaccaratPayout           = 1.0
	BaccaratTiePayout        = 8.0
	BaccaratBankerCommission = 0.05
)

// Poker Game Constants
var (
	PokerHandRankings = map[string]int{
		"high_card":       1,
		"pair":            2,
		"two_pair":        3,
		"three_of_a_kind": 4,
		"straight":        5,
		"flush":           6,
		"full_house":      7,
		"four_of_a_kind":  8,
		"straight_flush":  9,
		"royal_flush":     10,
	}

	PokerHandNames = map[int]string{
		1:  "High Card",
		2:  "Pair",
		3:  "Two Pair",
		4:  "Three of a Kind",
		5:  "Straight",
		6:  "Flush",
		7:  "Full House",
		8:  "Four of a Kind",
		9:  "Straight Flush",
		10: "Royal Flush",
	}
)

// Betting Limits
const (
	MinBuyIn      = 100
	MinBlindRatio = 0.01 // Minimum blind as % of buy-in
	MaxBlindRatio = 0.1  // Maximum blind as % of buy-in
)

// UI Messages
const (
	TimeoutMessage     = "You did not respond in time. The interaction has timed out."
	GameTimeoutMessage = "You did not respond in time. Your game has timed out and you have forfeited your bet of %d <:chips:1404332422451040330>."
	GameCleanupMessage = "Your game has been removed due to inactivity. You have forfeited your bet of %d <:chips:1404332422451040330>."
	TopGGVoteLink      = "https://top.gg/bot/1396564026233983108/vote"
)

// Emojis and Discord Elements
const (
	ChipsEmoji = "<:chips:1404332422451040330>"
)

// Prestige and Premium badge emojis (used in profile "badges" row)
// If a prestige level doesn't exist in this map, the code will fall back to a roman numeral.
var PrestigeEmojis = map[int]string{
	1: "<:p1:1404690937925337150>",
	2: "<:p2:1404690951980584990>",
	3: "<:p3:1404690959538851936>",
	4: "<:p4:1404690967067623565>",
	5: "<:p5:1404690974059528233>",
}

// Optional premium animated emoji (may be used in future badge rows alongside prestige)
const PremiumEmoji = "<a:goat:1404690990505267384>"
