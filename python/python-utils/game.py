import nextcord
from typing import Dict, Any, Optional
from asyncpg import Record
import math
import time

from . import database as db
from . import embeds
from .constants import XP_PER_PROFIT
from .constants import MAX_CURRENT_XP
from .levels import get_user_level
from . import embeds as embed_utils
from . import notifications

class Game:
    """
    A base class for all casino games to handle common logic like
    bet validation, player data management, and final database updates.
    """
    
    # Class-level debouncing for achievement checks
    _last_achievement_check: Dict[int, float] = {}
    _achievement_debounce_seconds = 30  # Only check achievements every 30 seconds per user
    _last_cleanup = 0  # Track when we last cleaned the debounce cache

    def __init__(self, interaction: nextcord.Interaction, bet: int):
        self.interaction = interaction
        self.user = interaction.user
        self.bet = bet
        self.user_data: Record = {}
        self.is_game_over = False
        # Minimum fraction of pre-game chips required for a game result
        # to count towards wins/losses. Can be overridden by subclasses.
        self.count_win_loss_min_ratio: float = 0.0

    async def validate_bet(self) -> bool:
        """
        Validates if the player has enough chips for the initial bet.
        This does NOT deduct the bet.
        """
        self.user_data = await db.get_user(self.user.id)
        if self.user_data['chips'] < self.bet:
            embed = embeds.insufficient_chips_embed(
                required_chips=self.bet,
                current_balance=self.user_data['chips'],
                bet_description=f"that bet ({self.bet:,} chips)"
            )
            # If we haven't responded yet, send an initial ephemeral response.
            # Otherwise, use followup to avoid Unknown Webhook errors.
            if not self.interaction.response.is_done():
                await self.interaction.response.send_message(embed=embed, ephemeral=True)
            else:
                await self.interaction.followup.send(embed=embed, ephemeral=True)
            return False
        return True

    async def end_game(self, profit: int) -> Optional[Record]:
        """
        Finalizes the game, calculates final stats, updates the database,
        and returns the updated user record.

        :param profit: The net profit for the player. This should be
                       positive for a win and negative for a loss.
        :return: The updated user data record from the database.
        """
        if self.is_game_over:
            return None
        self.is_game_over = True

        xp_gain = 0
        wins_increment = 0
        losses_increment = 0

        if profit > 0:
            xp_gain = int(profit * XP_PER_PROFIT)
        
        # Determine whether this game's outcome should count towards W/L
        should_count_wl = True
        try:
            pre_game_chips = int(self.user_data['chips'])
        except Exception:
            try:
                # Fallback if validate_bet wasn't called for some reason
                pre_game_chips = int((await db.get_user(self.user.id))['chips'])
            except Exception:
                pre_game_chips = None
        
        if self.count_win_loss_min_ratio > 0.0 and pre_game_chips is not None:
            required_bet = math.ceil(pre_game_chips * self.count_win_loss_min_ratio)
            should_count_wl = self.bet >= required_bet

        if profit > 0 and should_count_wl:
            wins_increment = 1
        elif profit < 0 and should_count_wl:
            losses_increment = 1

        update_data = {
            'chips_increment': profit,
            'wins_increment': wins_increment,
            'losses_increment': losses_increment,
            'total_xp_increment': xp_gain,
        }

        # Cap current_xp at MAX_CURRENT_XP (do not overflow beyond last rank pre-prestige)
        if xp_gain > 0:
            try:
                current_xp = int(self.user_data['current_xp'])
            except Exception:
                current_xp = int((await db.get_user(self.user.id))['current_xp'])
            if current_xp < MAX_CURRENT_XP:
                allowed = min(xp_gain, MAX_CURRENT_XP - current_xp)
                if allowed > 0:
                    update_data['current_xp_increment'] = allowed
        
        
        # Filter out zero-value updates to avoid unnecessary database writes
        update_data = {k: v for k, v in update_data.items() if v != 0}

        prev_current_xp = int(self.user_data.get('current_xp', 0)) if self.user_data else 0
        prev_level = get_user_level(prev_current_xp, int(self.user_data.get('prestige', 0))) if self.user_data else 0

        if update_data:
            updated_user_data = await db.update_user(self.user.id, **update_data)
            # Post-update notifications (level up / prestige ready)
            try:
                if updated_user_data:
                    new_current_xp = int(updated_user_data['current_xp'])
                    prestige = int(updated_user_data['prestige'])
                    new_level = get_user_level(new_current_xp, prestige)

                    # Level-up message
                    if new_level > prev_level and notifications.should_announce_level_up(self.user.id, new_level):
                        try:
                            await self.interaction.followup.send(
                                content=f"{self.user.mention}",
                                embed=embed_utils.level_up_embed(new_level),
                                ephemeral=False
                            )
                        except Exception:
                            pass

                    # Prestige ready indicator (just hit cap)
                    if (prev_current_xp < MAX_CURRENT_XP and new_current_xp >= MAX_CURRENT_XP and
                        notifications.should_announce_prestige_ready(self.user.id, prestige)):
                        try:
                            await self.interaction.followup.send(
                                content=f"{self.user.mention}",
                                embed=embed_utils.prestige_ready_embed(),
                                ephemeral=False
                            )
                        except Exception:
                            pass
            except Exception:
                pass
            
            # Check for achievements after updating user stats (with debouncing)
            try:
                current_time = time.time()
                
                # Periodic cleanup of old achievement check timestamps (every 5 minutes)
                if current_time - Game._last_cleanup > 300:
                    Game._cleanup_achievement_cache(current_time)
                    Game._last_cleanup = current_time
                
                last_check = self._last_achievement_check.get(self.user.id, 0)
                
                if current_time - last_check >= self._achievement_debounce_seconds:
                    from .achievements_config import check_and_award_achievements
                    from . import embeds as embed_utils
                    
                    # Update last check time
                    self._last_achievement_check[self.user.id] = current_time
                    
                    # Check regular achievements
                    newly_awarded = await check_and_award_achievements(self.user.id)
                    
                    # Check for big win achievements if profit is significant
                    if profit >= 50000:  # Check big win achievements
                        from .database import check_achievement_progress, award_achievement
                        big_win_achievements = await check_achievement_progress(
                            self.user.id, "big_win", profit
                        )
                        for achievement in big_win_achievements:
                            awarded = await award_achievement(self.user.id, achievement["id"])
                            if awarded:
                                newly_awarded.append({
                                    "id": awarded["id"],
                                    "name": awarded["name"],
                                    "description": awarded["description"],
                                    "icon": awarded["icon"],
                                    "chips_reward": awarded["chips_reward"],
                                    "xp_reward": awarded["xp_reward"]
                                })
                    
                    if newly_awarded:
                        # Send achievement notification as a reply to the original interaction
                        achievement_embed = embed_utils.create_achievement_notification_embed(newly_awarded, self.user)
                        try:
                            # Try to reply to the original interaction message if possible
                            original_message = await self.interaction.original_message()
                            await original_message.reply(embed=achievement_embed, mention_author=True)
                        except (nextcord.NotFound, nextcord.HTTPException):
                            # Fallback to followup if reply fails
                            await self.interaction.followup.send(embed=achievement_embed, ephemeral=False)
                    
            except Exception as e:
                # Silently fail achievements to not disrupt gameplay
                import logging
                logging.error(f"Failed to check achievements for user {self.user.id}: {e}")
            
            return updated_user_data
        
        return self.user_data

    @classmethod
    def _cleanup_achievement_cache(cls, current_time: float) -> None:
        """Clean up old entries from the achievement check cache to prevent memory leaks."""
        cutoff_time = current_time - (cls._achievement_debounce_seconds * 2)  # Keep entries for 2x debounce time
        old_keys = [user_id for user_id, timestamp in cls._last_achievement_check.items() if timestamp < cutoff_time]
        
        for user_id in old_keys:
            del cls._last_achievement_check[user_id]
            
        if old_keys:
            import logging
            logging.info(f"Cleaned up {len(old_keys)} old achievement check entries. Active: {len(cls._last_achievement_check)}")
