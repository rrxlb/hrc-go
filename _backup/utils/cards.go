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
		Cards:      make([]Card, 0),
		NumDecks:   numDecks,
		Game:       game,
		DealtCards: 0,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	
	deck.buildDeck()
	deck.shuffle()
	
	return deck
}

// buildDeck builds a standard 52-card deck multiplied by numDecks
func (d *Deck) buildDeck() {
	d.Cards = make([]Card, 0, 52*d.NumDecks)
	
	// Create all cards for each deck
	for deckNum := 0; deckNum < d.NumDecks; deckNum++ {
		for _, suit := range CardSuits {
			for rank := range CardRanks {
				card := NewCard(rank, suit)
				d.Cards = append(d.Cards, card)
			}
		}
	}
	
	d.TotalCards = len(d.Cards)
	d.DealtCards = 0
}

// Shuffle shuffles the deck
func (d *Deck) shuffle() {
	d.rng.Shuffle(len(d.Cards), func(i, j int) {
		d.Cards[i], d.Cards[j] = d.Cards[j], d.Cards[i]
	})
}

// Deal deals a single card from the deck
func (d *Deck) Deal() Card {
	if d.IsEmpty() {
		// If deck is empty, rebuild and shuffle
		d.buildDeck()
		d.shuffle()
	}
	
	card := d.Cards[d.DealtCards]
	d.DealtCards++
	
	return card
}

// DealMultiple deals multiple cards from the deck
func (d *Deck) DealMultiple(count int) []Card {
	cards := make([]Card, count)
	for i := 0; i < count; i++ {
		cards[i] = d.Deal()
	}
	return cards
}

// IsEmpty checks if the deck is empty or needs reshuffling
func (d *Deck) IsEmpty() bool {
	return d.DealtCards >= d.TotalCards
}

// ShouldShuffle checks if the deck should be shuffled based on the shuffle threshold
func (d *Deck) ShouldShuffle() bool {
	remaining := float64(d.TotalCards - d.DealtCards)
	total := float64(d.TotalCards)
	
	return (remaining / total) <= ShuffleThreshold
}

// CardsRemaining returns the number of cards remaining in the deck
func (d *Deck) CardsRemaining() int {
	return d.TotalCards - d.DealtCards
}

// Reset resets the deck to its initial state
func (d *Deck) Reset() {
	d.buildDeck()
	d.shuffle()
}

// Hand represents a collection of cards
type Hand struct {
	Cards []Card `json:"cards"`
	Game  string `json:"game"`
}

// NewHand creates a new empty hand
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

// AddCards adds multiple cards to the hand
func (h *Hand) AddCards(cards []Card) {
	h.Cards = append(h.Cards, cards...)
}

// GetValue calculates the total value of the hand for the specific game
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
		value := card.getBlackjackValue()
		total += value
		if card.IsAce() {
			aces++
		}
	}
	
	// Adjust for Aces (count as 1 instead of 11 if total > 21)
	for aces > 0 && total > 21 {
		total -= 10 // Convert an Ace from 11 to 1
		aces--
	}
	
	return total
}

// getBaccaratValue calculates baccarat hand value (rightmost digit)
func (h *Hand) getBaccaratValue() int {
	total := 0
	for _, card := range h.Cards {
		total += card.getBaccaratValue()
	}
	return total % 10
}

// IsBlackjack checks if the hand is a natural blackjack
func (h *Hand) IsBlackjack() bool {
	if h.Game != "blackjack" || len(h.Cards) != 2 {
		return false
	}
	
	return h.GetValue() == 21
}

// IsBust checks if the hand is busted (> 21 in blackjack)
func (h *Hand) IsBust() bool {
	if h.Game != "blackjack" {
		return false
	}
	
	return h.GetValue() > 21
}

// IsFiveCardCharlie checks if the hand qualifies for five card charlie
func (h *Hand) IsFiveCardCharlie() bool {
	if h.Game != "blackjack" {
		return false
	}
	
	return len(h.Cards) == 5 && !h.IsBust()
}

// HasAce checks if the hand contains an Ace
func (h *Hand) HasAce() bool {
	for _, card := range h.Cards {
		if card.IsAce() {
			return true
		}
	}
	return false
}

// HasSoftAce checks if the hand has an Ace counted as 11
func (h *Hand) HasSoftAce() bool {
	if !h.HasAce() {
		return false
	}
	
	// Calculate value without soft ace conversion
	hardTotal := 0
	for _, card := range h.Cards {
		if card.IsAce() {
			hardTotal += 1
		} else {
			hardTotal += card.getBlackjackValue()
		}
	}
	
	// If we can use an ace as 11 without busting, we have a soft ace
	return hardTotal + 10 <= 21
}

// CanSplit checks if the hand can be split (two cards of same rank)
func (h *Hand) CanSplit() bool {
	if len(h.Cards) != 2 {
		return false
	}
	
	// Check if both cards have the same rank or value
	card1, card2 := h.Cards[0], h.Cards[1]
	
	// Allow splitting of any two 10-value cards (10, J, Q, K)
	if card1.GetValue(h.Game) == 10 && card2.GetValue(h.Game) == 10 {
		return true
	}
	
	// Otherwise, must be same rank
	return card1.Rank == card2.Rank
}

// Split splits the hand into two separate hands
func (h *Hand) Split() (*Hand, *Hand) {
	if !h.CanSplit() {
		return nil, nil
	}
	
	hand1 := NewHand(h.Game)
	hand2 := NewHand(h.Game)
	
	hand1.AddCard(h.Cards[0])
	hand2.AddCard(h.Cards[1])
	
	return hand1, hand2
}

// String returns a string representation of the hand
func (h *Hand) String() string {
	if len(h.Cards) == 0 {
		return "Empty hand"
	}
	
	result := ""
	for _, card := range h.Cards {
		result += card.String() + " "
	}
	
	return fmt.Sprintf("%s(%d)", result, h.GetValue())
}

// Size returns the number of cards in the hand
func (h *Hand) Size() int {
	return len(h.Cards)
}

// IsEmpty checks if the hand is empty
func (h *Hand) IsEmpty() bool {
	return len(h.Cards) == 0
}

// Clear removes all cards from the hand
func (h *Hand) Clear() {
	h.Cards = make([]Card, 0)
}

// GetLastCard returns the last card added to the hand
func (h *Hand) GetLastCard() *Card {
	if len(h.Cards) == 0 {
		return nil
	}
	
	return &h.Cards[len(h.Cards)-1]
}

// RemoveLastCard removes and returns the last card from the hand
func (h *Hand) RemoveLastCard() *Card {
	if len(h.Cards) == 0 {
		return nil
	}
	
	lastCard := h.Cards[len(h.Cards)-1]
	h.Cards = h.Cards[:len(h.Cards)-1]
	
	return &lastCard
}