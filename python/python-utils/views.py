import asyncio
import nextcord
from nextcord.ui import View, Button, button
from utils.embeds import create_timeout_embed, create_game_timeout_embed, _create_branded_embed
from utils.database import update_user, get_user
from typing import List, TYPE_CHECKING, Dict, Any, Optional, Callable, Union
from .constants import TOPGG_VOTE_LINK, STARTING_CHIPS
import weakref
from abc import ABC, abstractmethod

if TYPE_CHECKING:
    from cogs.roulette_game import RouletteGameLogic
    from cogs.baccarat_game import BaccaratGameLogic
    from cogs.higher_or_lower_game import HigherOrLowerGameLogic

# Generic button interaction patterns
class GameAction:
    """Represents a game action that can be bound to a button."""
    
    def __init__(self, label: str, style: nextcord.ButtonStyle, callback: Callable, 
                 custom_id: Optional[str] = None, row: Optional[int] = None,
                 disabled: bool = False, emoji: Optional[str] = None):
        self.label = label
        self.style = style
        self.callback = callback
        self.custom_id = custom_id
        self.row = row
        self.disabled = disabled
        self.emoji = emoji

class DynamicButton(Button):
    """Generic button that can be reconfigured at runtime."""
    
    def __init__(self, action: GameAction, game_logic=None):
        super().__init__(
            style=action.style,
            label=action.label,
            custom_id=action.custom_id,
            row=action.row,
            disabled=action.disabled,
            emoji=action.emoji
        )
        self.action = action
        self._game_logic = weakref.ref(game_logic) if game_logic else None
    
    @property
    def game_logic(self):
        """Get game logic with weak reference protection."""
        return self._game_logic() if self._game_logic else None
    
    async def callback(self, interaction: nextcord.Interaction):
        try:
            if self.action.callback:
                if self.game_logic:
                    await self.action.callback(self.game_logic, interaction)
                else:
                    await self.action.callback(interaction)
        except Exception as e:
            print(f"Button callback error: {e}")
            try:
                await interaction.response.send_message("An error occurred. Please try again.", ephemeral=True)
            except:
                pass  # Interaction may already be responded to

class BaseGameView(View, ABC):
    """Base class for all game views with common patterns."""
    
    def __init__(self, author: nextcord.User, game_logic=None, timeout: int = 300):
        super().__init__(timeout=timeout)
        self.author = author
        self._game_logic = weakref.ref(game_logic) if game_logic else None
        self.message: Optional[nextcord.Message] = None
        self._is_finished = False
    
    @property
    def game_logic(self):
        """Get game logic with weak reference protection."""
        return self._game_logic() if self._game_logic else None
    
    async def interaction_check(self, interaction: nextcord.Interaction) -> bool:
        """Enhanced interaction check with better error handling."""
        if self._is_finished:
            await interaction.response.send_message("This game has already ended.", ephemeral=True)
            return False
            
        if self.author is None:
            return True  # Allow any user when no specific author is set
            
        if interaction.user.id != self.author.id:
            await interaction.response.send_message("This is not your game!", ephemeral=True)
            return False
        return True
    
    def add_game_action(self, action: GameAction):
        """Add a game action as a button to this view."""
        button = DynamicButton(action, self.game_logic)
        self.add_item(button)
    
    def disable_all_buttons(self):
        """Efficiently disable all buttons in the view."""
        for item in self.children:
            if isinstance(item, Button):
                item.disabled = True
    
    def enable_buttons(self, *custom_ids: str):
        """Enable specific buttons by custom_id."""
        for item in self.children:
            if isinstance(item, Button) and item.custom_id in custom_ids:
                item.disabled = False
    
    def update_button_labels(self, updates: Dict[str, str]):
        """Update button labels by custom_id."""
        for item in self.children:
            if isinstance(item, Button) and item.custom_id in updates:
                item.label = updates[item.custom_id]
    
    async def cleanup(self):
        """Clean up resources when view is finished."""
        self._is_finished = True
        self.disable_all_buttons()
        if self.game_logic and hasattr(self.game_logic, 'cleanup_memory'):
            self.game_logic.cleanup_memory()
    
    async def on_timeout(self) -> None:
        """Enhanced timeout handling with game-specific logic."""
        await self.cleanup()
        
        if self.message:
            timeout_embed = create_timeout_embed()
            try:
                await self.message.edit(embed=timeout_embed, view=self)
            except (nextcord.HTTPException, nextcord.NotFound):
                pass  # Message was deleted or couldn't be edited
        
        # Call game-specific timeout handling
        if self.game_logic and hasattr(self.game_logic, 'handle_timeout'):
            try:
                await self.game_logic.handle_timeout()
            except Exception as e:
                print(f"Game timeout handling error: {e}")
        
        self.stop()

class CallbackButton(Button):
    """Legacy callback button for backwards compatibility."""
    def __init__(self, *, style: nextcord.ButtonStyle = nextcord.ButtonStyle.secondary, 
                 label: str = None, custom_id: str = None, row: int = None, callback: callable):
        super().__init__(style=style, label=label, custom_id=custom_id, row=row)
        self._callback = callback

    async def callback(self, interaction: nextcord.Interaction):
        if self._callback:
            await self._callback(interaction)

class TimeoutView(BaseGameView):
    """Enhanced timeout view with improved performance and memory management."""
    
    def __init__(self, author: nextcord.User, *, timeout: int = 300):
        super().__init__(author, timeout=timeout)

class Confirm(TimeoutView):
    """Optimized confirmation dialog with clear state management."""
    
    def __init__(self, author: nextcord.User):
        super().__init__(author, timeout=60)
        self.value = None
        self._setup_buttons()
    
    def _setup_buttons(self):
        """Setup buttons using the new action system."""
        confirm_action = GameAction(
            label="Confirm",
            style=nextcord.ButtonStyle.green,
            callback=self._confirm_callback,
            custom_id="confirm"
        )
        cancel_action = GameAction(
            label="Cancel", 
            style=nextcord.ButtonStyle.red,
            callback=self._cancel_callback,
            custom_id="cancel"
        )
        
        self.add_game_action(confirm_action)
        self.add_game_action(cancel_action)
    
    async def _confirm_callback(self, interaction: nextcord.Interaction):
        self.value = True
        await self.cleanup()
        self.stop()
    
    async def _cancel_callback(self, interaction: nextcord.Interaction):
        self.value = False
        await self.cleanup()
        self.stop()

class Paginator(TimeoutView):
    def __init__(self, author: nextcord.User, embeds: List[nextcord.Embed]):
        super().__init__(author, timeout=120)
        self.embeds = embeds
        self.current_page = 0

    @button(label="Previous", style=nextcord.ButtonStyle.blurple)
    async def previous_button(self, button: Button, interaction: nextcord.Interaction):
        self.current_page -= 1
        if self.current_page < 0:
            self.current_page = len(self.embeds) - 1
        await self.update_message(interaction)

    @button(label="Next", style=nextcord.ButtonStyle.blurple)
    async def next_button(self, button: Button, interaction: nextcord.Interaction):
        self.current_page += 1
        if self.current_page >= len(self.embeds):
            self.current_page = 0
        await self.update_message(interaction)

    async def update_message(self, interaction: nextcord.Interaction):
        await interaction.response.edit_message(embed=self.embeds[self.current_page])

class ResumeView(View):
    def __init__(self, author: nextcord.User, game_logic, original_view: View):
        super().__init__(timeout=60)
        self.author = author
        self.game_logic = game_logic
        self.original_view = original_view
        self.value = None

    async def interaction_check(self, interaction: nextcord.Interaction) -> bool:
        return interaction.user.id == self.author.id

    @button(label="Resume", style=nextcord.ButtonStyle.green)
    async def resume(self, button: Button, interaction: nextcord.Interaction):
        self.value = True
        self.stop()
        await self.game_logic.resume_.game(interaction)

    @button(label="Quit", style=nextcord.ButtonStyle.red)
    async def quit(self, button: Button, interaction: nextcord.Interaction):
        self.value = False
        self.stop()
        await self.game_logic.quit_game(interaction)
    async def on_timeout(self):
        # If timeout occurs on the resume view, forfeit the game
        await self.game_logic.end_game(folded=True)

class RouletteView(BaseGameView):
    def __init__(self, author: nextcord.User, game_logic: "RouletteGameLogic"):
        super().__init__(author, game_logic)
        self._setup_buttons()

    def _setup_buttons(self):
        actions = [
            GameAction("Red", nextcord.ButtonStyle.red, self._bet_callback, "bet_red"),
            GameAction("Black", nextcord.ButtonStyle.secondary, self._bet_callback, "bet_black"),
            GameAction("Odd", nextcord.ButtonStyle.blurple, self._bet_callback, "bet_odd"),
            GameAction("Even", nextcord.ButtonStyle.blurple, self._bet_callback, "bet_even"),
            GameAction("Spin", nextcord.ButtonStyle.green, self._spin_callback, "spin", row=1),
        ]
        for action in actions:
            self.add_game_action(action)

    async def _bet_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            bet_type = interaction.data['custom_id'].replace('bet_', '')
            await game_logic.place_bet(interaction, bet_type)

    async def _spin_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.spin_wheel(interaction)


class BlackjackView(BaseGameView):
    """Optimized Blackjack view with dynamic button management."""
    
    def __init__(self, author: nextcord.User, game_logic):
        super().__init__(author, game_logic, timeout=300)
        self._setup_buttons()
    
    def _setup_buttons(self):
        """Setup blackjack buttons using the new action system."""
        actions = [
            GameAction("Hit", nextcord.ButtonStyle.green, self._hit_callback, "hit"),
            GameAction("Stand", nextcord.ButtonStyle.red, self._stand_callback, "stand"),
            GameAction("Double Down", nextcord.ButtonStyle.blurple, self._double_down_callback, "double_down"),
            GameAction("Split", nextcord.ButtonStyle.grey, self._split_callback, "split", row=1),
            GameAction("Insurance", nextcord.ButtonStyle.secondary, self._insurance_callback, "insurance", row=1),
        ]
        
        for action in actions:
            self.add_game_action(action)
    
    async def _hit_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.handle_hit(interaction)
    
    async def _stand_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.handle_stand(interaction)
    
    async def _double_down_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.handle_double_down(interaction)
    
    async def _split_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.handle_split(interaction)
    
    async def _insurance_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.handle_insurance(interaction)
    
    async def on_timeout(self):
        """Enhanced timeout with game-specific handling."""
        if self.game_logic and hasattr(self.game_logic, 'handle_timeout'):
            await self.game_logic.handle_timeout()
        await super().on_timeout()

class VoteView(View):
    def __init__(self):
        super().__init__(timeout=None)
        self.add_item(Button(label="Vote on Top.gg", url=TOPGG_VOTE_LINK, style=nextcord.ButtonStyle.link))

class BaccaratView(TimeoutView):
    def __init__(self, author: nextcord.User, bet_amount: int, active_games: Dict[int, Any]) -> None:
        super().__init__(author)
        self.bet_amount = bet_amount
        self.active_games = active_games
        self._game_started = False
        self._processing_lock = asyncio.Lock()

    async def end_game(self, interaction: nextcord.Interaction) -> None:
        player_id = interaction.user.id
        self.active_games.pop(player_id, None)
        self.stop()

    async def start_baccarat_game(self, interaction: nextcord.Interaction, choice: str) -> None:
        """Start baccarat game with proper synchronization."""
        async with self._processing_lock:
            if self._game_started:
                await interaction.response.defer()
                return
            
            self._game_started = True

        from cogs.baccarat_game import BaccaratGameLogic
        from utils.constants import XP_PER_PROFIT
        from utils import embeds
        from utils.embeds import clear_user_cache

        player_id = interaction.user.id

        # Disable all buttons immediately
        for item in self.children:
            item.disabled = True
        await interaction.response.edit_message(view=self)

        try:
            # Subtract bet from user's balance
            user_after_bet = await update_user(player_id, chips_increment=-self.bet_amount)

            # Run game logic
            game = BaccaratGameLogic(interaction, self.bet_amount, choice)
            results = game.run_game()
            
            profit = results['profit']
            xp_gain = 0
            update_data: Dict[str, Any] = {}

            if profit > 0:
                xp_gain = int(profit * XP_PER_PROFIT)
                update_data.update({
                    'wins_increment': 1,
                    'total_xp_increment': xp_gain,
                    'chips_increment': self.bet_amount + profit
                })

                # Cap current XP at maximum
                try:
                    from utils.constants import MAX_CURRENT_XP
                    current_xp = int(user_after_bet['current_xp']) if user_after_bet else 0
                    if current_xp < MAX_CURRENT_XP:
                        allowed = min(xp_gain, MAX_CURRENT_XP - current_xp)
                        if allowed > 0:
                            update_data['current_xp_increment'] = allowed
                except Exception:
                    pass
            elif profit < 0:
                update_data['losses_increment'] = 1
            else:  # Push
                update_data['chips_increment'] = self.bet_amount

            prev_xp = int(user_after_bet['current_xp']) if user_after_bet else 0
            prev_prestige = int(user_after_bet['prestige']) if user_after_bet else 0
            from utils.levels import get_user_level
            prev_level = get_user_level(prev_xp, prev_prestige)

            final_user_data = await update_user(player_id, **update_data)
            new_xp = int(final_user_data['current_xp']) if final_user_data else prev_xp
            new_level = get_user_level(new_xp, prev_prestige)

            # Notifications
            from utils.embeds import level_up_embed, prestige_ready_embed
            from utils.constants import MAX_CURRENT_XP
            from utils import notifications
            if new_level > prev_level and notifications.should_announce_level_up(interaction.user.id, new_level):
                try:
                    await interaction.followup.send(content=f"{interaction.user.mention}", embed=level_up_embed(new_level))
                except Exception:
                    pass
            if (prev_xp < MAX_CURRENT_XP and new_xp >= MAX_CURRENT_XP
                and notifications.should_announce_prestige_ready(interaction.user.id, prev_prestige)):
                try:
                    await interaction.followup.send(content=f"{interaction.user.mention}", embed=prestige_ready_embed())
                except Exception:
                    pass
            
            # Clear cache after update
            clear_user_cache(player_id)
            
            embed = await embeds.create_baccarat_embed(
                game_over=True,
                player_hand=results['player_hand'],
                banker_hand=results['banker_hand'],
                player_score=results['player_score'],
                banker_score=results['banker_score'],
                bet=self.bet_amount,
                outcome_text=results['result_text'],
                new_balance=final_user_data['chips'],
                profit=profit,
                xp_gain=xp_gain,
                choice=choice,
                user=interaction.user
            )
            
            await interaction.edit_original_message(embed=embed, view=None)
            
        except Exception as e:
            # Re-enable buttons on error
            self._game_started = False
            for item in self.children:
                item.disabled = False
            await interaction.edit_original_message(view=self)
            raise e
        finally:
            # Clean up from active games
            self.active_games.pop(player_id, None)

    @button(label="Player", style=nextcord.ButtonStyle.green, custom_id="player_bet")
    async def player_bet(self, button: Button, interaction: nextcord.Interaction) -> None:
        await self.start_baccarat_game(interaction, "player")

    @button(label="Banker", style=nextcord.ButtonStyle.red, custom_id="banker_bet")
    async def banker_bet(self, button: Button, interaction: nextcord.Interaction) -> None:
        await self.start_baccarat_game(interaction, "banker")

    @button(label="Tie", style=nextcord.ButtonStyle.secondary, custom_id="tie_bet")
    async def tie_bet(self, button: Button, interaction: nextcord.Interaction) -> None:
        await self.start_baccarat_game(interaction, "tie")

    async def on_timeout(self) -> None:
        # Check if the game has already completed (player_id not in active_games)
        player_id = self.author.id
        if player_id not in self.active_games:
            # Game already completed, don't show timeout message
            self.stop()
            return
            
        # Handle timeout - disable buttons
        for item in self.children:
            item.disabled = True
        
        if self.message:
            timeout_embed = create_game_timeout_embed(self.bet_amount)
            try:
                await self.message.edit(embed=timeout_embed, view=self)
            except (nextcord.HTTPException, nextcord.NotFound):
                pass  # Message was deleted or couldn't be edited
        
        # Clean up from active games
        if player_id in self.active_games:
            del self.active_games[player_id]
        self.stop()

class HigherOrLowerView(BaseGameView):
    """Optimized Higher or Lower view with efficient button state management."""
    
    def __init__(self, author: nextcord.User, game_logic: "HigherOrLowerGameLogic"):
        super().__init__(author, game_logic, timeout=300)
        self._setup_buttons()
        self.update_button_state()

    def _setup_buttons(self):
        """Setup game buttons using the new action system."""
        actions = [
            GameAction("Higher 🔼", nextcord.ButtonStyle.success, self._higher_callback, "higher"),
            GameAction("Lower 🔽", nextcord.ButtonStyle.danger, self._lower_callback, "lower"),
            GameAction("Cash Out 💰", nextcord.ButtonStyle.primary, self._cash_out_callback, "cash_out", row=1),
        ]
        
        for action in actions:
            self.add_game_action(action)

    def update_button_state(self):
        """Efficiently update button states based on game state."""
        if self.game_logic:
            cash_out_disabled = self.game_logic.streak == 0
            for item in self.children:
                if isinstance(item, Button) and item.custom_id == "cash_out":
                    item.disabled = cash_out_disabled
                    break

    async def _higher_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.handle_guess(interaction, "higher")

    async def _lower_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.handle_guess(interaction, "lower")

    async def _cash_out_callback(self, game_logic, interaction: nextcord.Interaction):
        if game_logic:
            await game_logic.cash_out(interaction)

    async def on_timeout(self):
        """Enhanced timeout with game-specific handling."""
        if self.game_logic and hasattr(self.game_logic, 'handle_timeout'):
            await self.game_logic.handle_timeout()
        await super().on_timeout()

class PrestigeConfirmation(TimeoutView):
    def __init__(self, author: nextcord.User):
        super().__init__(author, timeout=60)
        self.message: nextcord.Message = None

    @button(label="Continue", style=nextcord.ButtonStyle.green)
    async def confirm_prestige(self, button: Button, interaction: nextcord.Interaction):
        # Logic to handle the prestige is now in the button callback
        user_data = await get_user(interaction.user.id)
        new_prestige = user_data['prestige'] + 1
        
        await update_user(
            interaction.user.id,
            prestige=new_prestige,
            current_xp=0,
            chips=STARTING_CHIPS
        )
        try:
            from utils.notifications import reset_prestige_ready
            reset_prestige_ready(interaction.user.id)
        except Exception:
            pass
        
        for child in self.children:
            child.disabled = True
            
        await interaction.response.edit_message(
            content=f"**Congratulations!** You have reached Prestige level **{new_prestige}**! Your <:chips:1396988413151940629> and XP have been reset.",
            view=self,
            embed=None # Clear the embed
        )
        self.stop()

    @button(label="Cancel", style=nextcord.ButtonStyle.red)
    async def cancel_prestige(self, button: Button, interaction: nextcord.Interaction):
        for child in self.children:
            child.disabled = True
        
        # Update the embed to show a chicken emoji
        original_embed = self.message.embeds
        original_embed.description = "🐔"
        
        await interaction.response.edit_message(embed=original_embed, view=self)
        self.stop()

class ProfileAchievementsPaginator(View):
    """Ephemeral paginator for a user's earned achievements.
    Uses Discord timestamp rendering for viewer-local time display.
    """
    def __init__(self, viewer: nextcord.User, target: nextcord.User, pages: List[nextcord.Embed]):
        super().__init__(timeout=120)
        self.viewer = viewer
        self.target = target
        self.pages = pages
        self.index = 0
        # Disable nav if only one page
        if len(self.pages) == 1:
            for item in self.children:
                item.disabled = True

    async def interaction_check(self, interaction: nextcord.Interaction) -> bool:
        if interaction.user.id != self.viewer.id:
            await interaction.response.send_message("You cannot control someone else's paginator.", ephemeral=True)
            return False
        return True

    @button(label="Prev", style=nextcord.ButtonStyle.secondary)
    async def prev_btn(self, button: Button, interaction: nextcord.Interaction):  # type: ignore
        self.index = (self.index - 1) % len(self.pages)
        await interaction.response.edit_message(embed=self.pages[self.index], view=self)

    @button(label="Next", style=nextcord.ButtonStyle.secondary)
    async def next_btn(self, button: Button, interaction: nextcord.Interaction):  # type: ignore
        self.index = (self.index + 1) % len(self.pages)
        await interaction.response.edit_message(embed=self.pages[self.index], view=self)

    async def on_timeout(self):
        for child in self.children:
            if isinstance(child, Button):
                child.disabled = True
        # We cannot edit after ephemeral deletion; ignore failures.


def _build_achievement_pages(target: nextcord.User, earned: List[Dict[str, Any]]) -> List[nextcord.Embed]:
    """Build paginated embeds for achievements (earned list only), grouped by category.

    - Categories are sorted alphabetically.
    - Within each category, achievements are sorted by requirement_value then name.
    - Pagination is per category with a global page counter across all categories.
    """
    per_page = 8
    pages: List[nextcord.Embed] = []
    total_earned = len(earned)

    if total_earned == 0:
        embed = _create_branded_embed(
            title=f"🏆 {target.display_name}'s Achievements",
            description="No achievements earned yet.",
            color=nextcord.Color.greyple()
        )
        pages.append(embed)
        return pages

    # Group by category
    categories = sorted({(rec.get('category') or 'Other') for rec in earned}, key=lambda s: s.lower())

    # Prepare sorted lists per category
    cat_to_earned: Dict[str, List[Dict[str, Any]]] = {}
    for cat in categories:
        cat_items = [r for r in earned if (r.get('category') or 'Other') == cat]
        # Sort by requirement_value then name for consistency with main achievements view
        cat_items.sort(key=lambda r: (r.get('requirement_value', 0) or 0, str(r.get('name', '')).lower()))
        cat_to_earned[cat] = cat_items

    # Compute total page count across categories
    def pages_for(n: int) -> int:
        return (n - 1) // per_page + 1 if n > 0 else 1

    total_pages = sum(pages_for(len(items)) for items in cat_to_earned.values())
    page_counter = 0

    for cat in categories:
        items = cat_to_earned[cat]
        total_cat = len(items)
        for i in range(0, total_cat, per_page):
            chunk = items[i:i+per_page]
            page_counter += 1
            embed = _create_branded_embed(
                title=f"🏆 {target.display_name}'s Achievements • {cat}",
                description=f"Showing {i+1}-{i+len(chunk)} of {total_cat} • Page {page_counter}/{total_pages}",
                color=nextcord.Color.gold()
            )
            for record in chunk:
                ts = record.get('earned_at')
                ts_str = f"<t:{int(ts.timestamp())}:f>" if ts else "Unknown time"
                name = record.get('name', 'Unknown')
                icon = record.get('icon', '🏆')
                embed.add_field(
                    name=f"{icon} {name}",
                    value=f"Earned: {ts_str}",
                    inline=False
                )
            embed.set_footer(text=f"High Roller Club • Page {page_counter}/{total_pages}")
            pages.append(embed)
    return pages


class ProfileView(View):
    """View attached to profile embeds allowing users to open achievements paginator on demand."""
    def __init__(self, requester: nextcord.User, target: nextcord.User):
        super().__init__(timeout=90)
        self.requester = requester
        self.target = target

    async def interaction_check(self, interaction: nextcord.Interaction) -> bool:
        # Anyone can press; paginator will be ephemeral to the presser.
        return True

    @button(label="View Achievements", style=nextcord.ButtonStyle.blurple)
    async def achievements_btn(self, button: Button, interaction: nextcord.Interaction):  # type: ignore
        try:
            from .database import get_user_achievements
            earned = await get_user_achievements(self.target.id)
            # earned is list of rows including icon, name, earned_at (already used elsewhere)
            pages = _build_achievement_pages(self.target, earned)
            paginator = ProfileAchievementsPaginator(interaction.user, self.target, pages)
            await interaction.response.send_message(embed=pages[0], view=paginator, ephemeral=True)
        except Exception:
            await interaction.response.send_message("Failed to load achievements.", ephemeral=True)

    async def on_timeout(self):
        for child in self.children:
            if isinstance(child, Button):
                child.disabled = True
        # Cannot edit original if deleted; ignore.
        self.stop()
