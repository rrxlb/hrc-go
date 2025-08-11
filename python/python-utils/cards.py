import random
from typing import List, Tuple, Dict
from collections import Counter

# --- Card Constants ---
CARD_SUITS = ['♠️', '♥️', '♦️', '♣️']
CARD_RANKS = {
    "2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "10": 10,
    "J": 11, "Q": 12, "K": 13, "A": 14
}
BLACKJACK_RANKS = {
    "2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "10": 10,
    "J": 10, "Q": 10, "K": 10, "A": 11
}
BACCARAT_RANKS = {
    "A": 1, "2": 2, "3": 3, "4": 4, "5": 5, "6": 6, "7": 7, "8": 8, "9": 9, "10": 0, "J": 0, "Q": 0, "K": 0
}

# --- Helper Classes ---
class Card:
    def __init__(self, rank: str, suit: str, game: str = 'blackjack'):
        self.rank = rank
        self.suit = suit
        self.value = self._get_value(game)

    def _get_value(self, game: str) -> int:
        if game == 'baccarat':
            return BACCARAT_RANKS.get(self.rank, 0)
        elif game == 'blackjack':
            return BLACKJACK_RANKS.get(self.rank, 0)
        elif game == 'poker':
            return CARD_RANKS.get(self.rank, 0)
        # Default to poker values
        return CARD_RANKS.get(self.rank, 0)

    def __str__(self) -> str:
        return f"{self.rank}{self.suit}"

class Deck:
    def __init__(self, num_decks: int = 1, game: str = 'blackjack'):
        self.cards: List[Card] = []
        self.num_decks = num_decks
        self.game = game
        self.build()

    def build(self):
        """Builds the deck based on the specified game."""
        if self.game == 'baccarat':
            ranks = BACCARAT_RANKS
        elif self.game == 'blackjack':
            ranks = BLACKJACK_RANKS
        elif self.game == 'poker':
            ranks = CARD_RANKS
        else:
            ranks = CARD_RANKS
        self.cards = [Card(rank, suit, self.game) for _ in range(self.num_decks) for suit in CARD_SUITS for rank in ranks]
        self.shuffle()

    def shuffle(self):
        random.shuffle(self.cards)

    def deal(self) -> Card:
        if not self.cards:
            self.build() # Reshuffle if empty
        return self.cards.pop()

# --- Poker Hand Evaluation ---
class PokerHand:
    """Represents a poker hand with evaluation capabilities"""
    
    def __init__(self, cards: List[Card]):
        self.cards = cards
        self.ranking, self.rank_values = self._evaluate_hand()
        
    def _evaluate_hand(self) -> Tuple[int, List[int]]:
        """Evaluate poker hand and return (ranking, rank_values)"""
        if len(self.cards) != 5:
            raise ValueError("PokerHand evaluation requires exactly 5 cards.")

        values = sorted([c.value for c in self.cards], reverse=True)
        suits = [c.suit for c in self.cards]
        
        is_flush = len(set(suits)) == 1
        is_straight, straight_high = self._check_straight(values)

        if is_straight and is_flush:
            return (9, [straight_high]) if straight_high != 14 else (10, []) # 10 is Royal Flush
        
        rank_counts = Counter(values)
        counts = sorted(rank_counts.values(), reverse=True)
        
        if counts[0] == 4: # Four of a kind
            four_kind = [r for r, c in rank_counts.items() if c == 4][0]
            kicker = [r for r in values if r != four_kind][0]
            return 8, [four_kind, kicker]
        if counts == [3, 2]: # Full House
            three_kind = [r for r, c in rank_counts.items() if c == 3][0]
            pair = [r for r, c in rank_counts.items() if c == 2][0]
            return 7, [three_kind, pair]
        if is_flush:
            return 6, values
        if is_straight:
            return 5, [straight_high]
        if counts[0] == 3: # Three of a kind
            three_kind = [r for r, c in rank_counts.items() if c == 3][0]
            kickers = sorted([r for r in values if r != three_kind], reverse=True)
            return 4, [three_kind] + kickers
        if counts == [2, 2, 1]: # Two Pair
            pairs = sorted([r for r, c in rank_counts.items() if c == 2], reverse=True)
            kicker = [r for r, c in rank_counts.items() if c == 1][0]
            return 3, pairs + [kicker]
        if counts[0] == 2: # One Pair
            pair = [r for r, c in rank_counts.items() if c == 2][0]
            kickers = sorted([r for r in values if r != pair], reverse=True)
            return 2, [pair] + kickers
        
        return 1, values # High Card

    def _check_straight(self, sorted_values: List[int]) -> Tuple[bool, int]:
        """Check if a sorted list of 5 values is a straight."""
        # Ace-low straight (A, 5, 4, 3, 2)
        if sorted_values == [14, 5, 4, 3, 2]:
            return True, 5
        
        is_straight = all(sorted_values[i] - sorted_values[i+1] == 1 for i in range(len(sorted_values)-1))
        if is_straight:
            return True, sorted_values[0]
            
        return False, 0
    
    def __lt__(self, other: 'PokerHand') -> bool:
        """Compare poker hands (self < other)"""
        if self.ranking != other.ranking:
            return self.ranking < other.ranking
        
        # Same ranking, compare rank values
        for my_val, other_val in zip(self.rank_values, other.rank_values):
            if my_val != other_val:
                return my_val < other_val
        
        return False  # Hands are equal
    
    def __eq__(self, other: 'PokerHand') -> bool:
        """Check if poker hands are equal"""
        return self.ranking == other.ranking and self.rank_values == other.rank_values
    
    def get_hand_name(self) -> str:
        """Get human-readable name of the hand"""
        from utils.constants import POKER_HAND_NAMES
        return POKER_HAND_NAMES.get(self.ranking, "Unknown")

def evaluate_best_poker_hand(cards: List[Card]) -> PokerHand:
    """Evaluate the best 5-card poker hand from 7 cards"""
    if len(cards) < 5:
        raise ValueError("Need at least 5 cards to evaluate poker hand")
    
    if len(cards) == 5:
        return PokerHand(cards)
    
    # Generate all possible 5-card combinations
    from itertools import combinations
    best_hand = None
    
    for combo in combinations(cards, 5):
        hand = PokerHand(list(combo))
        if best_hand is None or hand > best_hand:
            best_hand = hand
    
    return best_hand

def get_hand_strength(cards: List[Card]) -> float:
    """Get hand strength as a float between 0.0 and 1.0 for AI decision making"""
    if len(cards) < 5:
        # Pre-flop hand strength estimation
        if len(cards) == 2:
            return _evaluate_preflop_strength(cards)
        else:
            return 0.0
    
    hand = evaluate_best_poker_hand(cards)
    
    # Convert ranking to strength (0.0 to 1.0)
    base_strength = hand.ranking / 10.0
    
    # Add fine-tuning based on rank values
    if hand.ranking >= 2:  # Pair or better
        high_card_bonus = hand.rank_values[0] / 14.0 * 0.1
        base_strength += high_card_bonus
    
    return min(1.0, base_strength)

def _evaluate_preflop_strength(hole_cards: List[Card]) -> float:
    """Evaluate pre-flop hand strength for 2 hole cards"""
    if len(hole_cards) != 2:
        return 0.0
    
    card1, card2 = hole_cards
    val1, val2 = card1.value, card2.value
    
    # Pocket pairs
    if val1 == val2:
        if val1 >= 10:  # High pairs (10s or better)
            return 0.85 + (val1 - 10) * 0.03
        elif val1 >= 7:  # Medium pairs
            return 0.65 + (val1 - 7) * 0.05
        else:  # Low pairs
            return 0.45 + (val1 - 2) * 0.04
    
    # Non-pairs
    high_val = max(val1, val2)
    low_val = min(val1, val2)
    
    # Suited bonus
    suited_bonus = 0.05 if card1.suit == card2.suit else 0.0
    
    # High cards (A, K, Q, J)
    if high_val >= 11:
        if low_val >= 11:  # Both high cards
            return 0.75 + suited_bonus
        elif low_val >= 10:  # One ace/king/queen with 10/J
            return 0.55 + suited_bonus  
        elif low_val >= 7:  # High card with decent kicker
            return 0.45 + suited_bonus
        else:
            return 0.35 + suited_bonus
    
    # Medium strength hands
    if high_val >= 8 and low_val >= 6:
        return 0.35 + suited_bonus
    
    # Connected cards (straight potential)
    if abs(val1 - val2) <= 4:
        return 0.25 + suited_bonus
    
    # Default low strength
    return 0.15 + suited_bonus
