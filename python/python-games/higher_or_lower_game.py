import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption
import time # Added for memory management
from typing import Dict, Optional
import weakref
from collections import deque
import asyncio

from utils.database import get_user, parse_bet
from utils import embeds
from utils.views import HigherOrLowerView
from utils.cards import Card, Deck
from utils.game import Game
from utils.constants import XP_PER_PROFIT, GUILD_ID

# Memory-optimized active games storage
active_games: Dict[int, "HigherOrLowerGameLogic"] = {}

# Activity tracking for cleanup optimization
game_activity: deque = deque(maxlen=50)

# Payout multiplier increases with each correct guess
STREAK_MULTIPLIERS = [0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0, 7.0, 10.0]

class HigherOrLowerGameLogic(Game):
    """Optimized Higher or Lower game with memory management and lazy evaluation."""
    
    def __init__(self, interaction: Interaction, bet: int):
        super().__init__(interaction, bet)
        self._deck = None  # Lazy initialization
        self.current_card: Optional[Card] = None
        self.next_card: Optional[Card] = None
        self.streak = 0
        self.view: Optional[HigherOrLowerView] = None
        self.message: Optional[nextcord.Message] = None
        self.created_at = time.time()  # For cleanup
        self._cached_multiplier: Optional[float] = None  # Cache multiplier calculation
        self._cached_winnings: Optional[int] = None  # Cache winnings calculation
    
    @property
    def deck(self) -> Deck:
        """Lazy initialization of deck to save memory until needed."""
        if self._deck is None:
            self._deck = Deck(game='poker')  # Use poker values (Ace high)
        return self._deck
    
    def _invalidate_cache(self):
        """Invalidate cached calculations when streak changes."""
        self._cached_multiplier = None
        self._cached_winnings = None

    @property
    def current_multiplier(self) -> float:
        """Cached multiplier calculation."""
        if self._cached_multiplier is None:
            if self.streak == 0:
                self._cached_multiplier = 0
            else:
                self._cached_multiplier = STREAK_MULTIPLIERS[min(self.streak - 1, len(STREAK_MULTIPLIERS) - 1)]
        return self._cached_multiplier

    @property
    def current_winnings(self) -> int:
        """Cached winnings calculation."""
        if self._cached_winnings is None:
            if self.streak == 0:
                self._cached_winnings = 0
            else:
                self._cached_winnings = self.bet + int(self.bet * self.current_multiplier)
        return self._cached_winnings
    
    def cleanup_memory(self):
        """Clean up memory-intensive objects."""
        self._deck = None
        self.current_card = None
        self.next_card = None
        self.view = None
        self._cached_multiplier = None
        self._cached_winnings = None

    async def start_game(self):
        self.current_card = self.deck.deal()
        self.view = HigherOrLowerView(self.user, self)

        embed = await embeds.create_higher_or_lower_embed(
            state='playing', current_card=str(self.current_card),
            bet=self.bet, streak=self.streak,
            winnings=self.current_winnings, multiplier=self.current_multiplier
        )

        # We have already deferred in the slash command; use followup to send the game message.
        self.message = await self.interaction.followup.send(embed=embed, view=self.view)
        self.view.message = self.message

    async def handle_guess(self, interaction: Interaction, guess: str):
        """Optimized guess handling with cache invalidation."""
        self.next_card = self.deck.deal()
        
        correct = (guess == "higher" and self.next_card.value > self.current_card.value) or \
                  (guess == "lower" and self.next_card.value < self.current_card.value)
        
        if self.next_card.value == self.current_card.value:
            self.current_card = self.next_card
            embed = await embeds.create_higher_or_lower_embed(
                state='playing', current_card=str(self.current_card),
                bet=self.bet, streak=self.streak,
                winnings=self.current_winnings, multiplier=self.current_multiplier
            )
            await interaction.response.edit_message(content="It's a tie! The streak continues.", embed=embed, view=self.view)
            return

        if correct:
            self.streak += 1
            self._invalidate_cache()  # Clear cached calculations
            self.current_card = self.next_card
            if self.view:
                self.view.update_button_state()
            
            embed = await embeds.create_higher_or_lower_embed(
                state='playing', current_card=str(self.current_card),
                bet=self.bet, streak=self.streak, winnings=self.current_winnings,
                multiplier=self.current_multiplier
            )
            await interaction.response.edit_message(content="You guessed correctly!", embed=embed, view=self.view)
            
            # Record activity for cleanup optimization
            game_activity.append(("guess_correct", time.time()))
        else:
            await self.end_game(interaction, lost=True)
            game_activity.append(("game_lost", time.time()))

    async def cash_out(self, interaction: Interaction):
        await self.end_game(interaction, cashed_out=True)

    async def handle_timeout(self):
        if self.user.id in active_games:
            profit = self.current_winnings - self.bet if self.streak > 0 else 0
            await self.end_game(self.interaction, cashed_out=True, profit_override=profit)

    async def end_game(self, interaction: Interaction, lost: bool = False, cashed_out: bool = False, profit_override: int = None):
        if self.is_game_over: return
        for item in self.view.children: item.disabled = True

        profit = 0
        outcome_text = ""
        
        if profit_override is not None:
            profit = profit_override
            outcome_text = f"Game timed out. Cashed out with {self.current_winnings:,}." if profit > 0 else "Game timed out. Your bet was refunded."
        elif lost:
            profit = -self.bet
            outcome_text = f"You lost! The next card was `{self.next_card}`."
        elif cashed_out:
            profit = self.current_winnings - self.bet
            outcome_text = f"Cashed out with {self.current_winnings:,} chips!"

        await super().end_game(profit=profit)
        
        final_user_data = await get_user(self.user.id)
        xp_gain = int(profit * XP_PER_PROFIT) if profit > 0 else 0

        embed = await embeds.create_higher_or_lower_embed(
            state='final', current_card=str(self.current_card), next_card=str(self.next_card) if self.next_card else None,
            bet=self.bet, streak=self.streak, winnings=self.current_winnings,
            outcome_text=outcome_text, new_balance=final_user_data['chips'],
            xp_gain=xp_gain, user=self.user, multiplier=self.current_multiplier
        )
        
        # If the interaction is from a button press, defer it to prevent "Interaction failed"
        if interaction.type == nextcord.InteractionType.component and not interaction.response.is_done():
            await interaction.response.defer()

        # Use the stored message object to edit, as the interaction token may have expired
        await self.message.edit(embed=embed, view=self.view)

        if self.user.id in active_games: del active_games[self.user.id]
        if self.view:
            self.view.stop()
        
        # Record activity and cleanup memory
        game_activity.append(("game_ended", time.time()))
        self.cleanup_memory()

class HigherOrLowerGame(commands.Cog):
    """Optimized Higher or Lower game cog with dynamic cleanup."""
    
    def __init__(self, bot: commands.Bot):
        self.bot = bot
        self.cleanup_old_games.start()

    def cog_unload(self) -> None:
        self.cleanup_old_games.cancel()
        # Clean up all active games
        for game in list(active_games.values()):
            if hasattr(game, 'cleanup_memory'):
                game.cleanup_memory()
        active_games.clear()

    def _calculate_cleanup_interval(self) -> int:
        """Calculate optimal cleanup interval based on game activity."""
        if len(game_activity) < 5:
            return 5  # Default
        
        current_time = time.time()
        recent_activity = sum(1 for _, timestamp in game_activity 
                            if current_time - timestamp < 300)  # Last 5 minutes
        
        if recent_activity > 10:
            return 2  # High activity
        elif recent_activity > 5:
            return 3  # Medium activity  
        elif recent_activity < 2:
            return 10  # Low activity
        else:
            return 5  # Normal activity

    @tasks.loop(minutes=5)  # Initial interval, will be dynamically adjusted
    async def cleanup_old_games(self) -> None:
        """Optimized cleanup with dynamic scheduling and memory management."""
        current_time = time.time()
        to_remove = []
        
        for user_id, game in list(active_games.items()):
            # Only cleanup actual game objects, not boolean locks
            if hasattr(game, 'created_at') and (current_time - game.created_at) > 600: # 10 minutes = 600 seconds
                to_remove.append(user_id)
                if hasattr(game, 'cleanup_memory'):
                    game.cleanup_memory()
        
        for user_id in to_remove:
            active_games.pop(user_id, None)
        
        if to_remove:
            print(f"Cleaned up {len(to_remove)} old higher/lower games")
            game_activity.append(("cleanup", current_time))
        
        # Adjust cleanup interval dynamically
        new_interval = self._calculate_cleanup_interval()
        self.cleanup_old_games.change_interval(minutes=new_interval)

    @cleanup_old_games.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(name="horl", description="Play a game of Higher or Lower.")
    async def higher_or_lower(self, interaction: Interaction, bet: str = SlashOption(description="Chips to wager.", required=True)):
        if interaction.user.id in active_games:
            await interaction.response.send_message(embed=embeds.error_embed("You already have an active game."), ephemeral=True)
            return

        try:
            user_data = await get_user(interaction.user.id)
            bet_amount = await parse_bet(bet, user_data['chips'])
        except ValueError as e:
            await interaction.response.send_message(embed=embeds.error_embed(str(e)), ephemeral=True)
            return

        game = HigherOrLowerGameLogic(interaction, bet_amount)
        if not await game.validate_bet():
            return
            
        active_games[interaction.user.id] = game
        await interaction.response.defer()
        await game.start_game()

def setup(bot: commands.Bot):
    bot.add_cog(HigherOrLowerGame(bot))
