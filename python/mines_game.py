import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption, ButtonStyle
from nextcord.ui import View, Button
import random
import time # Added for memory management
from typing import List, Dict

from utils import database as db
from utils import embeds
from utils.views import BaseGameView
from utils.game import Game
from utils.constants import GUILD_ID, XP_PER_PROFIT

active_games: Dict[int, "MinesGameLogic"] = {}

PAYOUT_MULTIPLIERS = {
    1: 0.03, 2: 0.07, 3: 0.11, 4: 0.15, 5: 0.20, 6: 0.25, 7: 0.30, 8: 0.36,
    9: 0.43, 10: 0.50, 11: 0.58, 12: 0.67, 13: 0.77, 14: 0.88, 15: 1.00,
    16: 1.14, 17: 1.31, 18: 1.50, 19: 1.73, 20: 2.00, 21: 2.33, 22: 2.75,
    23: 3.33, 24: 4.00
}

class Tile:
    def __init__(self, row: int, col: int):
        self.row, self.col = row, col
        self.is_mine = self.is_revealed = False

class TileButton(Button):
    def __init__(self, tile: Tile, row: int):
        super().__init__(style=ButtonStyle.secondary, label="⬛", row=row)
        self.tile = tile

    async def callback(self, interaction: Interaction):
        game: MinesGameLogic = self.view.game_logic
        await game.reveal_tile(interaction, self.tile.row, self.tile.col)

class CashoutButton(Button):
    def __init__(self, row: int):
        super().__init__(style=ButtonStyle.success, label="Cash Out", custom_id="cash_out", row=row)

    async def callback(self, interaction: Interaction):
        game: MinesGameLogic = self.view.game_logic
        await game.cash_out(interaction)

class MinesView(BaseGameView):
    def __init__(self, author: nextcord.User, game_logic: "MinesGameLogic"):
        super().__init__(author, game_logic)
        self._add_buttons()

    def _add_buttons(self):
        for r, row_tiles in enumerate(self.game_logic.grid):
            for tile in row_tiles:
                self.add_item(TileButton(tile, r))
        self.add_item(CashoutButton(row=4))
        self.update_button_states()

    def update_button_states(self):
        for child in self.children:
            if isinstance(child, TileButton):
                tile = child.tile
                if tile.is_revealed:
                    child.disabled = True
                    child.label, child.style = ("💎", ButtonStyle.primary) if not tile.is_mine else ("💣", ButtonStyle.danger)
            elif isinstance(child, CashoutButton):
                child.disabled = self.game_logic.revealed_gems == 0

    async def on_timeout(self):
        await self.game_logic.handle_timeout()

class MinesGameLogic(Game):
    def __init__(self, interaction: Interaction, bet: int, mine_count: int):
        super().__init__(interaction, bet)
        self.mine_count = mine_count
        self.grid: List[List[Tile]] = [[Tile(r, c) for c in range(5)] for r in range(4)]
        self.revealed_gems = 0
        self.view = MinesView(self.user, self)
        self.created_at = time.time()  # For cleanup
        self._place_mines()

    def _place_mines(self):
        placed = 0
        while placed < self.mine_count:
            tile = self.grid[random.randint(0, 3)][random.randint(0, 4)]
            if not tile.is_mine:
                tile.is_mine = True
                placed += 1

    @property
    def current_multiplier(self) -> float:
        if self.revealed_gems == 0:
            return 1.0
        return round(1 + (PAYOUT_MULTIPLIERS.get(self.mine_count, 0.01) * self.revealed_gems), 2)

    @property
    def current_winnings(self) -> int:
        if self.revealed_gems == 0:
            return 0
        return int(self.bet * self.current_multiplier)

    async def start_game(self):
        embed = await embeds.create_mines_embed(self, 'playing')
        await self.interaction.followup.send(embed=embed, view=self.view)
        self.view.message = await self.interaction.original_message()

    async def reveal_tile(self, interaction: Interaction, row: int, col: int):
        if self.is_game_over: return
        tile = self.grid[row][col]
        if tile.is_revealed: return
        tile.is_revealed = True

        if tile.is_mine:
            await self.end_game(interaction, lost=True, reason="You hit a mine!")
        else:
            self.revealed_gems += 1
            if self.revealed_gems >= (20 - self.mine_count):
                await self.end_game(interaction, lost=False, reason="You found all the gems!")
            else:
                self.view.update_button_states()
                embed = await embeds.create_mines_embed(self, 'playing')
                await interaction.response.edit_message(embed=embed, view=self.view)

    async def cash_out(self, interaction: Interaction):
        await self.end_game(interaction, lost=False, reason="You cashed out.")

    async def end_game(self, interaction: Interaction, lost: bool, reason: str):
        if self.is_game_over: return
        self.is_game_over = True

        profit = -self.bet if lost else self.current_winnings - self.bet
        await super().end_game(profit=profit)
        
        for row in self.grid:
            for tile in row: tile.is_revealed = True
        
        self.view.update_button_states()
        for child in self.view.children:
            if isinstance(child, Button): child.disabled = True

        final_user_data = await db.get_user(self.user.id)
        xp_gain = int(profit * XP_PER_PROFIT) if profit > 0 else 0
        
        embed = await embeds.create_mines_embed(
            self, 'final', outcome_text=reason, profit=profit, xp_gain=xp_gain,
            new_balance=final_user_data['chips'], user=self.user
        )
        await interaction.response.edit_message(embed=embed, view=self.view)
        
        if self.user.id in active_games: del active_games[self.user.id]
        self.view.stop()

    async def handle_timeout(self):
        if self.is_game_over or not self.view.message: return
        # In case of timeout, refund the bet by setting profit to 0
        profit = -self.bet
        await super().end_game(profit=profit)
        
        for row in self.grid:
            for tile in row: tile.is_revealed = True
        
        self.view.update_button_states()
        for child in self.view.children:
            if isinstance(child, Button): child.disabled = True

        final_user_data = await db.get_user(self.user.id)
        
        embed = await embeds.create_mines_embed(
            self, 'final', outcome_text="Game timed out. Your bet was refunded.", profit=profit, xp_gain=0,
            new_balance=final_user_data['chips'], user=self.user
        )
        if self.view.message:
            await self.view.message.edit(embed=embed, view=self.view)
        
        if self.user.id in active_games: del active_games[self.user.id]
        self.view.stop()

class MinesGame(commands.Cog):
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
                # Create cleanup embed with chip forfeiture message
                cleanup_embed = embeds.create_game_cleanup_embed(game_to_remove.bet)
                
                # Disable all buttons and update message
                if hasattr(game_to_remove, 'view') and game_to_remove.view:
                    for child in game_to_remove.view.children:
                        if isinstance(child, Button): 
                            child.disabled = True
                    
                    try:
                        await game_to_remove.view.message.edit(embed=cleanup_embed, view=game_to_remove.view)
                    except (nextcord.HTTPException, nextcord.NotFound):
                        pass  # Message was deleted or couldn't be edited
                    
                    try:
                        game_to_remove.view.stop()
                    except Exception:
                        pass  # Ignore errors during view stop
        
        if to_remove:
            print(f"Cleaned up {len(to_remove)} old mines games")

    @cleanup_old_games.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(name="mines", description="Play Mines and uncover gems for cash.")
    async def mines(self, interaction: Interaction, bet: str, mines: int = SlashOption(min_value=1, max_value=19)):
        if interaction.user.id in active_games:
            await interaction.response.send_message(embed=embeds.error_embed("You already have an active game."), ephemeral=True)
            return

        try:
            user_data = await db.get_user(interaction.user.id)
            bet_amount = await db.parse_bet(bet, user_data['chips'])
        except ValueError as e:
            await interaction.response.send_message(embed=embeds.error_embed(str(e)), ephemeral=True)
            return

        game = MinesGameLogic(interaction, bet_amount, mines)
        if not await game.validate_bet():
            return
            
        active_games[interaction.user.id] = game
        await interaction.response.defer()
        await game.start_game()

def setup(bot: commands.Bot):
    bot.add_cog(MinesGame(bot))
