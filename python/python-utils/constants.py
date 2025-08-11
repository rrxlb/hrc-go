from collections import OrderedDict

# --- General ---
GUILD_ID = 1396567190102347776
HIGH_ROLLERS_CLUB_LINK = "https://discord.gg/RK4K8tDsHB"
DEVELOPER_ROLE_ID = 1399860810574073978  # Replace with actual developer role ID

BOT_COLOR = 0x5865F2
PREMIUM_ROLE_ID = 1396631093154943026
# --- Economy & XP ---
STARTING_CHIPS = 1000
DAILY_REWARD = 500
XP_PER_PROFIT = 2

# --- Ranks ---
# Using an OrderedDict to maintain insertion order for rank progression
RANKS = OrderedDict([
    (0, {"name": "Novice", "icon": "🥉", "xp_required": 0, "color": 0xcd7f32}),
    (1, {"name": "Apprentice", "icon": "🥈", "xp_required": 10000, "color": 0xc0c0c0}),
    (2, {"name": "Gambler", "icon": "🥇", "xp_required": 40000, "color": 0xffd700}),
    (3, {"name": "High Roller", "icon": "💰", "xp_required": 125000, "color": 0x22a7f0}),
    (4, {"name": "Card Shark", "icon": "🦈", "xp_required": 350000, "color": 0x1f3a93}),
    (5, {"name": "Pit Boss", "icon": "👑", "xp_required": 650000, "color": 0x9b59b6}),
    (6, {"name": "Legend", "icon": "🌟", "xp_required": 2000000, "color": 0xf1c40f}),
    (7, {"name": "Casino Elite", "icon": "💎", "xp_required": 4500000, "color": 0x1abc9c}),
])

# Maximum XP tracked for progression before requiring prestige
MAX_CURRENT_XP = RANKS[max(RANKS.keys())]["xp_required"]

# --- Blackjack Game ---
CARD_SUITS = ["♠️", "♥️", "♦️", "♣️"]
CARD_RANKS = {
    "2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "10": 10,
    "J": 10, "Q": 10, "K": 10, "A": 11
}
DECK_COUNT = 6  # Standard shoe size
SHUFFLE_THRESHOLD = 0.25  # Shuffle when 25% of the shoe remains
DEALER_STAND_VALUE = 17
BLACKJACK_PAYOUT = 1.5
FIVE_CARD_CHARLIE_PAYOUT = 1.75

# --- Baccarat Game ---
BACCARAT_PAYOUT = 1.0
BACCARAT_TIE_PAYOUT = 8.0
BACCARAT_BANKER_COMMISSION = 0.05

# --- Poker Game ---
POKER_HAND_RANKINGS = {
    "high_card": 1,
    "pair": 2,
    "two_pair": 3,
    "three_of_a_kind": 4,
    "straight": 5,
    "flush": 6,
    "full_house": 7,
    "four_of_a_kind": 8,
    "straight_flush": 9,
    "royal_flush": 10
}

# Poker hand ranking names for display
POKER_HAND_NAMES = {
    1: "High Card",
    2: "Pair",
    3: "Two Pair", 
    4: "Three of a Kind",
    5: "Straight",
    6: "Flush",
    7: "Full House",
    8: "Four of a Kind",
    9: "Straight Flush",
    10: "Royal Flush"
}

# Minimum bet validation
MIN_BUY_IN = 100
MIN_BLIND_RATIO = 0.01  # Minimum blind as % of buy-in
MAX_BLIND_RATIO = 0.1   # Maximum blind as % of buy-in

# --- UI ---
TIMEOUT_MESSAGE = "You did not respond in time. The interaction has timed out."
GAME_TIMEOUT_MESSAGE = "You did not respond in time. Your game has timed out and you have forfeited your bet of {bet_amount:,} <:chips:1396988413151940629>."
GAME_CLEANUP_MESSAGE = "Your game has been removed due to inactivity. You have forfeited your bet of {bet_amount:,} <:chips:1396988413151940629>."
TOPGG_VOTE_LINK = "https://top.gg/bot/1396564026233983108/vote"