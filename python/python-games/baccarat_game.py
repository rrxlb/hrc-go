import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption
import time
from typing import List, Dict, Any

from utils.database import get_user, parse_bet
from utils import embeds
from utils.views import BaccaratView
from utils.cards import Card, Deck
from utils.game import Game
from utils.constants import (
    DECK_COUNT, SHUFFLE_THRESHOLD, XP_PER_PROFIT, GUILD_ID,
    BACCARAT_PAYOUT, BACCARAT_TIE_PAYOUT, BACCARAT_BANKER_COMMISSION
)

active_games: Dict[int, "BaccaratGameLogic"] = {}

class BaccaratGameLogic(Game):
    def __init__(self, interaction: Interaction, bet: int, choice: str):
        super().__init__(interaction, bet)
        self.choice = choice
        self.deck = Deck(num_decks=DECK_COUNT, game='baccarat')
        self.player_hand: List[Card] = []
        self.banker_hand: List[Card] = []
        self.player_score = 0
        self.banker_score = 0
        self.created_at = time.time()  # For cleanup

    def _get_baccarat_value(self, card: Card) -> int:
        if card.rank in ['J', 'Q', 'K', '10']: return 0
        if card.rank == 'A': return 1
        return int(card.rank)

    def _update_scores(self):
        self.player_score = sum(self._get_baccarat_value(c) for c in self.player_hand) % 10
        self.banker_score = sum(self._get_baccarat_value(c) for c in self.banker_hand) % 10

    def run_game(self) -> Dict[str, Any]:
        self.player_hand.extend([self.deck.deal(), self.deck.deal()])
        self.banker_hand.extend([self.deck.deal(), self.deck.deal()])
        self._update_scores()

        player_draws = False
        if self.player_score < 8 and self.banker_score < 8:
            if self.player_score <= 5:
                self.player_hand.append(self.deck.deal())
                player_draws = True
            
            player_third_card_value = self._get_baccarat_value(self.player_hand[2]) if player_draws else -1
            
            if not player_draws and self.banker_score <= 5:
                self.banker_hand.append(self.deck.deal())
            elif player_draws:
                if self.banker_score <= 2 or \
                  (self.banker_score == 3 and player_third_card_value != 8) or \
                  (self.banker_score == 4 and player_third_card_value in [2,3,4,5,6,7]) or \
                  (self.banker_score == 5 and player_third_card_value in [4,5,6,7]) or \
                  (self.banker_score == 6 and player_third_card_value in [6,7]):
                    self.banker_hand.append(self.deck.deal())
            
            self._update_scores()

        if self.player_score > self.banker_score: winner = 'player'
        elif self.banker_score > self.player_score: winner = 'banker'
        else: winner = 'tie'

        profit = 0
        if self.choice == winner:
            if winner == 'player': profit = self.bet * BACCARAT_PAYOUT
            elif winner == 'banker': profit = (self.bet * BACCARAT_PAYOUT) * (1 - BACCARAT_BANKER_COMMISSION)
            elif winner == 'tie': profit = self.bet * BACCARAT_TIE_PAYOUT
        elif winner == 'tie' and self.choice in ['player', 'banker']:
            profit = 0 # Push
        else:
            profit = -self.bet

        return {
            "player_hand": [str(c) for c in self.player_hand], "banker_hand": [str(c) for c in self.banker_hand],
            "player_score": self.player_score, "banker_score": self.banker_score,
            "result_text": f"{winner.title()} wins!" if winner != 'tie' else "It's a Tie!",
            "profit": profit,
        }

class BaccaratGame(commands.Cog):
    def __init__(self, bot: commands.Bot) -> None:
        self.bot = bot
        self.cleanup_old_games.start()

    def cog_unload(self) -> None:
        self.cleanup_old_games.cancel()

    @tasks.loop(minutes=30)  # Run every 30 minutes
    async def cleanup_old_games(self) -> None:
        """Remove games older than 1 hour to prevent memory leaks."""
        current_time = time.time()
        to_remove = []
        
        for user_id, game in list(active_games.items()):
            # Only cleanup actual game objects, not boolean locks
            if hasattr(game, 'created_at') and (current_time - game.created_at) > 3600:
                to_remove.append(user_id)
        
        for user_id in to_remove:
            game_to_remove = active_games.pop(user_id, None)
            if game_to_remove:
                # Get bet amount
                bet_amount = game_to_remove.bet if hasattr(game_to_remove, 'bet') else 0
                
                # Create cleanup embed with chip forfeiture message
                cleanup_embed = embeds.create_game_cleanup_embed(bet_amount)
                
                # Disable all buttons and update message
                if hasattr(game_to_remove, 'view') and game_to_remove.view:
                    for item in game_to_remove.view.children:
                        item.disabled = True
                    
                    try:
                        await game_to_remove.view.message.edit(embed=cleanup_embed, view=game_to_remove.view)
                    except (nextcord.HTTPException, nextcord.NotFound):
                        pass  # Message was deleted or couldn't be edited
                    
                    try:
                        game_to_remove.view.stop()
                    except Exception:
                        pass  # Ignore errors during view stop
        
        if to_remove:
            print(f"Cleaned up {len(to_remove)} old baccarat games")

    @cleanup_old_games.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(name="baccarat", description="Start a game of Baccarat.")
    async def baccarat(self, interaction: Interaction, bet: str = SlashOption(description="Chips to wager.", required=True)):
        if interaction.user.id in active_games:
            await interaction.response.send_message(embed=embeds.error_embed("You already have an active game."), ephemeral=True)
            return

        try:
            user_data = await get_user(interaction.user.id)
            bet_amount = await parse_bet(bet, user_data['chips'])
        except ValueError as e:
            await interaction.response.send_message(embed=embeds.error_embed(str(e)), ephemeral=True)
            return
        
        # We need a dummy Game object for the initial bet validation
        temp_game = Game(interaction, bet_amount)
        if not await temp_game.validate_bet():
            return
        
        active_games[interaction.user.id] = True # Lock
        
        view = BaccaratView(interaction.user, bet_amount, active_games)
        embed = await embeds.create_baccarat_embed(game_over=False, bet=bet_amount)
        await interaction.response.send_message(embed=embed, view=view)
        view.message = await interaction.original_message()
        
def setup(bot: commands.Bot):
    bot.add_cog(BaccaratGame(bot))
