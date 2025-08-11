package utils

import (
	"fmt"
	"math/rand"
	"time"
)

// Card represents a playing card
type Card struct {
	Rank string `json:"rank"`
	Suit string `json:"suit"`
}

// NewCard creates a new card
func NewCard(rank, suit string) Card {
	return Card{
		Rank: rank,
		Suit: suit,
	}
}

// String returns the string representation of a card
func (c Card) String() string {
	return c.Rank + c.Suit
}

// GetValue returns the numeric value of a card for a specific game
func (c Card) GetValue(game string) int {
	switch game {
	case "blackjack":
		return c.getBlackjackValue()
	case "baccarat":
		return c.getBaccaratValue()
	default:
		if value, exists := CardRanks[c.Rank]; exists {
			return value
		}
		return 0
	}
}

// CardRanks defines the basic values for card ranks
var CardRanks = map[string]int{
	"2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "10": 10,
	"J": 10, "Q": 10, "K": 10, "A": 11,
}

// CardSuits defines the available card suits
var CardSuits = []string{"♠️", "♥️", "♦️", "♣️"}

// getBlackjackValue returns the blackjack value of the card
func (c Card) getBlackjackValue() int {
	if value, exists := CardRanks[c.Rank]; exists {
		return value
	}
	return 0
}

// getBaccaratValue returns the baccarat value of the card (0-9)
func (c Card) getBaccaratValue() int {
	switch c.Rank {
	case "A":
		return 1
	case "2", "3", "4", "5", "6", "7", "8", "9":
		if value, exists := CardRanks[c.Rank]; exists {
			return value
		}
		return 0
	case "10", "J", "Q", "K":
		return 0
	default:
		return 0
	}
}

// IsAce checks if the card is an Ace
func (c Card) IsAce() bool {
	return c.Rank == "A"
}

// IsTen checks if the card has a value of 10 (10, J, Q, K)
func (c Card) IsTen() bool {
	return c.Rank == "10" || c.Rank == "J" || c.Rank == "Q" || c.Rank == "K"
}

// IsRed checks if the card is red (hearts or diamonds)
func (c Card) IsRed() bool {
	return c.Suit == "♥️" || c.Suit == "♦️"
}

// IsBlack checks if the card is black (spades or clubs)
func (c Card) IsBlack() bool {
	return c.Suit == "♠️" || c.Suit == "♣️"
}

// Deck represents a deck of playing cards
type Deck struct {
	Cards       []Card `json:"cards"`
	NumDecks    int    `json:"num_decks"`
	Game        string `json:"game"`
	DealtCards  int    `json:"dealt_cards"`
	TotalCards  int    `json:"total_cards"`
	rng         *rand.Rand
}

// NewDeck creates a new deck of cards
func NewDeck(numDecks int, game string) *Deck {
	deck := &Deck{
		Cards:      make([]Card, 0, numDecks*52),
		NumDecks:   numDecks,
		Game:       game,
		DealtCards: 0,
		TotalCards: numDecks * 52,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Create cards for each deck
	ranks := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	suits := CardSuits

	for d := 0; d < numDecks; d++ {
		for _, suit := range suits {
			for _, rank := range ranks {
				deck.Cards = append(deck.Cards, NewCard(rank, suit))
			}
		}
	}

	deck.Shuffle()
	return deck
}

// Shuffle shuffles the deck
func (d *Deck) Shuffle() {
	d.rng.Shuffle(len(d.Cards), func(i, j int) {
		d.Cards[i], d.Cards[j] = d.Cards[j], d.Cards[i]
	})
	d.DealtCards = 0
}

// Deal deals one card from the deck
func (d *Deck) Deal() Card {
	if d.DealtCards >= len(d.Cards) {
		// Reshuffle if no cards left
		d.Shuffle()
	}

	card := d.Cards[d.DealtCards]
	d.DealtCards++
	return card
}

// CardsRemaining returns the number of cards left in the deck
func (d *Deck) CardsRemaining() int {
	return len(d.Cards) - d.DealtCards
}

// ShouldShuffle determines if the deck should be reshuffled based on game rules
func (d *Deck) ShouldShuffle() bool {
	remaining := float64(d.CardsRemaining())
	total := float64(d.TotalCards)
	threshold := 0.25 // Shuffle when 25% of cards remain

	switch d.Game {
	case "blackjack":
		threshold = 0.25
	case "baccarat":
		threshold = 0.15
	default:
		threshold = 0.25
	}

	return (remaining / total) <= threshold
}

// Hand represents a hand of playing cards
type Hand struct {
	Cards []Card `json:"cards"`
	Game  string `json:"game"`
}

// NewHand creates a new hand
func NewHand(game string) *Hand {
	return &Hand{
		Cards: make([]Card, 0),
		Game:  game,
	}
}

// AddCard adds a card to the hand
func (h *Hand) AddCard(card Card) {
	h.Cards = append(h.Cards, card)
}

// GetValue returns the total value of the hand for the specific game
func (h *Hand) GetValue() int {
	switch h.Game {
	case "blackjack":
		return h.getBlackjackValue()
	case "baccarat":
		return h.getBaccaratValue()
	default:
		total := 0
		for _, card := range h.Cards {
			total += card.GetValue(h.Game)
		}
		return total
	}
}

// getBlackjackValue calculates blackjack hand value with Ace handling
func (h *Hand) getBlackjackValue() int {
	total := 0
	aces := 0

	for _, card := range h.Cards {
		if card.IsAce() {
			aces++
			total += 11
		} else {
			total += card.GetValue("blackjack")
		}
	}

	// Adjust for Aces
	for aces > 0 && total > 21 {
		total -= 10
		aces--
	}

	return total
}

// getBaccaratValue calculates baccarat hand value (modulo 10)
func (h *Hand) getBaccaratValue() int {
	total := 0
	for _, card := range h.Cards {
		total += card.getBaccaratValue()
	}
	return total % 10
}

// String returns string representation of the hand
func (h *Hand) String() string {
	result := ""
	for i, card := range h.Cards {
		if i > 0 {
			result += " "
		}
		result += card.String()
	}
	return result
}

// IsBlackjack checks if the hand is a natural blackjack (21 with 2 cards)
func (h *Hand) IsBlackjack() bool {
	return h.Game == "blackjack" && len(h.Cards) == 2 && h.GetValue() == 21
}

// IsBusted checks if the hand is busted (over 21 in blackjack)
func (h *Hand) IsBusted() bool {
	return h.Game == "blackjack" && h.GetValue() > 21
}

// IsSoft checks if the hand contains a "soft" Ace (counted as 11)
func (h *Hand) IsSoft() bool {
	if h.Game != "blackjack" {
		return false
	}

	total := 0
	hasUsableAce := false

	for _, card := range h.Cards {
		if card.IsAce() {
			if total+11 <= 21 {
				total += 11
				hasUsableAce = true
			} else {
				total += 1
			}
		} else {
			total += card.GetValue("blackjack")
		}
	}

	return hasUsableAce && total <= 21
}

// CanSplit checks if the hand can be split (two cards of same rank)
func (h *Hand) CanSplit() bool {
	return len(h.Cards) == 2 && h.Cards[0].Rank == h.Cards[1].Rank
}

// Count returns the number of cards in the hand
func (h *Hand) Count() int {
	return len(h.Cards)
}