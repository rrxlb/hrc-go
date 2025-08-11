"""
AI Bot Personalities for Texas Hold 'Em Poker
Each bot has distinct playing style and decision-making algorithms
"""

import random
import asyncio
from abc import ABC, abstractmethod
from typing import List, Dict, Any, Optional, Tuple
from enum import Enum

from .cards import get_hand_strength
from .poker_engine import Player, PlayerAction, GamePhase


class BotPersonality(ABC):
    """Abstract base class for poker bot personalities"""
    
    def __init__(self, name: str, description: str):
        self.name = name
        self.description = description
        self.memory: Dict[str, Any] = {}  # Bot's memory of game events
        
    @abstractmethod
    async def decide_action(self, game_state: Dict[str, Any], player: Player, valid_actions: List[PlayerAction]) -> Tuple[PlayerAction, int]:
        """
        Decide what action to take given the current game state
        Returns: (action, amount) where amount is used for raises
        """
        pass
    
    def update_memory(self, event: Dict[str, Any]):
        """Update bot's memory with game events"""
        if 'opponent_actions' not in self.memory:
            self.memory['opponent_actions'] = []
        self.memory['opponent_actions'].append(event)
    
    def get_chat_message(self, action: PlayerAction, game_state: Dict[str, Any]) -> Optional[str]:
        """Get an optional chat message based on the action taken"""
        return None


class CalculatingCarla(BotPersonality):
    """
    Calculating Carla - Plays optimally using pot odds and hand strength
    Very mathematical and rarely bluffs
    """
    
    def __init__(self):
        super().__init__(
            name="Calculating Carla",
            description="Mathematical player who calculates pot odds precisely"
        )
        self.aggression_factor = 0.3  # Low aggression
        self.bluff_frequency = 0.05   # Rarely bluffs
    
    async def decide_action(self, game_state: Dict[str, Any], player: Player, valid_actions: List[PlayerAction]) -> Tuple[PlayerAction, int]:
        await asyncio.sleep(random.uniform(2.0, 4.0))  # Thinking time
        
        # Calculate hand strength
        all_cards = player.hole_cards + game_state.get("community_cards", [])
        hand_strength = get_hand_strength(all_cards) if all_cards else get_hand_strength(player.hole_cards)
        
        # Calculate pot odds
        total_pot = game_state.get('pot_total', 0)
        current_bet = game_state.get('current_bet', 0)
        bet_to_call = current_bet - player.current_bet
        
        pot_odds = bet_to_call / (total_pot + bet_to_call) if (total_pot + bet_to_call) > 0 else 0
        
        # Decision logic based on mathematical analysis
        if hand_strength < 0.2:  # Very weak hand
            if bet_to_call == 0 and PlayerAction.CHECK in valid_actions:
                return PlayerAction.CHECK, 0
            else:
                return PlayerAction.FOLD, 0
        
        elif hand_strength < 0.4:  # Weak hand
            if pot_odds > 0.3:  # Good pot odds
                if PlayerAction.CALL in valid_actions:
                    return PlayerAction.CALL, 0
                elif PlayerAction.CHECK in valid_actions:
                    return PlayerAction.CHECK, 0
            return PlayerAction.FOLD, 0
        
        elif hand_strength < 0.7:  # Medium hand
            if bet_to_call == 0 and PlayerAction.CHECK in valid_actions:
                return PlayerAction.CHECK, 0
            elif PlayerAction.CALL in valid_actions and pot_odds < 0.4:
                return PlayerAction.CALL, 0
            elif PlayerAction.RAISE in valid_actions and hand_strength > 0.5:
                raise_amount = min(total_pot // 2, player.chips // 3)
                return PlayerAction.RAISE, current_bet + raise_amount
            else:
                if PlayerAction.CALL in valid_actions:
                    return PlayerAction.CALL, 0
                else:
                    return PlayerAction.FOLD, 0
        
        else:  # Strong hand
            if PlayerAction.RAISE in valid_actions:
                # Value betting with strong hands
                raise_amount = min(total_pot * 0.75, player.chips // 2)
                return PlayerAction.RAISE, int(current_bet + raise_amount)
            elif PlayerAction.CALL in valid_actions:
                return PlayerAction.CALL, 0
            else:
                return PlayerAction.CHECK, 0
    
    def get_chat_message(self, action: PlayerAction, game_state: Dict[str, Any]) -> Optional[str]:
        messages = {
            PlayerAction.FOLD: ["The math doesn't add up.", "Negative expected value.", "🧮"],
            PlayerAction.CALL: ["Pot odds are favorable.", "I'll see that bet.", "📊"],
            PlayerAction.RAISE: ["Positive EV play here.", "The numbers support this raise.", "💹"],
            PlayerAction.CHECK: ["Optimal to check here.", "No value in betting.", "✓"]
        }
        
        if random.random() < 0.3:  # 30% chance to chat
            return random.choice(messages.get(action, []))
        return None


class TimidTom(BotPersonality):
    """
    Timid Tom - Tight-passive player who folds often and rarely bluffs
    Very conservative and only plays premium hands
    """
    
    def __init__(self):
        super().__init__(
            name="Timid Tom", 
            description="Conservative player who only plays premium hands"
        )
        self.aggression_factor = 0.1  # Very low aggression
        self.bluff_frequency = 0.01   # Almost never bluffs
        self.fold_threshold = 0.4     # Folds anything below medium strength
    
    async def decide_action(self, game_state: Dict[str, Any], player: Player, valid_actions: List[PlayerAction]) -> Tuple[PlayerAction, int]:
        await asyncio.sleep(random.uniform(1.5, 3.0))  # Quick decisions, usually folds
        
        # Calculate hand strength
        all_cards = player.hole_cards + game_state.get("community_cards", [])
        hand_strength = get_hand_strength(all_cards) if all_cards else get_hand_strength(player.hole_cards)
        
        current_bet = game_state.get('current_bet', 0)
        bet_to_call = current_bet - player.current_bet
        
        # Very conservative decision making
        if hand_strength < self.fold_threshold:
            if bet_to_call == 0 and PlayerAction.CHECK in valid_actions:
                return PlayerAction.CHECK, 0
            else:
                return PlayerAction.FOLD, 0
        
        elif hand_strength < 0.6:  # Medium hands
            if bet_to_call == 0:
                return PlayerAction.CHECK, 0
            elif bet_to_call < player.chips * 0.1:  # Only call small bets
                if PlayerAction.CALL in valid_actions:
                    return PlayerAction.CALL, 0
                else:
                    return PlayerAction.FOLD, 0
            else:
                return PlayerAction.FOLD, 0
        
        elif hand_strength < 0.8:  # Good hands
            if PlayerAction.CALL in valid_actions:
                return PlayerAction.CALL, 0
            elif PlayerAction.CHECK in valid_actions:
                return PlayerAction.CHECK, 0
            else:
                return PlayerAction.FOLD, 0
        
        else:  # Premium hands only
            if PlayerAction.RAISE in valid_actions and random.random() < 0.3:
                # Small value bet with premium hands
                raise_amount = min(current_bet, player.chips // 4)
                return PlayerAction.RAISE, current_bet + raise_amount
            elif PlayerAction.CALL in valid_actions:
                return PlayerAction.CALL, 0
            else:
                return PlayerAction.CHECK, 0
    
    def get_chat_message(self, action: PlayerAction, game_state: Dict[str, Any]) -> Optional[str]:
        messages = {
            PlayerAction.FOLD: ["I'm not feeling good about this hand.", "Better safe than sorry.", "😰", "This is too rich for my blood."],
            PlayerAction.CALL: ["I suppose I'll call...", "Reluctantly calling.", "😬"],
            PlayerAction.RAISE: ["I actually have something here!", "This is a rare moment for me.", "😅"],
            PlayerAction.CHECK: ["I'll just check.", "No need to get fancy.", "😐"]
        }
        
        if random.random() < 0.4:  # 40% chance to chat (Tom is chatty when nervous)
            return random.choice(messages.get(action, []))
        return None


class WildWill(BotPersonality):
    """
    Wild Will - Aggressive and unpredictable player who bluffs often
    High variance playing style with frequent raises and all-ins
    """
    
    def __init__(self):
        super().__init__(
            name="Wild Will",
            description="Unpredictable aggressive player who loves to bluff"
        )
        self.aggression_factor = 0.8  # Very high aggression
        self.bluff_frequency = 0.4    # Bluffs frequently
        self.tilt_factor = 0.0        # Increases after losses
    
    async def decide_action(self, game_state: Dict[str, Any], player: Player, valid_actions: List[PlayerAction]) -> Tuple[PlayerAction, int]:
        await asyncio.sleep(random.uniform(1.0, 3.0))  # Variable timing to be unpredictable
        
        # Calculate hand strength
        all_cards = player.hole_cards + game_state.get("community_cards", [])
        hand_strength = get_hand_strength(all_cards) if all_cards else get_hand_strength(player.hole_cards)
        
        current_bet = game_state.get('current_bet', 0)
        total_pot = game_state.get('pot_total', 0)
        
        # Apply tilt factor (makes Will more aggressive after losses)
        effective_aggression = min(1.0, self.aggression_factor + self.tilt_factor)
        
        # Wild Will's unpredictable logic
        random_factor = random.random()
        
        # Bluff opportunity
        if random_factor < self.bluff_frequency and hand_strength < 0.3:
            if PlayerAction.RAISE in valid_actions:
                # Big bluff
                bluff_size = random.choice([total_pot // 2, total_pot, player.chips // 2])
                return PlayerAction.RAISE, current_bet + bluff_size
            elif PlayerAction.ALL_IN in valid_actions and random.random() < 0.2:
                return PlayerAction.ALL_IN, 0
        
        # Normal aggressive play
        if hand_strength < 0.2:  # Weak hand
            if random.random() < 0.3:  # Sometimes bluffs even with terrible hands
                if PlayerAction.RAISE in valid_actions:
                    return PlayerAction.RAISE, current_bet + (total_pot // 3)
                elif PlayerAction.CALL in valid_actions and random.random() < 0.2:
                    return PlayerAction.CALL, 0
            
            if PlayerAction.CHECK in valid_actions:
                return PlayerAction.CHECK, 0
            else:
                return PlayerAction.FOLD, 0
        
        elif hand_strength < 0.5:  # Medium-weak hand
            if random.random() < effective_aggression:
                if PlayerAction.RAISE in valid_actions:
                    raise_amount = random.choice([total_pot // 4, total_pot // 2, total_pot])
                    return PlayerAction.RAISE, current_bet + raise_amount
            
            if PlayerAction.CALL in valid_actions:
                return PlayerAction.CALL, 0
            else:
                return PlayerAction.CHECK, 0
        
        elif hand_strength < 0.8:  # Good hand
            if PlayerAction.RAISE in valid_actions:
                raise_amount = random.choice([total_pot // 2, total_pot, total_pot * 2])
                return PlayerAction.RAISE, current_bet + raise_amount
            else:
                if PlayerAction.CALL in valid_actions:
                    return PlayerAction.CALL, 0
                else:
                    return PlayerAction.CHECK, 0
        
        else:  # Premium hand
            if PlayerAction.ALL_IN in valid_actions and random.random() < 0.3:
                return PlayerAction.ALL_IN, 0
            elif PlayerAction.RAISE in valid_actions:
                # Big value bet
                raise_amount = random.choice([total_pot, total_pot * 2, player.chips // 2])
                return PlayerAction.RAISE, current_bet + raise_amount
            else:
                return PlayerAction.CALL, 0
    
    def get_chat_message(self, action: PlayerAction, game_state: Dict[str, Any]) -> Optional[str]:
        messages = {
            PlayerAction.FOLD: ["This one time I'll fold.", "Even I have limits!", "🙄", "Saving my chips for the next bluff."],
            PlayerAction.CALL: ["I'll just call... for now.", "Setting up for next street.", "😏"],
            PlayerAction.RAISE: ["Let's make this interesting!", "I'm feeling lucky!", "🎰", "Go big or go home!", "Who wants to dance?"],
            PlayerAction.ALL_IN: ["ALL IN BABY!", "YOLO!", "🚀", "This is how we do it!", "Let's see what you're made of!"],
            PlayerAction.CHECK: ["I'll slow play this one.", "Just checking... for now.", "😈"]
        }
        
        if random.random() < 0.6:  # 60% chance to chat (Will is very talkative)
            return random.choice(messages.get(action, []))
        return None
    
    def update_memory(self, event: Dict[str, Any]):
        super().update_memory(event)
        # Increase tilt factor after losses
        if event.get('event_type') == 'hand_result' and event.get('winner_id') != self.name:
            self.tilt_factor = min(0.3, self.tilt_factor + 0.05)
        elif event.get('event_type') == 'hand_result' and event.get('winner_id') == self.name:
            self.tilt_factor = max(0.0, self.tilt_factor - 0.02)


class BalancedBenny(BotPersonality):
    """
    Balanced Benny - Mixed playing style that adapts to opponents and table dynamics
    Studies opponent patterns and adjusts strategy accordingly
    """
    
    def __init__(self):
        super().__init__(
            name="Balanced Benny",
            description="Adaptive player who adjusts to table dynamics"
        )
        self.base_aggression = 0.5
        self.bluff_frequency = 0.2
        self.adaptation_factor = 0.0  # Adjusts based on opponent behavior
        
    async def decide_action(self, game_state: Dict[str, Any], player: Player, valid_actions: List[PlayerAction]) -> Tuple[PlayerAction, int]:
        await asyncio.sleep(random.uniform(2.0, 4.5))  # Takes time to analyze
        
        # Calculate hand strength
        all_cards = player.hole_cards + game_state.get("community_cards", [])
        hand_strength = get_hand_strength(all_cards) if all_cards else get_hand_strength(player.hole_cards)
        
        # Analyze table dynamics
        self._analyze_opponents(game_state)
        
        current_bet = game_state.get('current_bet', 0)
        total_pot = game_state.get('pot_total', 0)
        phase = game_state.get('phase', 'pre_flop')
        
        # Adjust strategy based on position and game phase
        position_factor = self._get_position_factor(game_state, player)
        phase_factor = self._get_phase_factor(phase)
        
        # Adaptive aggression based on table dynamics
        effective_aggression = self.base_aggression + self.adaptation_factor + position_factor + phase_factor
        effective_aggression = max(0.1, min(0.9, effective_aggression))
        
        # Balanced decision making
        if hand_strength < 0.25:  # Weak hands
            # Occasionally bluff in position
            if random.random() < (self.bluff_frequency * position_factor) and PlayerAction.RAISE in valid_actions:
                bluff_size = total_pot // 3
                return PlayerAction.RAISE, current_bet + bluff_size
            
            if PlayerAction.CHECK in valid_actions:
                return PlayerAction.CHECK, 0
            else:
                return PlayerAction.FOLD, 0
        
        elif hand_strength < 0.5:  # Medium hands
            if random.random() < effective_aggression and PlayerAction.RAISE in valid_actions:
                raise_amount = total_pot // 2
                return PlayerAction.RAISE, current_bet + raise_amount
            elif PlayerAction.CALL in valid_actions:
                return PlayerAction.CALL, 0
            else:
                return PlayerAction.CHECK, 0
        
        elif hand_strength < 0.8:  # Good hands
            if PlayerAction.RAISE in valid_actions:
                # Value betting
                raise_amount = min(total_pot * 0.6, player.chips // 3)
                return PlayerAction.RAISE, current_bet + raise_amount
            else:
                if PlayerAction.CALL in valid_actions:
                    return PlayerAction.CALL, 0
                else:
                    return PlayerAction.CHECK, 0
        
        else:  # Premium hands
            if PlayerAction.RAISE in valid_actions:
                # Maximize value
                raise_amount = min(total_pot, player.chips // 2)
                return PlayerAction.RAISE, current_bet + raise_amount
            elif PlayerAction.ALL_IN in valid_actions and random.random() < 0.15:
                return PlayerAction.ALL_IN, 0
            else:
                return PlayerAction.CALL, 0
    
    def _analyze_opponents(self, game_state: Dict[str, Any]):
        """Analyze opponent behavior and adjust adaptation factor"""
        # Count aggressive vs passive actions from memory
        if 'opponent_actions' in self.memory:
            recent_actions = self.memory['opponent_actions'][-20:]  # Last 20 actions
            aggressive_count = sum(1 for action in recent_actions 
                                 if action.get('action') in ['raise', 'all_in'])
            total_actions = len(recent_actions)
            
            if total_actions > 5:
                table_aggression = aggressive_count / total_actions
                # Adapt: if table is aggressive, play tighter; if passive, play looser
                self.adaptation_factor = (0.5 - table_aggression) * 0.3
    
    def _get_position_factor(self, game_state: Dict[str, Any], player: Player) -> float:
        """Calculate position-based adjustment factor"""
        # Simplified position analysis - late position is more aggressive
        players = game_state.get('players', [])
        active_players = [p for p in players if not p.get('is_folded', False)]
        
        if len(active_players) <= 3:
            return 0.1  # Late position bonus
        elif len(active_players) <= 6:
            return 0.0  # Middle position
        else:
            return -0.1  # Early position penalty
    
    def _get_phase_factor(self, phase: str) -> float:
        """Calculate game phase adjustment factor"""
        phase_adjustments = {
            'pre_flop': -0.1,  # More conservative pre-flop
            'flop': 0.0,       # Neutral on flop
            'turn': 0.1,       # Slightly more aggressive on turn
            'river': 0.2       # More aggressive on river
        }
        return phase_adjustments.get(phase, 0.0)
    
    def get_chat_message(self, action: PlayerAction, game_state: Dict[str, Any]) -> Optional[str]:
        messages = {
            PlayerAction.FOLD: ["Reading the table dynamics.", "Not the right spot.", "📚"],
            PlayerAction.CALL: ["Floating to see what develops.", "I'll see where this goes.", "🎯"],
            PlayerAction.RAISE: ["Time to apply some pressure.", "Building this pot.", "⚖️", "Adjusting my play."],
            PlayerAction.CHECK: ["Controlling the pot size.", "Playing it smart.", "🧠"],
            PlayerAction.ALL_IN: ["This is the perfect spot!", "Maximum value here.", "💎"]
        }
        
        if random.random() < 0.25:  # 25% chance to chat (professional but not chatty)
            return random.choice(messages.get(action, []))
        return None


# Bot factory for easy creation
BOT_PERSONALITIES = {
    'carla': CalculatingCarla,
    'tom': TimidTom, 
    'will': WildWill,
    'benny': BalancedBenny
}

def create_bot_player(bot_type: str, user_id: int, chips: int) -> Tuple[Player, BotPersonality]:
    """Create a bot player with specified personality"""
    if bot_type not in BOT_PERSONALITIES:
        bot_type = 'benny'  # Default to balanced
    
    personality_class = BOT_PERSONALITIES[bot_type]
    personality = personality_class()
    
    player = Player(
        user_id=user_id,
        name=personality.name,
        chips=chips,
        is_bot=True
    )
    
    return player, personality


async def get_bot_action(personality: BotPersonality, game_state: Dict[str, Any], 
                        player: Player, valid_actions: List[PlayerAction]) -> Tuple[PlayerAction, int, Optional[str]]:
    """Get bot action and optional chat message"""
    action, amount = await personality.decide_action(game_state, player, valid_actions)
    chat_message = personality.get_chat_message(action, game_state)
    return action, amount, chat_message