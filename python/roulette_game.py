import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption, Color
from nextcord.ui import Modal, TextInput
import random
import asyncio
import time # Added for memory management
from typing import Dict, Any

from utils.database import get_user, parse_bet
from utils import embeds
from utils.views import RouletteView
from utils.game import Game
from utils.constants import XP_PER_PROFIT, GUILD_ID

# --- Game Constants ---
RED_NUMBERS = {1, 3, 5, 7, 9, 12, 14, 16, 18, 19, 21, 23, 25, 27, 30, 32, 34, 36}
BLACK_NUMBERS = {2, 4, 6, 8, 10, 11, 13, 15, 17, 20, 22, 24, 26, 28, 29, 31, 33, 35}
GREEN_NUMBER = 0
EVEN_MONEY_BETS = {"red", "black", "odd", "even", "1-18", "19-36"}

PAYOUTS = {
    "red": 1, "black": 1, "odd": 1, "even": 1,
    "1-18": 1, "19-36": 1,
    "col1": 2, "col2": 2, "col3": 2,
    "dozen1": 2, "dozen2": 2, "dozen3": 2,
    "single": 35
}

# --- Game State Management ---
active_games: Dict[int, "RouletteGameLogic"] = {}

# --- UI Components ---
class BettingModal(Modal):
    def __init__(self, bet_type: str, game_logic: "RouletteGameLogic"):
        super().__init__(f"Place Bet on {bet_type.title()}")
        self.bet_type = bet_type
        self.game_logic = game_logic

        self.bet_amount_input = TextInput(
            label=f"Amount for {bet_type.replace('_', ' ').title()}",
            placeholder="Enter chips amount or 'all'/'half'",
            required=True,
        )
        self.add_item(self.bet_amount_input)

    async def callback(self, interaction: Interaction):
        await self.game_logic.handle_new_bet(interaction, self.bet_type, self.bet_amount_input.value)

# --- Game Logic ---
class RouletteGameLogic(Game):
    def __init__(self, interaction: Interaction):
        # Initialize Game with a dummy bet of 0, as bets are placed later
        super().__init__(interaction, 0)
        self.view = RouletteView(self.user, self)
        self.bets: Dict[str, int] = {}
        self.result: Dict[str, Any] = {}
        self.message: nextcord.Message = None
        self.created_at = time.time()  # For cleanup

    async def start_game(self):
        # Get user data at the start
        self.user_data = await get_user(self.user.id)
        active_games[self.user.id] = self
        embed = await embeds.create_roulette_embed(state='betting', bets=self.bets)
        await self.interaction.response.send_message(embed=embed, view=self.view)
        self.message = await self.interaction.original_message()

    async def handle_new_bet(self, interaction: Interaction, bet_type: str, amount_str: str):
        try:
            # Calculate total current bets to check against balance
            total_current_bets = sum(self.bets.values())
            available_chips = self.user_data['chips'] - total_current_bets
            amount = await parse_bet(amount_str, available_chips)
        except ValueError as e:
            await interaction.response.send_message(embed=embeds.error_embed(str(e)), ephemeral=True)
            return

        # Check if the user can cover the new bet plus existing bets
        total_required = total_current_bets + amount
        if self.user_data['chips'] < total_required:
            await interaction.response.send_message(
                embed=embeds.insufficient_chips_embed(
                    required_chips=total_required,
                    current_balance=self.user_data['chips'],
                    bet_description=f"that bet ({amount:,} chips on {bet_type.replace('_', ' ').title()})"
                ), 
                ephemeral=True
            )
            return
        
        if bet_type in self.bets:
            # Overriding a bet doesn't require a refund now, just updating the dictionary
            self.bets[bet_type] = amount
        else:
            self.bets[bet_type] = amount
        
        embed = await embeds.create_roulette_embed(state='betting', bets=self.bets)
        await interaction.response.edit_message(embed=embed, view=self.view)

    async def place_bet(self, interaction: Interaction, bet_type: str):
        modal = BettingModal(bet_type, self)
        await interaction.response.send_modal(modal)

    async def spin_wheel(self, interaction: Interaction):
        if not self.bets:
            await interaction.response.send_message("You must place at least one bet before spinning.", ephemeral=True)
            return

        for item in self.view.children:
            item.disabled = True
        
        embed = await embeds.create_roulette_embed(state='spinning', bets=self.bets)
        await interaction.response.edit_message(embed=embed, view=self.view)
        
        await asyncio.sleep(2)

        winning_number = random.randint(0, 36)
        
        if winning_number in RED_NUMBERS:
            winning_color = "red"
        elif winning_number in BLACK_NUMBERS:
            winning_color = "black"
        else:
            winning_color = "green"
            
        self.result = {"number": winning_number, "color": winning_color}
        await self.end_game()

    async def end_game(self):
        if self.is_game_over:
            return
            
        total_profit = 0
        total_wagered = sum(self.bets.values())
        
        num = self.result["number"]

        # La Partage Rule: If zero hits, half is lost on even-money bets
        if num == 0:
            for bet_type, bet_amount in self.bets.items():
                if bet_type in EVEN_MONEY_BETS:
                    total_profit -= bet_amount / 2  # Lose half
                else:
                    total_profit -= bet_amount # Lose full
        else:
            for bet_type, bet_amount in self.bets.items():
                is_win = False
                payout_multiplier = PAYOUTS.get(bet_type, 1)
                
                if bet_type == "red" and self.result["color"] == "red": is_win = True
                elif bet_type == "black" and self.result["color"] == "black": is_win = True
                elif bet_type == "odd" and num % 2 != 0: is_win = True
                elif bet_type == "even" and num % 2 == 0: is_win = True
                elif bet_type == "1-18" and 1 <= num <= 18: is_win = True
                elif bet_type == "19-36" and 19 <= num <= 36: is_win = True
                elif bet_type == "dozen1" and 1 <= num <= 12: is_win = True
                elif bet_type == "dozen2" and 13 <= num <= 24: is_win = True
                elif bet_type == "dozen3" and 25 <= num <= 36: is_win = True
                elif bet_type == "col1" and num % 3 == 1: is_win = True
                elif bet_type == "col2" and num % 3 == 2: is_win = True
                elif bet_type == "col3" and num % 3 == 0: is_win = True
                elif bet_type.startswith("single_") and int(bet_type.split('_')[1]) == num: is_win = True

                if is_win:
                    total_profit += bet_amount * payout_multiplier
                else:
                    total_profit -= bet_amount

        # Use parent class to handle DB update
        await super().end_game(profit=total_profit)
        
        final_user_data = await get_user(self.user.id)
        new_balance = final_user_data['chips']
        xp_gain = int(total_profit * XP_PER_PROFIT) if total_profit > 0 else 0

        embed = await embeds.create_roulette_embed(
            state='final',
            bets=self.bets,
            result=self.result,
            new_balance=new_balance,
            profit=total_profit,
            xp_gain=xp_gain,
            user=self.user
        )
        await self.message.edit(embed=embed, view=None)

        if self.user.id in active_games:
            del active_games[self.user.id]
        self.view.stop()

# --- Cog Class ---
class RouletteGame(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot
        self.cleanup_old_games.start()

    def cog_unload(self) -> None:
        self.cleanup_old_games.cancel()

    @tasks.loop(minutes=5)  # Run every 5 minutes
    async def cleanup_old_games(self) -> None:
        """Remove games older than 10 minutes to prevent memory leaks."""
        current_time = time.time()
        to_remove = []
        
        for user_id, game in list(active_games.items()):
            # Only cleanup actual game objects, not boolean locks
            if hasattr(game, 'created_at') and (current_time - game.created_at) > 600: # 10 minutes = 600 seconds
                to_remove.append(user_id)
        
        for user_id in to_remove:
            game_to_remove = active_games.pop(user_id, None)
            if game_to_remove:
                # Calculate total bet amount
                total_bet = sum(game_to_remove.bets.values()) if hasattr(game_to_remove, 'bets') else 0
                
                # Create cleanup embed with chip forfeiture message
                cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
                
                # Disable all buttons and update message
                if hasattr(game_to_remove, 'view') and game_to_remove.view:
                    for item in game_to_remove.view.children:
                        item.disabled = True
                    
                    try:
                        await game_to_remove.message.edit(embed=cleanup_embed, view=game_to_remove.view)
                    except (nextcord.HTTPException, nextcord.NotFound):
                        pass  # Message was deleted or couldn't be edited
                    
                    try:
                        game_to_remove.view.stop()
                    except Exception:
                        pass  # Ignore errors during view stop
        
        if to_remove:
            print(f"Cleaned up {len(to_remove)} old roulette games")

    @cleanup_old_games.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(
        name="roulette",
        description="Play a game of Roulette.",
    )
    async def roulette(self, interaction: Interaction):
        player_id = interaction.user.id

        if player_id in active_games:
            await interaction.response.send_message(embed=embeds.error_embed("You already have an active game of Roulette."), ephemeral=True)
            return
        
        game = RouletteGameLogic(interaction)
        await game.start_game()

def setup(bot: commands.Bot):
    bot.add_cog(RouletteGame(bot))
