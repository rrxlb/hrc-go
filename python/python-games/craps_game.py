import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption, Color, ButtonStyle
from nextcord.ui import Modal, TextInput, button, Button
from nextcord.ui import View
import random
import math
import time # Added for memory management
from typing import List, Dict, Any, Optional, Tuple

from utils.database import get_user, parse_bet
from utils import embeds
from utils.views import TimeoutView, ResumeView
from utils.game import Game
from utils.constants import XP_PER_PROFIT, GUILD_ID

# --- Payout Ratios ---
PAYOUT_RATIOS = {
    "pass_line": 1, "dont_pass": 1, "come": 1, "dont_come": 1,
    "place_4": 9/5, "place_5": 7/5, "place_6": 7/6, "place_8": 7/6, "place_9": 7/5, "place_10": 9/5,
    "field_2": 2, "field_12": 3, "field_other": 1,
    "hard_4": 7, "hard_10": 7, "hard_6": 9, "hard_8": 9,
}

# --- Game State Management ---
active_games: Dict[int, "CrapsGameLogic"] = {}

def roll_dice() -> Tuple[int, int]:
    return (random.randint(1, 6), random.randint(1, 6))

DICE_EMOJI = {
    1: "<:dicesixfacesone:1396630388620656782>", 2: "<:dicesixfacestwo:1396630415455948990>",
    3: "<:dicesixfacesthree:1396630430136139907>", 4: "<:dicesixfacesfour:1396630442316398632>",
    5: "<:dicesixfacesfive:1396630450667262138>", 6: "<:dicesixfacessix:1396630463245844541>",
}

class AmountModal(Modal):
    def __init__(self, bet_type: str, game_logic: "CrapsGameLogic"):
        super().__init__("Enter Bet Amount")
        self.bet_type = bet_type
        self._game_logic_ref = game_logic
        self.add_item(TextInput(label=f"Amount for {bet_type.replace('_', ' ').title()}", placeholder="Enter chips amount", required=True))

    async def callback(self, interaction: Interaction):
        try:
            amount = int(self.children[0].value)
            if amount <= 0: raise ValueError
            await self._game_logic_ref.handle_new_bet(interaction, self.bet_type, amount)
        except ValueError:
            await interaction.response.send_message("Invalid bet amount.", ephemeral=True)

class BetTypeSelect(nextcord.ui.Select):
    def __init__(self, game_logic: "CrapsGameLogic"):
        self._game_logic_ref = game_logic
        super().__init__(placeholder="Choose a bet type...", options=self._get_options())

    def _get_options(self) -> List[nextcord.SelectOption]:
        options = [nextcord.SelectOption(label=bt.replace("_", " ").title(), value=bt) for bt in ["field"] + [f"place_{n}" for n in [4,5,6,8,9,10]] + [f"hard_{n}" for n in [4,6,8,10]]]
        if self._game_logic_ref.phase == "come_out":
            options.extend([nextcord.SelectOption(label="Pass Line", value="pass_line"), nextcord.SelectOption(label="Don't Pass", value="dont_pass")])
        else:
            options.extend([nextcord.SelectOption(label="Come", value="come"), nextcord.SelectOption(label="Don't Come", value="dont_come")])
        return options

    async def callback(self, interaction: Interaction):
        await interaction.response.send_modal(AmountModal(self.values[0], self._game_logic_ref))

class BettingView(View):
    def __init__(self, game_logic: "CrapsGameLogic"):
        super().__init__(timeout=180)
        self.add_item(BetTypeSelect(game_logic))

class CrapsView(TimeoutView):
    def __init__(self, author: nextcord.User, game_logic: "CrapsGameLogic"):
        super().__init__(author)
        self._game_logic_ref = game_logic

    @button(label="Roll", style=ButtonStyle.green)
    async def roll(self, button: Button, interaction: Interaction):
        await interaction.response.defer()
        await self._game_logic_ref.handle_roll(interaction)

    @button(label="Bet", style=ButtonStyle.blurple)
    async def bet(self, button: Button, interaction: Interaction):
        await interaction.response.send_message("Select a bet type:", view=BettingView(self._game_logic_ref), ephemeral=True)

    async def on_timeout(self):
        await self._game_logic_ref.handle_timeout()

class PlaceBetDecisionView(TimeoutView):
    def __init__(self, author: nextcord.User, game_logic: "CrapsGameLogic", bet_type: str, winnings: int):
        super().__init__(author, timeout=30)
        self._game_logic_ref = game_logic
        self.bet_type = bet_type
        self.winnings = winnings
        self.message: Optional[nextcord.Message] = None

    @button(label="Keep Bet Up", style=nextcord.ButtonStyle.green)
    async def keep_bet(self, button: Button, interaction: nextcord.Interaction):
        self._game_logic_ref.session_profit += self.winnings
        await interaction.response.send_message(f"You won {self.winnings:,} and kept your bet on {self.bet_type.replace('_', ' ').title()}.", ephemeral=True)
        self.stop()
        if self.message:
            await self.message.delete()
        await self._game_logic_ref.update_view(self._game_logic_ref.interaction)


    @button(label="Take Bet Down", style=nextcord.ButtonStyle.red)
    async def take_down(self, button: Button, interaction: nextcord.Interaction):
        original_bet = self._game_logic_ref.bets.pop(self.bet_type, 0)
        total_return = self.winnings + original_bet
        self._game_logic_ref.session_profit += self.winnings
        
        await interaction.response.send_message(f"You won {self.winnings:,} and took down your bet on {self.bet_type.replace('_', ' ').title()}, receiving {total_return:,} total.", ephemeral=True)
        self.stop()
        if self.message:
            await self.message.delete()
        await self._game_logic_ref.update_view(self._game_logic_ref.interaction)

    async def on_timeout(self):
        self._game_logic_ref.session_profit += self.winnings
        self.stop()
        if self.message:
            try:
                await self.message.delete()
            except nextcord.NotFound:
                pass
        await self._game_logic_ref.update_view(self._game_logic_ref.interaction)

class CrapsGameLogic(Game):
    def __init__(self, interaction: Interaction, initial_bet: int):
        super().__init__(interaction, initial_bet)
        self.view = CrapsView(self.user, self)
        self.phase = "come_out"
        self.point: Optional[int] = None
        self.bets: Dict[str, int] = {"pass_line": initial_bet}
        self.come_points: Dict[int, int] = {}
        self.session_profit = 0
        self.created_at = time.time()  # For cleanup

    async def start_game(self):
        await self.update_view(self.interaction, initial=True)

    async def handle_roll(self, interaction: Interaction):
        dice = roll_dice()
        total = sum(dice)
        
        if self.phase == "come_out":
            await self.handle_come_out_roll(interaction, total, dice)
        else:
            await self.handle_point_roll(interaction, total, dice)

    async def handle_new_bet(self, interaction: Interaction, bet_type: str, amount: int):
        if bet_type in ["pass_line", "dont_pass"] and self.phase != "come_out":
            await interaction.response.send_message("Pass Line and Don't Pass bets can only be made on the come-out roll.", ephemeral=True)
            return
        if bet_type in ["come", "dont_come"] and self.phase == "come_out":
            await interaction.response.send_message("Come and Don't Come bets can only be made when a point is established.", ephemeral=True)
            return
        if bet_type.startswith("place_") and self.phase == "come_out":
            await interaction.response.send_message("Place bets can only be made when a point is established.", ephemeral=True)
            return
        if bet_type in self.bets:
            await interaction.response.send_message(f"You already have a bet on {bet_type.replace('_', ' ').title()}.", ephemeral=True)
            return

        total_wagered = sum(self.bets.values()) + sum(self.come_points.values())
        total_required = total_wagered + amount
        if self.user_data['chips'] < total_required:
            await interaction.response.send_message(
                embed=embeds.insufficient_chips_embed(
                    required_chips=total_required,
                    current_balance=self.user_data['chips'],
                    bet_description=f"this bet ({amount:,} chips on {bet_type.replace('_', ' ').title()})"
                ), 
                ephemeral=True
            )
            return
        
        self.bets[bet_type] = self.bets.get(bet_type, 0) + amount
        await interaction.response.send_message(f"Bet of {amount:,} placed on {bet_type.replace('_', ' ').title()}.", ephemeral=True)
        await self.update_view(self.interaction)

    def _resolve_roll(self, total: int, dice: Tuple[int, int], is_come_out: bool) -> Tuple[str, int, Dict[str, int]]:
        roll_profit = 0
        outcome_lines = []
        bets_to_remove = []
        winning_place_bets_winnings = {}

        if "field" in self.bets:
            bet_amount = self.bets["field"]
            if total == 2:
                winnings = bet_amount * PAYOUT_RATIOS["field_2"]
                total_payout = bet_amount + winnings
                total_payout = math.ceil(total_payout)
                roll_profit += total_payout
                outcome_lines.append(f"Field bet wins {total_payout:,} total (2x payout)!")
            elif total == 12:
                winnings = bet_amount * PAYOUT_RATIOS["field_12"]
                total_payout = bet_amount + winnings
                total_payout = math.ceil(total_payout)
                roll_profit += total_payout
                outcome_lines.append(f"Field bet wins {total_payout:,} total (3x payout)!")
            elif total in (3, 4, 9, 10, 11):
                winnings = bet_amount * PAYOUT_RATIOS["field_other"]
                total_payout = bet_amount + winnings
                total_payout = math.ceil(total_payout)
                roll_profit += total_payout
                outcome_lines.append(f"Field bet wins {total_payout:,} total!")
            else:
                roll_profit -= bet_amount
                outcome_lines.append("Field bet loses.")
            bets_to_remove.append("field")

        for bet_type in list(self.bets.keys()):
            if bet_type.startswith("hard_"):
                num = int(bet_type.split("_")[1])
                if dice[0] == dice[1] and sum(dice) == num:
                    winnings = self.bets[bet_type] * PAYOUT_RATIOS[bet_type]
                    winnings = math.ceil(winnings)
                    winning_place_bets_winnings[bet_type] = winnings
                    outcome_lines.append(f"Hard {num} hits!")
                elif sum(dice) == 7 or (sum(dice) == num and dice[0] != dice[1]):
                    roll_profit -= self.bets[bet_type]
                    outcome_lines.append(f"Hard {num} loses.")
                    bets_to_remove.append(bet_type)

        if is_come_out:
            if "pass_line" in self.bets:
                if total in (7, 11):
                    total_payout = self.bets["pass_line"] * 2
                    roll_profit += total_payout
                    outcome_lines.append(f"Pass Line wins {total_payout:,} total!")
                elif total in (2, 3, 12):
                    roll_profit -= self.bets["pass_line"]
                    outcome_lines.append("Pass Line loses (Craps).")
            if "dont_pass" in self.bets:
                if total in (2, 3):
                    total_payout = self.bets["dont_pass"] * 2
                    roll_profit += total_payout
                    outcome_lines.append(f"Don't Pass wins {total_payout:,} total!")
                elif total in (7, 11):
                    roll_profit -= self.bets["dont_pass"]
                    outcome_lines.append("Don't Pass loses.")
                elif total == 12:
                    outcome_lines.append("Don't Pass pushes (Bar 12).")
        else:
            if "pass_line" in self.bets:
                if total == self.point:
                    total_payout = self.bets["pass_line"] * 2
                    roll_profit += total_payout
                    outcome_lines.append(f"Point of {self.point} hit! Pass Line wins {total_payout:,} total!")
                elif total == 7:
                    roll_profit -= self.bets["pass_line"]
                    outcome_lines.append("Seven out! Pass Line loses.")
            if "dont_pass" in self.bets:
                if total == 7:
                    total_payout = self.bets["dont_pass"] * 2
                    roll_profit += total_payout
                    outcome_lines.append(f"Seven out! Don't Pass wins {total_payout:,} total!")
                elif total == self.point:
                    roll_profit -= self.bets["dont_pass"]
                    outcome_lines.append(f"Point of {self.point} hit! Don't Pass loses.")

            for bet_type in list(self.bets.keys()):
                if bet_type.startswith("place_"):
                    num = int(bet_type.split("_")[1])
                    if total == num:
                        winnings = self.bets[bet_type] * PAYOUT_RATIOS[bet_type]
                        winnings = math.ceil(winnings)
                        winning_place_bets_winnings[bet_type] = winnings
                        outcome_lines.append(f"Place bet on {num} wins!")
                    elif total == 7:
                        roll_profit -= self.bets[bet_type]
                        outcome_lines.append(f"Place bet on {num} loses (Seven out).")
                        bets_to_remove.append(bet_type)

        if "come" in self.bets:
            if total in (7, 11):
                total_payout = self.bets["come"] * 2
                roll_profit += total_payout
                outcome_lines.append(f"Come bet wins {total_payout:,} total!")
            elif total in (2, 3, 12):
                roll_profit -= self.bets["come"]
                outcome_lines.append("Come bet loses.")
            else:
                self.come_points[total] = self.come_points.get(total, 0) + self.bets["come"]
                outcome_lines.append(f"Come point is now {total}.")
            bets_to_remove.append("come")

        if "dont_come" in self.bets:
            if total in (2, 3):
                total_payout = self.bets["dont_come"] * 2
                roll_profit += total_payout
                outcome_lines.append(f"Don't Come bet wins {total_payout:,} total!")
            elif total in (7, 11):
                roll_profit -= self.bets["dont_come"]
                outcome_lines.append("Don't Come bet loses.")
            elif total == 12:
                 outcome_lines.append("Don't Come bet pushes.")
            else:
                outcome_lines.append(f"Don't Come point established on {total}.")
            bets_to_remove.append("dont_come")

        for point, bet_amount in list(self.come_points.items()):
            if total == point:
                total_payout = bet_amount * 2
                roll_profit += total_payout
                outcome_lines.append(f"Come point {point} hit! You win {total_payout:,} total!")
                del self.come_points[point]
            elif total == 7:
                roll_profit -= bet_amount
                outcome_lines.append(f"Come point {point} loses (Seven out).")
                del self.come_points[point]

        for bet in set(bets_to_remove):
            if bet in self.bets:
                del self.bets[bet]

        return "\n".join(outcome_lines), roll_profit, winning_place_bets_winnings

    async def handle_come_out_roll(self, interaction: Interaction, total: int, dice: Tuple[int, int]):
        outcome_text, roll_profit, _ = self._resolve_roll(total, dice, is_come_out=True)
        self.session_profit += roll_profit

        if total not in (2, 3, 7, 11, 12):
            self.phase = "point"
            self.point = total
            point_set_message = f"Point is now {total}. Roll a {total} again to win!"
            outcome_text = f"{outcome_text}\n{point_set_message}" if outcome_text else point_set_message
        
        await self.update_view(interaction, roll_result=dice, outcome_text=outcome_text)

    async def handle_point_roll(self, interaction: Interaction, total: int, dice: Tuple[int, int]):
        outcome_text, roll_profit, winning_place_bets = self._resolve_roll(total, dice, is_come_out=False)
        self.session_profit += roll_profit

        game_over = False
        if total == self.point:
            win_message = f"Winner! You hit the point ({total}). A new come-out roll begins."
            outcome_text = f"{outcome_text}\n{win_message}" if outcome_text else win_message
            self.phase = "come_out"
            self.point = None
        elif total == 7:
            game_over = True
            profit = self.session_profit
            if profit >= 0:
                lose_message = f"Seven out! The game has ended.\n**Total Profit:** {profit:,} chips."
            else:
                lose_message = f"Seven out! The game has ended.\n**Total Loss:** {abs(profit):,} chips."
            outcome_text = f"{outcome_text}\n{lose_message}" if outcome_text else lose_message

        await self.update_view(interaction, roll_result=dice, outcome_text=outcome_text, game_over=game_over)

        for bet_type, winnings in winning_place_bets.items():
            view = PlaceBetDecisionView(self.user, self, bet_type, winnings)
            message = await interaction.followup.send(
                f"Your {bet_type.replace('_', ' ').title()} bet won! What would you like to do?", 
                view=view,
                ephemeral=True
            )
            view.message = message

        if game_over:
            await self.end_game(profit=self.session_profit)

    async def end_game(self, profit: int):
        await super().end_game(profit=profit)
        if self.user.id in active_games:
            del active_games[self.user.id]
        self.view.stop()

    async def resume_game(self, interaction: nextcord.Interaction):
        self.view = CrapsView(self.user, self)
        bet_summary_lines = [f"**{bet.replace('_', ' ').title()}:** {amount:,}" for bet, amount in self.bets.items()]
        bet_summary = "\n".join(bet_summary_lines) if bet_summary_lines else "No bets placed."
        layout = self._create_layout()
        roll_display = "Game resumed. Your turn to roll."

        embed = await embeds.create_craps_embed(
            user=self.user,
            point=self.point,
            bet_summary=bet_summary,
            layout=layout,
            roll_display=roll_display,
            outcome_text="Game resumed!"
        )
        
        await interaction.response.edit_message(embed=embed, view=self.view)

    async def quit_game(self, interaction: nextcord.Interaction):
        forfeited_amount = sum(self.bets.values()) + sum(self.come_points.values())
        final_profit = self.session_profit - forfeited_amount
        await self.end_game(profit=final_profit)
        
        for item in self.view.children:
            item.disabled = True
            
        final_embed = embeds.create_branded_embed(
            title="🎲 Craps Table",
            color=Color.red(),
            description="You have quit the game and forfeited your bets."
        )
        final_embed.set_footer(text="Start a new game with /craps.")
        
        await interaction.response.edit_message(embed=final_embed, view=None)
        if self.user.id in active_games:
            del active_games[self.user.id]
        self.view.stop()

    async def handle_timeout(self):
        for item in self.view.children:
            item.disabled = True
        
        resume_view = ResumeView(self.user, self, self.view)
        timeout_embed = embeds.create_timeout_embed(
            "Your game has timed out. Would you like to resume?"
        )
        if self.view.message:
            await self.view.message.edit(embed=timeout_embed, view=resume_view)
        else:
            await self.interaction.edit_original_message(embed=timeout_embed, view=resume_view)

    def _create_layout(self) -> str:
        place_numbers = [4, 5, 6, 8, 9, 10]
        place_bet_str = ""
        for num in place_numbers:
            bet_amount = self.bets.get(f"place_{num}")
            point_marker = " POINT" if self.point == num else ""
            if bet_amount:
                place_bet_str += f"[{num}: {bet_amount:,}{point_marker}] "
            else:
                place_bet_str += f"[{num}{point_marker}] "
        
        separator = "—" * 40

        def format_bet(name: str, key: str) -> str:
            amount = self.bets.get(key)
            return f"{name}: {amount:,}" if amount else name

        pass_line = format_bet("Pass Line", "pass_line")
        come = format_bet("Come", "come")
        field = format_bet("Field", "field")
        
        dont_pass = format_bet("Don't Pass", "dont_pass")
        dont_come = format_bet("Don't Come", "dont_come")

        layout = (
            f"`{place_bet_str.strip()}`\n"
            f"{separator}\n"
            f"**{pass_line} | {come} | {field}**\n"
            f"{separator}\n"
            f"**{dont_pass} | {dont_come}**"
        )
        return layout

    async def update_view(self, interaction: Interaction, **kwargs):
        is_initial = kwargs.pop('initial', False)
        game_over = kwargs.get('game_over', False)
        
        user = self.user
        point = self.point
        
        bet_summary_lines = [f"**{bet.replace('_', ' ').title()}:** {amount:,}" for bet, amount in self.bets.items()]
        bet_summary = "\n".join(bet_summary_lines) if bet_summary_lines else "No bets placed."

        layout = self._create_layout()
        
        roll_result = kwargs.pop('roll_result', None)
        if roll_result:
            dice1, dice2 = roll_result
            roll_display = f"{DICE_EMOJI[dice1]} {DICE_EMOJI[dice2]} (Total: {sum(roll_result)})"
        else:
            roll_display = "Waiting to roll..."

        if game_over:
            self.view.stop()
            for item in self.view.children:
                if isinstance(item, (Button, nextcord.ui.Select)):
                    item.disabled = True

        embed = await embeds.create_craps_embed(
            user=user,
            point=point,
            bet_summary=bet_summary,
            layout=layout,
            roll_display=roll_display,
            **kwargs
        )
        
        if is_initial:
            self.view.message = await interaction.followup.send(embed=embed, view=self.view)
        elif not self.view.message:
            self.view.message = await interaction.followup.send(embed=embed, view=self.view)
        else:
            await self.view.message.edit(embed=embed, view=self.view)

class CrapsGame(commands.Cog):
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
                total_bet = sum(game_to_remove.bets.values()) + sum(game_to_remove.come_points.values())
                
                # Create cleanup embed with chip forfeiture message
                cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
                
                # Disable all buttons and update message
                if hasattr(game_to_remove, 'view') and game_to_remove.view:
                    for child in game_to_remove.view.children:
                        if isinstance(child, (Button, nextcord.ui.Select)): 
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
            print(f"Cleaned up {len(to_remove)} old craps games")

    @cleanup_old_games.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(name="craps", description="Play a game of Craps.")
    async def craps(self, interaction: Interaction, bet: str = SlashOption(description="Your Pass Line bet. Use 'all' or 'half'.", required=True)):
        if interaction.user.id in active_games:
            await interaction.response.send_message(embed=embeds.error_embed("You already have an active game."), ephemeral=True)
            return

        try:
            user_data = await get_user(interaction.user.id)
            bet_amount = await parse_bet(bet, user_data['chips'])
        except ValueError as e:
            await interaction.response.send_message(embed=embeds.error_embed(str(e)), ephemeral=True)
            return

        game = CrapsGameLogic(interaction, bet_amount)
        if not await game.validate_bet():
            return
            
        active_games[interaction.user.id] = game
        await interaction.response.defer()
        await game.start_game()

def setup(bot: commands.Bot):
    bot.add_cog(CrapsGame(bot))
