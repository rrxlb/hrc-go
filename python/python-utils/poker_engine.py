"""
Texas Hold 'Em Poker Engine
Handles core game logic, betting rounds, pot management, and game flow
"""

import random
from typing import List, Dict, Optional, Tuple, Any
from enum import Enum
from dataclasses import dataclass, field

from .cards import Card, Deck, evaluate_best_poker_hand


class GamePhase(Enum):
    """Poker game phases"""
    WAITING = "waiting"
    PRE_FLOP = "pre_flop"
    FLOP = "flop"
    TURN = "turn"
    RIVER = "river"
    SHOWDOWN = "showdown"
    FINISHED = "finished"


class PlayerAction(Enum):
    """Possible player actions"""
    FOLD = "fold"
    CHECK = "check"
    CALL = "call"
    RAISE = "raise"
    ALL_IN = "all_in"


@dataclass
class Player:
    """Represents a poker player"""
    user_id: int
    name: str
    starting_chips: int
    chips: int
    is_bot: bool = False
    hole_cards: List[Card] = field(default_factory=list)
    current_bet: int = 0
    total_bet_in_hand: int = 0
    is_folded: bool = False
    is_all_in: bool = False
    is_sitting_out: bool = False


@dataclass 
class Pot:
    """Represents a poker pot (main or side pot)"""
    amount: int = 0
    eligible_players: List[int] = field(default_factory=list)


class PokerEngine:
    """Core Texas Hold 'Em poker engine"""
    
    def __init__(self, channel_id: int, small_blind: int = 10, big_blind: int = 20):
        self.channel_id = channel_id
        self.small_blind = small_blind
        self.big_blind = big_blind
        
        self.players: List[Player] = []
        self.deck = Deck(num_decks=1, game='poker')
        self.community_cards: List[Card] = []
        self.pots: List[Pot] = [Pot()]
        self.phase = GamePhase.WAITING
        
        self.dealer_index = -1
        self.current_player_index = -1
        self.last_raiser_index = -1
        
        self.current_bet_amount = 0
        self.min_raise_amount = 0
        self.hand_number = 0
        self.action_history: List[Dict[str, Any]] = []
        
    def add_player(self, user_id: int, name: str, chips: int, is_bot: bool = False) -> bool:
        if len(self.players) >= 9 or any(p.user_id == user_id for p in self.players):
            return False
            
        player = Player(user_id=user_id, name=name, starting_chips=chips, chips=chips, is_bot=is_bot)
        self.players.append(player)
        return True
    
    def remove_player(self, user_id: int) -> bool:
        player = self._get_player_by_id(user_id)
        if not player:
            return False
        
        if self.phase != GamePhase.WAITING:
            player.is_sitting_out = True
            player.is_folded = True
        else:
            self.players.remove(player)
        return True
    
    def get_active_players(self, in_hand_only=False, not_all_in=False) -> List[Player]:
        """Helper to get players who are still in the game/hand."""
        p_list = [p for p in self.players if not p.is_sitting_out]
        if in_hand_only:
            p_list = [p for p in p_list if not p.is_folded]
        if not_all_in:
            p_list = [p for p in p_list if not p.is_all_in]
        return p_list

    def start_new_hand(self) -> bool:
        active_players = self.get_active_players()
        if len(active_players) < 2:
            return False
            
        self.hand_number += 1
        self.deck = Deck(num_decks=1, game='poker')
        self.community_cards = []
        self.pots = [Pot(eligible_players=[p.user_id for p in active_players])]
        self.action_history = []
        self.phase = GamePhase.PRE_FLOP
        
        for player in self.players:
            player.hole_cards = []
            player.current_bet = 0
            player.total_bet_in_hand = 0
            player.is_folded = False
            player.is_all_in = False
        
        self.dealer_index = self._get_next_player_index(self.dealer_index)
        
        for _ in range(2):
            for i in range(len(active_players)):
                player_index = (self.dealer_index + 1 + i) % len(self.players)
                player = self.players[player_index]
                if not player.is_sitting_out:
                    player.hole_cards.append(self.deck.deal())
        
        self._post_blinds()
        return True
    
    def _post_blinds(self):
        active_players = self.get_active_players()
        sb_index = self._get_next_player_index(self.dealer_index)
        bb_index = self._get_next_player_index(sb_index)

        # Heads-up case
        if len(active_players) == 2:
            sb_index = self.dealer_index
            bb_index = self._get_next_player_index(sb_index)

        self._post_bet(sb_index, self.small_blind)
        self._post_bet(bb_index, self.big_blind)

        self.current_bet_amount = self.big_blind
        self.min_raise_amount = self.big_blind
        self.current_player_index = self._get_next_player_index(bb_index)
        self.last_raiser_index = bb_index

    def _post_bet(self, player_index: int, amount: int):
        player = self.players[player_index]
        bet_amount = min(amount, player.chips)
        player.chips -= bet_amount
        player.current_bet += bet_amount
        player.total_bet_in_hand += bet_amount
        if player.chips == 0:
            player.is_all_in = True
    
    def get_current_player(self) -> Optional[Player]:
        if self.current_player_index == -1:
            return None
        return self.players[self.current_player_index]

    def get_valid_actions(self, player: Player) -> List[PlayerAction]:
        if player.is_folded or player.is_all_in or player.is_sitting_out:
            return []
        
        actions = [PlayerAction.FOLD, PlayerAction.ALL_IN]
        bet_to_call = self.current_bet_amount - player.current_bet
        
        if bet_to_call == 0:
            actions.append(PlayerAction.CHECK)
        elif player.chips > bet_to_call:
            actions.append(PlayerAction.CALL)
        
        if player.chips > bet_to_call:
             actions.append(PlayerAction.RAISE)
        
        return actions

    def execute_action(self, player_id: int, action: PlayerAction, amount: int = 0) -> bool:
        player = self._get_player_by_id(player_id)
        if not player or player != self.get_current_player():
            return False
        
        if action not in self.get_valid_actions(player):
            return False
        
        if action == PlayerAction.FOLD:
            player.is_folded = True
        elif action == PlayerAction.CHECK:
            pass
        elif action == PlayerAction.CALL:
            call_amount = self.current_bet_amount - player.current_bet
            self._post_bet(self.current_player_index, call_amount)
        elif action == PlayerAction.ALL_IN:
            self._post_bet(self.current_player_index, player.chips)
        elif action == PlayerAction.RAISE:
            call_amount = self.current_bet_amount - player.current_bet
            raise_amount = amount - self.current_bet_amount
            
            if amount < self.current_bet_amount + self.min_raise_amount:
                return False # Raise is too small
            
            self._post_bet(self.current_player_index, call_amount + raise_amount)
            self.current_bet_amount = player.current_bet
            self.min_raise_amount = raise_amount
            self.last_raiser_index = self.current_player_index

        self.action_history.append({'player_id': player_id, 'action': action.value, 'amount': amount})
        
        if self._is_betting_round_complete():
            self._next_phase()
        else:
            self.current_player_index = self._get_next_player_index(self.current_player_index, in_hand_only=True, not_all_in=True)
        
        return True

    def _is_betting_round_complete(self) -> bool:
        active_players = self.get_active_players(in_hand_only=True)
        if len(active_players) <= 1:
            return True
            
        acting_players = self.get_active_players(in_hand_only=True, not_all_in=True)
        if not acting_players:
            return True

        # Check if all acting players have matched the current bet
        for player in acting_players:
            if player.current_bet != self.current_bet_amount:
                return False
        
        # Check if action has returned to the last raiser
        if self.current_player_index == self.last_raiser_index:
            return True
            
        return False

    def _next_phase(self):
        self._create_side_pots()
        
        for player in self.players:
            player.current_bet = 0
        
        self.current_bet_amount = 0
        self.min_raise_amount = self.big_blind
        self.current_player_index = self._get_next_player_index(self.dealer_index, in_hand_only=True, not_all_in=True)
        self.last_raiser_index = self.current_player_index

        if self.phase == GamePhase.PRE_FLOP: self.phase = GamePhase.FLOP; self._deal_community(3)
        elif self.phase == GamePhase.FLOP: self.phase = GamePhase.TURN; self._deal_community(1)
        elif self.phase == GamePhase.TURN: self.phase = GamePhase.RIVER; self._deal_community(1)
        elif self.phase == GamePhase.RIVER: self.phase = GamePhase.SHOWDOWN
        
        if self.phase == GamePhase.SHOWDOWN or len(self.get_active_players(in_hand_only=True)) <= 1:
            self._determine_winners()

    def _deal_community(self, count: int):
        self.deck.deal() # Burn card
        for _ in range(count):
            self.community_cards.append(self.deck.deal())

    def _create_side_pots(self):
        # Simplified side pot logic for now
        # A full implementation is more complex
        total_pot = sum(p.total_bet_in_hand for p in self.players)
        self.pots = [Pot(amount=total_pot, eligible_players=[p.user_id for p in self.get_active_players(in_hand_only=True)])]

    def _determine_winners(self) -> List[Dict[str, Any]]:
        self.phase = GamePhase.FINISHED
        winners_data = []
        
        for pot in self.pots:
            pot_winners = []
            best_hand = None
            
            eligible_contenders = [p for p in self.players if p.user_id in pot.eligible_players and not p.is_folded]
            if not eligible_contenders: continue

            if len(eligible_contenders) == 1:
                pot_winners = eligible_contenders
            else:
                hand_evals = []
                for player in eligible_contenders:
                    all_cards = player.hole_cards + self.community_cards
                    hand_evals.append((player, evaluate_best_poker_hand(all_cards)))
                
                hand_evals.sort(key=lambda x: (x[1].ranking, x[1].rank_values), reverse=True)
                best_hand = hand_evals[0][1]
                pot_winners = [p for p, h in hand_evals if h.ranking == best_hand.ranking and h.rank_values == best_hand.rank_values]

            if pot_winners:
                win_amount = pot.amount // len(pot_winners)
                for winner in pot_winners:
                    winner.chips += win_amount
                    winners_data.append({'player': winner, 'amount': win_amount, 'hand': best_hand})
        return winners_data

    def _get_player_by_id(self, user_id: int) -> Optional[Player]:
        return next((p for p in self.players if p.user_id == user_id), None)

    def _get_next_player_index(self, start_index: int, in_hand_only=False, not_all_in=False) -> int:
        for i in range(1, len(self.players) + 1):
            next_idx = (start_index + i) % len(self.players)
            player = self.players[next_idx]
            if player.is_sitting_out: continue
            if in_hand_only and player.is_folded: continue
            if not_all_in and player.is_all_in: continue
            return next_idx
        return -1

    def get_game_state(self) -> Dict[str, Any]:
        return {
            'phase': self.phase.value,
            'community_cards_display': [str(card) for card in self.community_cards],
            'pot_total': sum(pot.amount for pot in self.pots),
            'current_bet': self.current_bet_amount,
            'players': [
                {
                    'user_id': p.user_id, 'name': p.name, 'chips': p.chips,
                    'current_bet': p.current_bet, 'is_folded': p.is_folded,
                    'is_all_in': p.is_all_in, 'is_dealer': self.players.index(p) == self.dealer_index,
                    'is_current': p == self.get_current_player(),
                    'hole_cards': [str(card) for card in p.hole_cards]
                } for p in self.players
            ]
        }