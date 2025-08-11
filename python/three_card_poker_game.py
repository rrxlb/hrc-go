import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption, ButtonStyle, Color
from nextcord.ui import View, Button, button
import time # Added for memory management
from typing import List, Dict, Optional, Tuple

from utils.database import get_user, parse_bet
from utils.cards import Card, Deck
from utils.embeds import error_embed, create_tcp_embed, insufficient_chips_embed
from utils.game import Game
from utils.constants import GUILD_ID, XP_PER_PROFIT, DEVELOPER_ROLE_ID

# --- Game Constants ---
ANTE_BONUS_PAYOUTS = {"Straight Flush": 5, "Three of a Kind": 4, "Straight": 1}
PAIR_PLUS_PAYOUTS = {"Straight Flush": 40, "Three of a Kind": 30, "Straight": 6, "Flush": 3, "Pair": 1}
GAME_TIMEOUT = 90.0

active_games: Dict[int, "TCPGameLogic"] = {}

def has_developer_role(interaction: Interaction) -> bool:
    if not interaction.guild: return False
    member = interaction.guild.get_member(interaction.user.id)
    if not member: return False
    return any(role.id == DEVELOPER_ROLE_ID for role in member.roles)

def evaluate_three_card_hand(hand: List[Card]) -> Tuple[str, int, List[int]]:
    values = sorted([card.value for card in hand], reverse=True)
    suits = [card.suit for card in hand]
    is_flush = len(set(suits)) == 1
    is_straight = (values[0] - 1 == values[1] and values[1] - 1 == values[2]) or (values == [14, 3, 2])
    
    display_values = [3, 2, 1] if values == [14, 3, 2] else values

    if is_straight and is_flush: return "Straight Flush", 8, display_values
    if values[0] == values[1] == values[2]: return "Three of a Kind", 7, values
    if is_straight: return "Straight", 6, display_values
    if is_flush: return "Flush", 5, values
    if values[0] == values[1] or values[1] == values[2]:
        pair_val = values[1]
        kicker_val = values[0] if values[1] == values[2] else values[2]
        return "Pair", 4, [pair_val, kicker_val]
    return "High Card", 3, values

class TCPGameLogic(Game):
    def __init__(self, interaction: Interaction, ante_bet: int, pair_plus_bet: int):
        super().__init__(interaction, ante_bet) # Main bet is the Ante
        self.pair_plus_bet = pair_plus_bet
        self.play_bet = 0
        self.deck = Deck(num_decks=1, game='poker')
        self.player_hand: List[Card] = []
        self.dealer_hand: List[Card] = []
        self.player_eval = ("", 0, [])
        self.dealer_eval = ("", 0, [])
        self.view: Optional[TCPView] = None
        self.created_at = time.time()  # For cleanup

    async def start_game(self):
        self.player_hand = [self.deck.deal() for _ in range(3)]
        self.dealer_hand = [self.deck.deal() for _ in range(3)]
        self.player_eval = evaluate_three_card_hand(self.player_hand)
        self.dealer_eval = evaluate_three_card_hand(self.dealer_hand)

        self.view = TCPView(self)
        embed = await create_tcp_embed(
            user=self.user,
            player_hand=[str(c) for c in self.player_hand],
            dealer_hand=[str(c) for c in self.dealer_hand],
            player_eval=self.player_eval[0],
            dealer_eval=self.dealer_eval[0],
            bets={'ante': self.bet, 'pair_plus': self.pair_plus_bet},
            game_over=False
        )
        await self.interaction.followup.send(embed=embed, view=self.view)
        self.view.message = await self.interaction.original_message()

    async def handle_play(self):
        total_wager = self.bet + self.pair_plus_bet + self.bet # Ante + PP + Play
        if self.user_data['chips'] < total_wager:
            await self.interaction.followup.send(
                embed=insufficient_chips_embed(
                    required_chips=total_wager,
                    current_balance=self.user_data['chips'],
                    bet_description="the Play bet"
                ), 
                ephemeral=True
            )
            await self.handle_fold(is_forced=True)
            return
        
        self.play_bet = self.bet
        await self.end_game(folded=False)

    async def handle_fold(self, is_forced: bool = False):
        await self.end_game(folded=True, is_forced=is_forced)

    async def end_game(self, folded: bool, is_forced: bool = False):
        if self.is_game_over: return
        if self.view: self.view.stop()

        total_profit = 0
        payout_results = []
        
        # Pair Plus Bet
        if self.pair_plus_bet > 0:
            if not folded and self.player_eval[0] in PAIR_PLUS_PAYOUTS:
                payout = PAIR_PLUS_PAYOUTS[self.player_eval[0]]
                profit = self.pair_plus_bet * payout
                total_profit += profit
                payout_results.append(f"Pair Plus: `+{profit:,}`")
            else:
                total_profit -= self.pair_plus_bet
                payout_results.append(f"Pair Plus: `-{self.pair_plus_bet:,}`")

        # Ante & Play Bets
        if folded:
            total_profit -= self.bet
            outcome_summary = "You folded and forfeited your Ante bet."
        else:
            dealer_qualifies = self.dealer_eval[1] > 3 or (self.dealer_eval[1] == 3 and self.dealer_eval[2][0] >= 12)
            player_wins = self.player_eval[1] > self.dealer_eval[1] or (self.player_eval[1] == self.dealer_eval[1] and self.player_eval[2] > self.dealer_eval[2])

            if not dealer_qualifies:
                outcome_summary = "Dealer does not qualify. Ante wins, Play pushes."
                total_profit += self.bet # Ante pays 1:1
            elif player_wins:
                outcome_summary = f"You win with a {self.player_eval[0]}!"
                total_profit += self.bet + self.play_bet # Ante and Play pay 1:1
                # Ante Bonus
                if self.player_eval[0] in ANTE_BONUS_PAYOUTS:
                    bonus_profit = self.bet * ANTE_BONUS_PAYOUTS[self.player_eval[0]]
                    total_profit += bonus_profit
                    payout_results.append(f"Ante Bonus: `+{bonus_profit:,}`")
            else:
                outcome_summary = f"Dealer wins with a {self.dealer_eval[0]}."
                total_profit -= (self.bet + self.play_bet)
        
        await super().end_game(profit=total_profit)
        
        final_user_data = await get_user(self.user.id)
        xp_gain = int(total_profit * XP_PER_PROFIT) if total_profit > 0 else 0
        
        final_embed = await create_tcp_embed(
            user=self.user, player_hand=[str(c) for c in self.player_hand], dealer_hand=[str(c) for c in self.dealer_hand],
            player_eval=self.player_eval[0], dealer_eval=self.dealer_eval[0],
            bets={'ante': self.bet, 'pair_plus': self.pair_plus_bet}, game_over=True,
            outcome_summary=outcome_summary, payout_results=payout_results,
            final_balance=final_user_data['chips'], profit=total_profit, xp_gain=xp_gain
        )
        
        if is_forced: await self.interaction.followup.send(embed=final_embed, view=None)
        else: await self.view.message.edit(embed=final_embed, view=None)

        if self.user.id in active_games: del active_games[self.user.id]

class TCPView(View):
    def __init__(self, game_logic: TCPGameLogic):
        super().__init__(timeout=GAME_TIMEOUT)
        self.game = game_logic
        self.message: Optional[nextcord.Message] = None

    async def on_timeout(self):
        for item in self.children: item.disabled = True
        
        # Calculate total bet amount for forfeiture message
        total_bet = self.game.bet + self.game.pair_plus_bet
        
        # Create cleanup embed with chip forfeiture message
        from utils import embeds
        cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
        
        # Update message with cleanup embed
        if self.message:
            try:
                await self.message.edit(embed=cleanup_embed, view=self)
            except (nextcord.HTTPException, nextcord.NotFound):
                pass  # Message was deleted or couldn't be edited
        
        await self.game.handle_fold(is_forced=True)

    async def interaction_check(self, interaction: Interaction) -> bool:
        if interaction.user.id != self.game.user.id:
            await interaction.response.send_message("This is not your game!", ephemeral=True)
            return False
        return True

    @button(label="Play", style=ButtonStyle.success)
    async def play_button(self, button: Button, interaction: Interaction):
        for item in self.children: item.disabled = True
        await interaction.response.edit_message(view=self)
        await self.game.handle_play()

    @button(label="Fold", style=ButtonStyle.danger)
    async def fold_button(self, button: Button, interaction: Interaction):
        for item in self.children: item.disabled = True
        await interaction.response.edit_message(view=self)
        await self.game.handle_fold()

class ThreeCardPokerGame(commands.Cog):
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
                # Calculate total bet amount for forfeiture message
                total_bet = game_to_remove.bet + game_to_remove.pair_plus_bet
                
                # Create cleanup embed with chip forfeiture message
                from utils import embeds
                cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
                
                # Disable all buttons and update message
                if hasattr(game_to_remove, 'view') and game_to_remove.view:
                    for child in game_to_remove.view.children:
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
            print(f"Cleaned up {len(to_remove)} old three card poker games")

    @cleanup_old_games.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(name="tcpoker", description="Play Three Card Poker.")
    @commands.check(has_developer_role)
    async def tcpoker(self, interaction: Interaction, ante_bet: str, pair_plus_bet: Optional[str] = None):
        if interaction.user.id in active_games:
            await interaction.response.send_message(embed=error_embed("You have an active game."), ephemeral=True)
            return

        try:
            user_data = await get_user(interaction.user.id)
            ante_amount = await parse_bet(ante_bet, user_data['chips'])
            pp_amount = 0
            if pair_plus_bet:
                pp_amount = await parse_bet(pair_plus_bet, user_data['chips'] - ante_amount)
        except ValueError as e:
            await interaction.response.send_message(embed=error_embed(str(e)), ephemeral=True)
            return

        # Validate the total potential bet (Ante + PP + Play)
        total_potential_bet = ante_amount * 2 + pp_amount
        if user_data['chips'] < total_potential_bet:
            await interaction.response.send_message(
                embed=insufficient_chips_embed(
                    required_chips=total_potential_bet,
                    current_balance=user_data['chips'],
                    bet_description=f"all bets (Ante, Pair Plus, and Play)"
                ), 
                ephemeral=True
            )
            return

        game = TCPGameLogic(interaction, ante_amount, pp_amount)
        # The Game class __init__ doesn't need the full bet, just a placeholder for validation
        game.bet = total_potential_bet
        if not await game.validate_bet():
            return
        game.bet = ante_amount # Reset bet to the ante amount

        active_games[interaction.user.id] = game
        await interaction.response.defer()
        await game.start_game()

def setup(bot: commands.Bot):
    bot.add_cog(ThreeCardPokerGame(bot))
