import nextcord
from nextcord.ext import commands
from nextcord import Interaction, SlashOption, ButtonStyle
import random
import asyncio
from typing import List, Dict, Any, Tuple, Optional

from utils.database import get_user, parse_bet
from utils import embeds
from utils.game import Game
from utils.constants import XP_PER_PROFIT
from utils.jackpot import (
    ensure_jackpot_seeded,
    contribute_to_jackpot,
    get_jackpot_amount,
    win_and_reset_jackpot,
)

# --- Constants ---
SYMBOLS = {
    'common': ['🍒', '🍋', '🍊', '🍉'], 'uncommon': ['🔔', '⭐'],
    'rare': ['💎'], 'jackpot': ['🎰']
}
SYMBOL_WEIGHTS = {'common': 0.75, 'uncommon': 0.10, 'rare': 0.13, 'jackpot': 0.02}
PAYOUTS = {
    '🍒': 3, '🍋': 3, '🍊': 3, '🍉': 3, '🔔': 5, '⭐': 5, '💎': 10, '🎰': 15
}
PAYLINES = 5 # Top, Middle, Bottom, and two diagonals
MIN_BET = PAYLINES  # ensure at least 1 chip per payline

# Progressive jackpot config
JACKPOT_SYMBOL = '🎰'
JACKPOT_LOSS_CONTRIBUTION_RATE = 0.10  # 10% of losses added to the progressive jackpot


def get_random_symbol() -> str:
    """Returns a symbol based on weighted rarity."""
    all_symbols, weights = [], []
    for rarity, symbols in SYMBOLS.items():
        weight_per_symbol = SYMBOL_WEIGHTS[rarity] / len(symbols)
        all_symbols.extend(symbols)
        weights.extend([weight_per_symbol] * len(symbols))
    return random.choices(all_symbols, weights=weights, k=1)[0]


def create_reels() -> List[List[str]]:
    return [[get_random_symbol() for _ in range(3)] for _ in range(3)]


def format_reels(reels: List[List[str]]) -> str:
    return "\n".join([" ".join(row) for row in reels])


def _normalize_bet_for_paylines(bet: int, balance: int) -> Tuple[int, Optional[str]]:
    """Clamp bet to balance and down to nearest multiple of PAYLINES. Ensure >= MIN_BET.

    Returns (adjusted_bet, note). If adjusted_bet is 0, caller should error out.
    """
    if bet <= 0 or balance <= 0:
        return 0, None

    original = bet
    # Clamp to available balance first
    bet = min(bet, balance)
    # Round down to nearest multiple of PAYLINES
    adjusted = bet - (bet % PAYLINES)

    if adjusted < MIN_BET:
        return 0, None

    note: Optional[str] = None
    if adjusted != original:
        note = f"Adjusted bet from {original:,} to {adjusted:,} to fit {PAYLINES} paylines."

    return adjusted, note


class SlotsView(nextcord.ui.View):
    """Owner-only 'Spin Again' button that reuses the same bet and message."""
    def __init__(self, user_id: int, bet: int, *, timeout: Optional[float] = 45, message: Optional[nextcord.Message] = None):
        super().__init__(timeout=timeout)
        self.user_id = user_id
        self.bet = bet
        self.message = message

    async def interaction_check(self, interaction: Interaction) -> bool:
        if interaction.user.id != self.user_id:
            await interaction.response.send_message(
                embed=embeds.error_embed("This isn't your game."), ephemeral=True
            )
            return False
        return True

    async def on_timeout(self) -> None:
        # Disable and grey-out all buttons when the view times out
        for item in self.children:
            item.disabled = True
            if isinstance(item, nextcord.ui.Button):
                item.style = ButtonStyle.gray
        try:
            if self.message:
                await self.message.edit(view=self)
        except Exception:
            pass

    @nextcord.ui.button(label="Spin Again", style=ButtonStyle.green)
    async def spin_again(self, _: nextcord.ui.Button, interaction: Interaction):
        await interaction.response.defer()

        # Normalize bet against current balance to avoid fractional line bets
        try:
            user_data = await get_user(interaction.user.id)
        except Exception:
            user_data = {'chips': 0}
        adjusted_bet, note = _normalize_bet_for_paylines(int(self.bet), int(user_data.get('chips', 0)))

        if adjusted_bet == 0:
            await interaction.followup.send(
                embed=embeds.error_embed(f"Bet must be at least {MIN_BET:,} and divisible by {PAYLINES}."),
                ephemeral=True,
            )
            return

        # Proceed only after validation; keep the view if validation fails
        game = SlotsGameLogic(interaction, adjusted_bet, message=interaction.message)
        if not await game.validate_bet():
            return

        # Remove controls from the previous message and run the game
        try:
            await interaction.message.edit(view=None)
        except Exception:
            pass

        # Persist the adjusted bet for subsequent spins
        self.bet = adjusted_bet

        await game.play_game()

        if note:
            try:
                await interaction.followup.send(note, ephemeral=True)
            except Exception:
                pass


class SlotsGameLogic(Game):
    def __init__(self, interaction: Interaction, bet: int, message: Optional[nextcord.Message] = None):
        super().__init__(interaction, bet)
        # Only count wins/losses if bet >= 20% of pre-game chips
        self.count_win_loss_min_ratio = 0.20
        self.reels: List[List[str]] = []
        self.message: Optional[nextcord.Message] = message

    async def play_game(self):
        # Ensure jackpot exists; contribution happens post-result from losses
        await ensure_jackpot_seeded()

        initial_embed = await embeds.create_slots_embed(state='initial')
        if self.message is None:
            await self.interaction.followup.send(embed=initial_embed)
            self.message = await self.interaction.original_message()
        else:
            await self.message.edit(embed=initial_embed, view=None)

        # Decide the final reels first, then animate towards them
        final_reels = create_reels()
        await self._run_spinning_animation(final_reels)

        self.reels = final_reels
        total_winnings, is_jackpot = self._calculate_results()

        # If jackpot line hit, pay out the progressive jackpot
        jackpot_payout = 0
        if is_jackpot:
            jackpot_payout = await win_and_reset_jackpot()
            total_winnings += jackpot_payout

        profit = total_winnings - self.bet

        # Loss-funded jackpot contribution
        if profit < 0:
            loss_amount = -profit
            await contribute_to_jackpot(int(loss_amount * JACKPOT_LOSS_CONTRIBUTION_RATE))

        await self.end_game(profit)

        final_user_data = await get_user(self.user.id)
        xp_gain = int(profit * XP_PER_PROFIT) if profit > 0 else 0

        # Current jackpot after this spin (post contribution/payout)
        current_jackpot = await get_jackpot_amount()

        # Build outcome text
        outcome_text = "No wins this time. Better luck next time!"
        if total_winnings > 0:
            outcome_text = f"Congratulations! You won {total_winnings:,} chips!"
            if is_jackpot:
                outcome_text = f"JACKPOT! You won {total_winnings:,} chips! (+{jackpot_payout:,})"

        final_embed = await embeds.create_slots_embed(
            state='final',
            reels=format_reels(self.reels),
            outcome_text=outcome_text,
            xp_gain=xp_gain,
            new_balance=final_user_data['chips'],
            bet=self.bet,
            user=self.user,
            jackpot_amount=current_jackpot,
        )
        # Attach Spin Again (same bet), and track message on the view for timeout updates
        await self.message.edit(embed=final_embed, view=SlotsView(self.user.id, self.bet, message=self.message))

        if xp_gain > 0:
            old_rank = embeds.get_current_rank(self.user_data['current_xp'])
            new_rank = embeds.get_current_rank(final_user_data['current_xp'])
            if new_rank['name'] != old_rank['name']:
                await self.interaction.followup.send(embed=embeds.rank_up_embed(new_rank['name'], new_rank['icon']), ephemeral=True)

    async def _run_spinning_animation(self, final_reels: List[List[str]]):
        """Smooth vertical spin with staggered stops and easing delays."""
        total_frames = 20
        stop_steps = [int(total_frames * 0.6), int(total_frames * 0.8), total_frames]

        # Build symbol strips per column to simulate vertical scrolling
        strip_len = 21
        strips: List[List[str]] = [[get_random_symbol() for _ in range(strip_len)] for _ in range(3)]
        idx = [0, 0, 0]

        for step in range(1, total_frames + 1):
            frame: List[List[str]] = [[None for _ in range(3)] for _ in range(3)]

            for c in range(3):
                if step < stop_steps[c]:
                    # Column still spinning: take three consecutive symbols from the strip
                    col_syms = [strips[c][(idx[c] + r) % strip_len] for r in range(3)]
                    idx[c] = (idx[c] + 1) % strip_len
                else:
                    # Column locked: show the final column symbols
                    col_syms = [final_reels[r][c] for r in range(3)]

                for r in range(3):
                    frame[r][c] = col_syms[r]

            reels_txt = "\n".join(" ".join(row) for row in frame)
            await self.message.edit(embed=await embeds.create_slots_embed(state='spinning', reels=reels_txt))

            # Easing: start fast, slow down near the end
            t = step / total_frames
            delay = 0.06 + (0.18 - 0.06) * (t ** 2)
            await asyncio.sleep(delay)

    def _calculate_results(self) -> Tuple[int, bool]:
        total_winnings = 0
        is_jackpot = False
        bet_per_line = self.bet / PAYLINES

        lines = [
            self.reels[0], self.reels[1], self.reels[2], # Rows
            [self.reels[i][i] for i in range(3)],      # Diagonal
            [self.reels[i][2 - i] for i in range(3)]   # Anti-diagonal
        ]

        for line in lines:
            if line[0] == line[1] == line[2]:
                symbol = line[0]
                total_winnings += int(PAYOUTS[symbol] * bet_per_line)
                if symbol == JACKPOT_SYMBOL:
                    is_jackpot = True
        
        return total_winnings, is_jackpot


class SlotsGame(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot

    @nextcord.slash_command(name="slots", description="Play a game of slots!")
    async def slots(self, interaction: Interaction, bet: str = SlashOption(description="The number of chips to wager.", required=True)):
        try:
            user_data = await get_user(interaction.user.id)
            bet_amount = await parse_bet(bet, user_data['chips'])
        except ValueError as e:
            await interaction.response.send_message(embed=embeds.error_embed(str(e)), ephemeral=True)
            return

        # Lenient normalization: clamp to balance, round down to nearest multiple of PAYLINES
        adjusted_bet, note = _normalize_bet_for_paylines(int(bet_amount), int(user_data['chips']))
        if adjusted_bet == 0:
            await interaction.response.send_message(
                embed=embeds.error_embed(f"Bet must be at least {MIN_BET:,} and divisible by {PAYLINES}."),
                ephemeral=True,
            )
            return

        game = SlotsGameLogic(interaction, adjusted_bet)
        if not await game.validate_bet():
            return
        
        await interaction.response.defer()
        await game.play_game()

        # Inform about any automatic adjustment (ephemeral)
        if note:
            try:
                await interaction.followup.send(note, ephemeral=True)
            except Exception:
                pass


def setup(bot: commands.Bot):
    bot.add_cog(SlotsGame(bot))
