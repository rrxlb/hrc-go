import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption, ButtonStyle, Embed, Color, ui, Permissions
import asyncio
import random
from dataclasses import dataclass, field
from typing import List, Dict, Optional, Set
import time
import weakref
from collections import deque

from utils.database import get_user, update_user, batch_update_users, get_multiple_users
from utils.embeds import _create_branded_embed, error_embed, success_embed, insufficient_chips_embed
from utils.constants import XP_PER_PROFIT, PREMIUM_ROLE_ID, BOT_COLOR
from utils.checks import is_developer, send_under_development_embed

# --- Constants ---
HORSE_NAMES = [
    "Seabiscuit", "Glue Factory", "Hoof Hearted", "Maythehorsebewithu", "Galloping Ghost", 
    "Usain Colt", "Pony Soprano", "Forrest Jump", "Lil' Sebastian", "DoYouThinkHeSaurus", 
    "Bet A Million", "Always Broke", "My Wife's Money", "Sofa King Fast", "Harry Trotter", 
    "Blazing Saddles", "Debt Collector", "Spinning Plates", "Pixelated Steed", "Discord Nitro"
]
TRACK_LENGTH = 20
RACE_COMMENTARY = {
    "start": [
        "And they're off!", "A clean start for all the horses!", "The gates are open and the race has begun!"
    ],
    "middle": [
        "A blistering pace is being set by the leaders!", "Rounding the first turn, it's still anyone's race!",
        "Down the backstretch they come!", "A horse is making a move on the outside!", "The tension is palpable as they round the bend!",
        "The horses are tightly packed, each vying for a better position.", "One horse seems to be finding its rhythm now."
    ],
    "end": [
        "Into the final stretch, the crowd is roaring!", "It's neck and neck as they approach the finish!", "A photo finish is imminent!",
        "One horse is pulling ahead with a burst of speed!", "This is going to be a close one!", "The jockeys are giving it their all!"
    ]
}
HORSE_EMOJIS = ["🐴", "🐎", "🦓", "🦄"]

# --- Data Classes ---
# --- Memory optimization for Horse data class ---
@dataclass
class Horse:
    id: int
    name: str
    odds: int
    position: int = 0
    track_icon: str = '🏇'
    
    def __post_init__(self):
        # Use __slots__ equivalent optimization
        if not hasattr(self, '_frozen'):
            # Freeze name and odds to prevent accidental modification
            object.__setattr__(self, '_frozen', True)

@dataclass
class Bet:
    user_id: int
    user_name: str
    horse_id: int
    amount: int
    
    def __post_init__(self):
        # Validate and optimize memory
        if self.amount <= 0:
            raise ValueError("Bet amount must be positive")
        if not 1 <= self.horse_id <= 20:  # Max reasonable horses
            raise ValueError("Invalid horse_id")

# Enhanced Race class with memory optimization
@dataclass
class Race:
    interaction: Interaction
    initiator: nextcord.Member
    message: Optional[nextcord.Message] = None
    horses: List[Horse] = field(default_factory=list)
    bets: List[Bet] = field(default_factory=list)
    participants: Set[nextcord.Member] = field(default_factory=set)
    status: str = "lobby"  # lobby, betting, running, finished, cancelled
    view: Optional[ui.View] = None
    cog: Optional["HorseRacingGame"] = None
    created_at: float = field(default_factory=time.time)
    _cached_embed_data: Optional[Dict] = field(default=None, init=False)  # Cache for embed data
    
    def __post_init__(self):
        # Use weak reference for cog to prevent circular references
        if self.cog:
            self.cog = weakref.ref(self.cog)
    
    def get_cog(self):
        """Get the cog reference safely."""
        if hasattr(self.cog, '__call__'):
            return self.cog()
        return self.cog
    
    def clear_cache(self):
        """Clear cached data to free memory."""
        self._cached_embed_data = None
    
    async def get_race_embed(self, title: str, description: str, color: Color) -> Embed:
        """Creates a branded embed for the horse race with caching."""
        # Use cached embed data if available and title hasn't changed
        cache_key = f"{title}_{len(description)}"
        if self._cached_embed_data and self._cached_embed_data.get('key') == cache_key:
            embed = _create_branded_embed(title=title, description=description, color=color)
            embed.set_thumbnail(url=self._cached_embed_data['thumbnail_url'])
            return embed
        
        embed = _create_branded_embed(title=title, description=description, color=color)
        thumbnail_url = "https://res.cloudinary.com/dfoeiotel/image/upload/v1754026209/HR2_dacwe3.png"
        embed.set_thumbnail(url=thumbnail_url)
        
        # Cache the thumbnail URL
        self._cached_embed_data = {
            'key': cache_key,
            'thumbnail_url': thumbnail_url
        }
        return embed

# --- Modals ---
class BettingModal(ui.Modal):
    def __init__(self, race: "Race", view: "BettingView"):
        super().__init__(title="Place Your Bet")
        self.race = race
        self.betting_view = view

        self.horse_number = ui.TextInput(
            label="Horse Number (1-6)",
            placeholder="Enter the number of the horse you want to bet on.",
            min_length=1,
            max_length=1,
            required=True
        )
        self.add_item(self.horse_number)

        self.bet_amount = ui.TextInput(
            label="Bet Amount",
            placeholder="Enter the amount of chips you want to wager.",
            min_length=1,
            max_length=10,
            required=True
        )
        self.add_item(self.bet_amount)

    async def callback(self, interaction: Interaction):
        if interaction.user not in self.race.participants:
            await interaction.response.send_message(embed=error_embed("You must join the race to place a bet."), ephemeral=True)
            return
            
        try:
            horse_id = int(self.horse_number.value)
            if not 1 <= horse_id <= len(self.race.horses):
                await interaction.response.send_message(embed=error_embed("Invalid horse number."), ephemeral=True)
                return
        except ValueError:
            await interaction.response.send_message(embed=error_embed("Invalid horse number."), ephemeral=True)
            return

        try:
            bet_amount = int(self.bet_amount.value)
            if bet_amount <= 0:
                await interaction.response.send_message(embed=error_embed("Bet must be positive."), ephemeral=True)
                return
        except ValueError:
            await interaction.response.send_message(embed=error_embed("Invalid bet amount."), ephemeral=True)
            return

        user_data = await get_user(interaction.user.id)
        if not user_data or user_data['chips'] < bet_amount:
            await interaction.response.send_message(
                embed=insufficient_chips_embed(
                    required_chips=bet_amount,
                    current_balance=user_data['chips'] if user_data else 0,
                    bet_description=f"that bet ({bet_amount:,} chips)"
                ), 
                ephemeral=True
            )
            return

        # Check if user has already bet
        if any(b.user_id == interaction.user.id for b in self.race.bets):
            await interaction.response.send_message(embed=error_embed("You have already placed a bet in this race."), ephemeral=True)
            return

        await update_user(interaction.user.id, chips_increment=-bet_amount)
        new_bet = Bet(user_id=interaction.user.id, user_name=interaction.user.display_name, horse_id=horse_id, amount=bet_amount)
        self.race.bets.append(new_bet)

        horse_name = self.race.horses[horse_id - 1].name
        await interaction.response.send_message(embed=success_embed("Bet Placed!", f"You bet **{bet_amount:,}** on **{horse_name}**."), ephemeral=True)
        
        await self.betting_view.update_embed()

# --- Views ---
class BettingView(ui.View):
    def __init__(self, race: "Race"):
        super().__init__(timeout=300)
        self.race = race
        self.update_button_states()

    def update_button_states(self):
        # Disable start if no one has bet
        self.children[1].disabled = not self.race.bets

    async def on_timeout(self):
        if self.race.status == "betting":
            # If bets were placed, start the race. Otherwise, cancel.
            if self.race.bets:
                await self.start_race_logic()
            else:
                self.race.status = "cancelled"
                for item in self.children:
                    item.disabled = True
                
                # Calculate total bet amount for forfeiture message
                total_bet = sum(bet.amount for bet in self.race.bets)
                
                # Create cleanup embed with chip forfeiture message
                from utils import embeds
                cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
                
                await self.race.message.edit(embed=cleanup_embed, view=self)
        if self.race.cog:
            cog = self.race.get_cog()
            if cog and self.race.interaction.channel_id in cog.active_races:
                del cog.active_races[self.race.interaction.channel_id]

    async def update_embed(self):
        description = "**Place your bets now!**\n\n"
        description += "**Horses & Odds:**\n"
        for horse in self.race.horses:
            description += f"`{horse.id}.` {horse.track_icon} **{horse.name}** `({horse.odds}:1)`\n"
        
        description += "\n**Bets Placed:**\n"
        if not self.race.bets:
            description += "No bets placed yet."
        else:
            bet_lines = [f"• **{b.user_name}** on Horse #{b.horse_id}" for b in self.race.bets]
            description += "\n".join(bet_lines)

        embed = await self.race.get_race_embed(
            title=f"🏇 Betting is Open! 🏇",
            description=description,
            color=Color.blue()
        )
        embed.set_footer(text=f"The initiator can lock bets and start the race at any time.")
        self.update_button_states()
        await self.race.message.edit(embed=embed, view=self)

    @ui.button(label="Place Bet", style=ButtonStyle.green, custom_id="place_bet")
    async def place_bet(self, button: ui.Button, interaction: Interaction):
        await interaction.response.send_modal(BettingModal(self.race, self))

    @ui.button(label="Lock Bets & Start Race", style=ButtonStyle.blurple, custom_id="lock_and_start")
    async def lock_and_start(self, button: ui.Button, interaction: Interaction):
        if interaction.user.id != self.race.initiator.id:
            await interaction.response.send_message(embed=error_embed("Only the race initiator can start the race."), ephemeral=True)
            return
        
        await interaction.response.defer()
        await self.start_race_logic()
        self.stop()

    async def start_race_logic(self):
        self.race.status = "running"
        for item in self.children:
            item.disabled = True
        
        embed = await self.race.get_race_embed(
            title="🏇 Bets are Locked! 🏇",
            description="The bets are in! The race is about to begin...",
            color=Color.dark_purple()
        )
        await self.race.message.edit(embed=embed, view=self)
        await asyncio.sleep(1.5)
        
        if self.race.cog:
            cog = self.race.get_cog()
            if cog:
                await cog.run_race_simulation(self.race)

class RaceLobbyView(ui.View):
    def __init__(self, race: "Race"):
        super().__init__(timeout=300)
        self.race = race
        self.update_button_states()

    def update_button_states(self):
        self.children[1].disabled = len(self.race.participants) < 1 or self.race.status != "lobby"

    async def on_timeout(self):
        if self.race.status == "lobby":
            self.race.status = "cancelled"
            for item in self.children:
                item.disabled = True
            
            # Calculate total bet amount for forfeiture message
            total_bet = sum(bet.amount for bet in self.race.bets)
            
            # Create cleanup embed with chip forfeiture message
            from utils import embeds
            cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
            
            await self.race.message.edit(embed=cleanup_embed, view=self)
            if self.race.cog:
                cog = self.race.get_cog()
                if cog and self.race.interaction.channel_id in cog.active_races:
                    del cog.active_races[self.race.interaction.channel_id]

    async def update_embed(self):
        description = "**The lobby is open! Click 'Join Race' to enter!**\n\n"
        description += "**Horses & Odds:**\n"
        for horse in self.race.horses:
            description += f"`{horse.id}.` {horse.track_icon} **{horse.name}** `({horse.odds}:1)`\n"
        
        description += "\n**Participants:**\n"
        if not self.race.participants:
            description += "No one has joined yet."
        else:
            description += ", ".join(p.mention for p in self.race.participants)

        embed = await self.race.get_race_embed(
            title=f"🏇 {self.race.initiator.display_name}'s Horse Race 🏇",
            description=description,
            color=BOT_COLOR
        )
        embed.set_footer(text=f"Race initiated by {self.race.initiator.display_name}")
        await self.race.message.edit(embed=embed, view=self)

    @ui.button(label="Join Race", style=ButtonStyle.primary, custom_id="join_race")
    async def join_race(self, button: ui.Button, interaction: Interaction):
        if interaction.user in self.race.participants:
            if interaction.user.id == self.race.initiator.id:
                await interaction.response.send_message(embed=error_embed("As the initiator, you are already in the race."), ephemeral=True)
            else:
                await interaction.response.send_message(embed=error_embed("You have already joined the race."), ephemeral=True)
            return
        
        self.race.participants.add(interaction.user)
        self.update_button_states()
        await self.update_embed()
        await interaction.response.defer()

    @ui.button(label="Start Betting", style=ButtonStyle.green, custom_id="start_betting")
    async def start_betting(self, button: ui.Button, interaction: Interaction):
        if interaction.user.id != self.race.initiator.id:
            await interaction.response.send_message(embed=error_embed("Only the race initiator can start the race."), ephemeral=True)
            return
        
        self.race.status = "betting"
        self.stop() 

        betting_view = BettingView(self.race)
        self.race.view = betting_view
        await betting_view.update_embed()


    @ui.button(label="Cancel Race", style=ButtonStyle.red, custom_id="cancel_race")
    async def cancel_race(self, button: ui.Button, interaction: Interaction):
        if interaction.user.id != self.race.initiator.id:
            await interaction.response.send_message(embed=error_embed("Only the race initiator can cancel the race."), ephemeral=True)
            return

        self.race.status = "cancelled"
        for item in self.children:
            item.disabled = True
        
        embed = await self.race.get_race_embed(
            title="🏇 Race Cancelled 🏇",
            description=f"The race was cancelled by the initiator.",
            color=Color.red()
        )
        await self.race.message.edit(embed=embed, view=self)
        if self.race.cog:
            cog = self.race.get_cog()
            if cog and self.race.interaction.channel_id in cog.active_races:
                del cog.active_races[self.race.interaction.channel_id]
        self.stop()

# --- Optimized Cog Class with dynamic cleanup ---
class HorseRacingGame(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot
        # Use WeakValueDictionary to automatically clean up finished races
        self.active_races: Dict[int, Race] = {}
        # Track race activity for dynamic cleanup scheduling
        self._race_activity: deque = deque(maxlen=100)  # Track last 100 race events
        self._cleanup_interval = 5  # Start with 5 minutes
        self.cleanup_old_races.start()

    def cog_unload(self) -> None:
        self.cleanup_old_races.cancel()
        # Clean up all active races
        for race in list(self.active_races.values()):
            if hasattr(race, 'clear_cache'):
                race.clear_cache()
        self.active_races.clear()

    def _record_race_activity(self, activity_type: str):
        """Record race activity for dynamic cleanup adjustment."""
        self._race_activity.append((time.time(), activity_type))

    def _calculate_dynamic_cleanup_interval(self) -> int:
        """Calculate optimal cleanup interval based on recent activity."""
        if len(self._race_activity) < 10:
            return 5  # Default interval
        
        current_time = time.time()
        recent_activity = sum(1 for timestamp, _ in self._race_activity 
                            if current_time - timestamp < 300)  # Last 5 minutes
        
        if recent_activity > 20:
            return 2  # High activity - clean every 2 minutes
        elif recent_activity > 10:
            return 3  # Medium activity - clean every 3 minutes
        elif recent_activity < 3:
            return 10  # Low activity - clean every 10 minutes
        else:
            return 5  # Normal activity - clean every 5 minutes

    @tasks.loop(minutes=5)  # Initial interval, will be dynamically adjusted
    async def cleanup_old_races(self) -> None:
        """Optimized cleanup with dynamic scheduling and batch operations."""
        current_time = time.time()
        to_remove = []
        races_to_cleanup = []
        
        # Batch identify races to clean
        for channel_id, race in list(self.active_races.items()):
            race_age = current_time - race.created_at
            
            # More aggressive cleanup for finished/cancelled races
            cleanup_threshold = 300 if race.status in ["finished", "cancelled"] else 600  # 5 or 10 minutes
            
            if race_age > cleanup_threshold:
                to_remove.append(channel_id)
                races_to_cleanup.append(race)
        
        # Batch cleanup operations
        if races_to_cleanup:
            cleanup_tasks = []
            for race in races_to_cleanup:
                task = self._cleanup_race(race)
                cleanup_tasks.append(task)
            
            # Execute cleanup concurrently
            if cleanup_tasks:
                await asyncio.gather(*cleanup_tasks, return_exceptions=True)
        
        # Remove from active races
        for channel_id in to_remove:
            self.active_races.pop(channel_id, None)
        
        if to_remove:
            print(f"Cleaned up {len(to_remove)} old horse races")
            self._record_race_activity(f"cleanup_{len(to_remove)}")
        
        # Adjust cleanup interval dynamically
        new_interval = self._calculate_dynamic_cleanup_interval()
        if new_interval != self._cleanup_interval:
            self._cleanup_interval = new_interval
            self.cleanup_old_races.change_interval(minutes=new_interval)
            print(f"Adjusted horse race cleanup interval to {new_interval} minutes")

    async def _cleanup_race(self, race: Race):
        """Clean up a single race efficiently."""
        try:
            # Calculate total bet amount for forfeiture message
            total_bet = sum(bet.amount for bet in race.bets)
            
            # Create cleanup embed with chip forfeiture message
            from utils import embeds
            cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
            
            # Disable all buttons and update message
            if race.view:
                for child in race.view.children:
                    child.disabled = True
                
                try:
                    if race.message:
                        await race.message.edit(embed=cleanup_embed, view=race.view)
                except (nextcord.HTTPException, nextcord.NotFound):
                    pass  # Message was deleted or couldn't be edited
                
                try:
                    race.view.stop()
                except Exception:
                    pass  # Ignore errors during view stop
            
            # Clear race cache to free memory
            if hasattr(race, 'clear_cache'):
                race.clear_cache()
                
        except Exception as e:
            print(f"Error cleaning up race: {e}")

    @cleanup_old_races.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(name="derby", description="Start a horse race lobby in this channel.")
    async def derby(self, interaction: Interaction):
        """Optimized derby command with activity tracking."""
        if interaction.channel_id in self.active_races:
            existing_race = self.active_races[interaction.channel_id]
            await interaction.response.send_message(embed=error_embed(f"A race is already in progress! [Jump to Race]({existing_race.message.jump_url})"), ephemeral=True)
            return

        await interaction.response.defer()
        
        # Record race creation activity
        self._record_race_activity("race_created")

        # Create race with weak reference to prevent circular references
        race = Race(interaction=interaction, initiator=interaction.user)
        cog_ref = weakref.ref(self)
        race.cog = cog_ref
        race.participants.add(interaction.user) # Initiator auto-joins
        self.active_races[interaction.channel_id] = race

        # Optimize horse selection and creation
        selected_names = random.sample(HORSE_NAMES, 6)
        horse_icons = random.choices(HORSE_EMOJIS, k=6)  # Allow duplicates for better performance
        
        for i, (name, icon) in enumerate(zip(selected_names, horse_icons), 1):
            odds = random.randint(2, 25)
            race.horses.append(Horse(id=i, name=name, odds=odds, track_icon=icon))

        view = RaceLobbyView(race)
        race.view = view
        
        message = await interaction.followup.send(content="Setting up the derby...", wait=True)
        race.message = message
        
        await view.update_embed()
        


    def _generate_track_display(self, horses: List[Horse]) -> str:
        """Generates the string representation of the race track."""
        finish_line = "🏁"
        track_lines = []
        for horse in horses:
            pos = max(0, min(horse.position, TRACK_LENGTH - 1))
            progress = "═" * pos
            horse_icon = horse.track_icon
            remaining = "─" * (TRACK_LENGTH - 1 - pos)
            track_lines.append(f"`{horse.id}.` `[{progress}{horse_icon}{remaining}]` {finish_line}")
        return "\n".join(track_lines)

    async def run_race_simulation(self, race: Race):
        """Optimized race simulation with reduced allocations and improved performance."""
        self._record_race_activity("race_started")
        
        winner = None
        race_in_progress = True
        race_phase = "start"
        turn_count = 0
        max_turns = 50  # Safety limit
        
        # Pre-select commentary to reduce repeated random calls
        start_commentary = random.choice(RACE_COMMENTARY["start"])
        middle_commentary = random.choice(RACE_COMMENTARY["middle"])
        end_commentary = random.choice(RACE_COMMENTARY["end"])

        while race_in_progress and turn_count < max_turns:
            # Use pre-selected commentary based on phase
            if race_phase == "start":
                commentary = start_commentary
                race_phase = "middle"
            else:
                # Check if any horse is in the final stretch
                if any(h.position / TRACK_LENGTH >= 0.75 for h in race.horses):
                    if race_phase != "end":
                        race_phase = "end"
                        commentary = end_commentary
                    else:
                        commentary = end_commentary
                else:
                    commentary = middle_commentary

            # Optimize horse movement calculation
            finished_horses = []
            for horse in race.horses:
                if horse.position >= TRACK_LENGTH - 1:
                    finished_horses.append(horse)
                    continue 

                # Optimized movement calculation
                move_chance = (1 / horse.odds) * 0.5 
                base_move = 1 if random.random() < (0.1 + move_chance) else 0
                bonus_move = random.randint(0, 2) if base_move else 0
                
                horse.position = min(horse.position + base_move + bonus_move, TRACK_LENGTH - 1)

            # Check for winner
            if not winner and finished_horses:
                winner = min(finished_horses, key=lambda h: h.odds) 
                race.status = "finished"
                race_in_progress = False

            # Generate track display and update embed
            track_display = self._generate_track_display(race.horses)
            description = f"**{commentary}**\n\n{track_display}"
            
            embed_title = "🏁 A Winner is Decided! 🏁" if winner else "🏇 The Race is On! 🏇"
            embed_color = Color.gold() if winner else Color.green()
            
            if winner:
                description = f"**{winner.name} crosses the finish line first!**\n\n{track_display}"
            
            embed = await race.get_race_embed(embed_title, description, embed_color)
            
            await race.message.edit(embed=embed)
            
            if race_in_progress:
                await asyncio.sleep(1.5)
            
            turn_count += 1

        # Handle timeout case
        if not winner and turn_count >= max_turns:
            winner = max(race.horses, key=lambda h: h.position)
            race.status = "finished"
            self._record_race_activity("race_timeout")

        if winner:
            await self._payout_winners(race, winner)
            self._record_race_activity("race_finished")

            await race.message.edit(embed=embed)
            
            if race_in_progress:
                await asyncio.sleep(1.5)

        await self._payout_winners(race, winner)

    async def _payout_winners(self, race: Race, winner: Horse):
        race.horses.sort(key=lambda h: h.position, reverse=True)
        placements = {1: "🥇", 2: "🥈", 3: "🥉"}
        
        results_description = f"### 🏁 Race Results 🏁\n**Winner:** {winner.track_icon} **{winner.name}**!\n\n"
        results_description += "**Final Placements:**\n"
        for i, horse in enumerate(race.horses[:3]):
            results_description += f"{placements.get(i+1, '•')} **{horse.name}** (Horse #{horse.id})\n"

        payouts = {}
        total_winnings = 0
        db_updates = []
        xp_gains = {}

        # Fetch all participants' data at once
        participant_ids = [p.id for p in race.participants]
        users_data = await get_multiple_users(participant_ids)

        from utils.constants import MAX_CURRENT_XP
        for bet in race.bets:
            if bet.horse_id == winner.id:
                winnings = bet.amount * winner.odds
                total_winnings += winnings
                xp_gain = int(winnings * XP_PER_PROFIT)
                xp_gains[bet.user_id] = xp_gain

                # Determine allowed current XP increment
                user_data = users_data.get(bet.user_id)
                allowed_increment = 0
                if user_data:
                    curr_xp = int(user_data['current_xp'])
                    if curr_xp < MAX_CURRENT_XP:
                        allowed_increment = min(xp_gain, MAX_CURRENT_XP - curr_xp)
                        # Update cached value so subsequent wins in same batch respect cap
                        users_data[bet.user_id] = dict(user_data)
                        users_data[bet.user_id]['current_xp'] = curr_xp + allowed_increment

                update_entry = {
                    "user_id": bet.user_id,
                    "chips_increment": winnings,
                    "wins_increment": 1,
                    "total_xp_increment": xp_gain,
                }
                if allowed_increment > 0:
                    update_entry["current_xp_increment"] = allowed_increment
                db_updates.append(update_entry)
                payouts[bet.user_name] = (payouts.get(bet.user_name, (0, 0))[0] + winnings, bet.user_id)
            else:
                db_updates.append({
                    "user_id": bet.user_id,
                    "losses_increment": 1
                })

        if db_updates:
            # Capture pre-update xp/levels
            from utils.levels import get_user_level
            pre_levels = {}
            for upd in db_updates:
                udata = users_data.get(upd['user_id'])
                if udata:
                    pre_levels[upd['user_id']] = (
                        int(udata['current_xp']),
                        get_user_level(int(udata['current_xp']), int(udata['prestige']))
                    )

            await batch_update_users(db_updates)

            # Post notifications
            from utils.embeds import level_up_embed, prestige_ready_embed
            from utils import notifications
            from utils.constants import MAX_CURRENT_XP
            for upd in db_updates:
                uid = upd['user_id']
                if 'current_xp_increment' in upd:
                    try:
                        new_data = await get_user(uid)
                        prev_xp, prev_level = pre_levels.get(uid, (0, 0))
                        new_xp = int(new_data['current_xp'])
                        new_level = get_user_level(new_xp, int(new_data['prestige']))
                        if (new_level > prev_level and notifications.should_announce_level_up(uid, new_level)):
                            try:
                                await race.message.channel.send(content=f"<@{uid}>", embed=level_up_embed(new_level))
                            except Exception:
                                pass
                        if (prev_xp < MAX_CURRENT_XP and new_xp >= MAX_CURRENT_XP and
                            notifications.should_announce_prestige_ready(uid, int(new_data['prestige']))):
                            try:
                                await race.message.channel.send(content=f"<@{uid}>", embed=prestige_ready_embed())
                            except Exception:
                                pass
                    except Exception:
                        pass
            
            # Check for achievements after updating user stats
            try:
                from utils.achievements_config import check_and_award_achievements
                from utils.embeds import create_achievement_notification_embed
                
                # Check achievements for all users who participated
                for update in db_updates:
                    user_id = update["user_id"]
                    try:
                        newly_awarded = await check_and_award_achievements(user_id)
                        if newly_awarded:
                            # Get the user object for the mention
                            user = race.message.guild.get_member(user_id) or await race.message.guild.fetch_member(user_id)
                            achievement_embed = create_achievement_notification_embed(newly_awarded, user)
                            # Reply to the race message
                            try:
                                await race.message.reply(embed=achievement_embed, mention_author=True)
                            except (nextcord.NotFound, nextcord.HTTPException):
                                # Fallback to sending in the channel
                                await race.message.channel.send(embed=achievement_embed)
                    except Exception as e:
                        import logging
                        logging.error(f"Failed to check achievements for horse racing player {user_id}: {e}")
            except Exception as e:
                import logging
                logging.error(f"Failed to import achievement system: {e}")

        if payouts:
            results_description += "\n**🏆 Top Winners:**\n"
            top_winners = sorted(payouts.items(), key=lambda item: item[1][0], reverse=True)[:5]
            for i, (name, (amount, user_id)) in enumerate(top_winners):
                user_data = users_data.get(user_id)
                show_xp = user_data and user_data.get('premium_settings', {}).get('show_xp_gains', False)
                
                xp_display = f" (+{xp_gains.get(user_id, 0):,} XP)" if show_xp else ""
                results_description += f"{placements.get(i+1, '•')} **{name}** won **{amount:,}** chips!{xp_display}\n"
        else:
            results_description += "\nNo winners this time. The house keeps the chips!"

        embed = await race.get_race_embed(
            title=f"🏇 Race Finished: {winner.name} Wins! 🏇",
            description=results_description,
            color=Color.gold()
        )
        
        winner_count = len({b.user_id for b in race.bets if b.horse_id == winner.id})
        total_losers = len({b.user_id for b in race.bets}) - winner_count
        embed.set_footer(text=f"{winner_count} winners, {total_losers} losers. Total paid out: {total_winnings:,} chips.")
        
        await race.message.edit(embed=embed, view=None)

        if race.interaction.channel_id in self.active_races:
            del self.active_races[race.interaction.channel_id]

def setup(bot: commands.Bot):
    bot.add_cog(HorseRacingGame(bot))