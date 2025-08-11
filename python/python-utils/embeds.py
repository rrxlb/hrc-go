import nextcord
import asyncio
from typing import List, Dict, Any, TYPE_CHECKING, Optional
from .constants import RANKS, GUILD_ID, TIMEOUT_MESSAGE, GAME_TIMEOUT_MESSAGE, GAME_CLEANUP_MESSAGE, PREMIUM_ROLE_ID, MAX_CURRENT_XP
from .database import get_user
from .levels import get_user_level

if TYPE_CHECKING:
    from cogs.mines_game import MinesGameLogic

# Cache for user data to reduce DB calls
_user_cache: Dict[int, Dict[str, Any]] = {}

async def get_cached_user_data(user_id: int, force_refresh: bool = False) -> Dict[str, Any]:
    """Get user data with caching to reduce database calls."""
    if not force_refresh and user_id in _user_cache:
        return _user_cache[user_id]
    
    user_data = await get_user(user_id)
    _user_cache[user_id] = user_data
    
    # Limit cache size to prevent memory issues
    if len(_user_cache) > 1000:
        # Remove oldest entry
        oldest_key = next(iter(_user_cache))
        del _user_cache[oldest_key]
    
    return user_data

def clear_user_cache(user_id: int) -> None:
    """Clear cached user data after updates."""
    _user_cache.pop(user_id, None)

def _create_branded_embed(title: str, description: str = "", color: nextcord.Color = nextcord.Color.default(), timestamp=None) -> nextcord.Embed:
    """Creates a base embed with bot branding."""
    embed = nextcord.Embed(
        title=title,
        description=description,
        color=color,
        timestamp=timestamp or nextcord.utils.utcnow()
    )
    embed.set_footer(text="High Roller Club", icon_url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753043816/HRC-final_ymqwfy.png")
    return embed

def get_current_rank(xp: int) -> Dict[str, Any]:
    """Determines a user's rank based on their current XP."""
    current_rank = RANKS
    for rank_id, rank_info in RANKS.items():
        if xp >= rank_info["xp_required"]:
            current_rank = rank_info
        else:
            break
    return current_rank

def get_next_rank_xp(xp: int) -> int:
    """Gets the XP needed for the next rank."""
    for rank_id, rank_info in RANKS.items():
        if xp < rank_info["xp_required"]:
            return rank_info["xp_required"]
    return RANKS[max(RANKS.keys())]["xp_required"]

async def create_game_embed(
    player_hands: List[Dict[str, Any]], 
    dealer_hand: List[str], 
    dealer_value: int, 
    bet: int, 
    game_over: bool = False, 
    outcome_text: str = "", 
    new_balance: int = 0, 
    profit: int = 0, 
    xp_gain: int = 0, 
    user: Optional[nextcord.User] = None
) -> nextcord.Embed:
    """Creates the embed for the blackjack game using fields for clarity."""
    
    if game_over:
        if "win" in outcome_text.lower() or "pays" in outcome_text.lower():
            color = nextcord.Color.gold()
        elif "lost" in outcome_text.lower() or "bust" in outcome_text.lower() or "dealer wins" in outcome_text.lower():
            color = nextcord.Color.red()
        else: # Push
            color = nextcord.Color.light_grey()
    else:
        color = nextcord.Color.from_rgb(30, 86, 49) # Casino Green

    embed = _create_branded_embed(title="Blackjack", color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753042166/3_vxurig.png")
    
    # Player Hands
    for i, hand_data in enumerate(player_hands):
        hand_str = " ".join(hand_data["hand"])
        score = hand_data["score"]
        is_active = hand_data["is_active"]
        
        title = ""
        if len(player_hands) > 1:
            title = f"{'▶ ' if is_active else ''}Your Hand ({i+1}/{len(player_hands)}) - {score}"
        else:
            title = f"Your Hand - {score}"

        # Clarify Ace value
        if "A" in hand_data["hand"]:
            hand_str += " `(Aces can be 1 or 11)`"
            
        embed.add_field(name=title, value=f"`{hand_str}`", inline=False)

    # Dealer Hand
    dealer_hand_str = " ".join(dealer_hand)
    embed.add_field(name=f"Dealer's Hand - {dealer_value}", value=f"`{dealer_hand_str}`", inline=False)

    # Preserve the original footer icon and text
    original_footer_text = embed.footer.text or "High Roller Club"
    original_footer_icon = embed.footer.icon_url

    if game_over:
        embed.add_field(name="Outcome", value=outcome_text, inline=False)
        if profit > 0:
            winnings_text = f"{int(profit):,} <:chips:1396988413151940629>"
            if "blackjack" in outcome_text.lower():
                winnings_text += " `(1.5x)`"
            elif "5-card charlie" in outcome_text.lower():
                winnings_text += " `(1.75x)`"
            embed.add_field(name="Winnings", value=winnings_text, inline=True)
            
            # Use cached user data and show level up notification
            if user and xp_gain > 0:
                try:
                    user_data = await get_cached_user_data(user.id)
                    settings = user_data.get('premium_settings') or {}
                    has_premium_role = any(role.id == 1396631093154943026 for role in user.roles) if hasattr(user, 'roles') else False
                    show_xp = settings.get('xp_display', False)
                    
                    if has_premium_role and show_xp:
                        embed.add_field(name="XP Gained", value=f"{xp_gain:,} XP", inline=True)
                        
                        # (Level-up notification now sent as separate message; inline field removed)
                except Exception:
                    # Fallback if caching fails
                    pass
        elif profit < 0:
            embed.add_field(name="Losses", value=f"{-int(profit):,} <:chips:1396988413151940629>", inline=True)
        
        embed.add_field(name="New Balance", value=f"{new_balance:,} <:chips:1396988413151940629>", inline=False)
        embed.set_footer(text=f"{original_footer_text} | Game Over", icon_url=original_footer_icon)
    else:
        embed.set_footer(text=f"{original_footer_text} | Bet: {bet:,} chips", icon_url=original_footer_icon)

    return embed

def create_profile_embed(interaction: nextcord.Interaction, user: nextcord.User, user_data: Dict[str, Any], author_data: Dict[str, Any]) -> nextcord.Embed:
    """Creates the user profile embed."""
    rank = get_current_rank(user_data['current_xp'])
    next_rank_xp = get_next_rank_xp(user_data['current_xp'])
    
    prestige_emojis = {
        1: "<:p1:1401690446052458618>",
        2: "<:p2:1401690453643886762>",
        3: "<:p3:1401690460992442388>",
        4: "<:p4:1401690470194872320>",
        5: "<:p5:1401690479837577326>"
    }
    premium_emoji = "<a:premium:1401694358532784178>"

    status_emojis = ""
    prestige_level = user_data.get('prestige', 0)
    if prestige_level > 0:
        status_emojis += prestige_emojis.get(prestige_level, "")

    has_premium_role = any(role.id == PREMIUM_ROLE_ID for role in user.roles)
    if has_premium_role:
        status_emojis += f" {premium_emoji}"

    embed = _create_branded_embed(title="", color=rank['color'])
    embed.set_author(name=user.display_name)
    embed.set_thumbnail(url=user.display_avatar.url)
    embed.description = status_emojis
    
    embed.add_field(name="Rank", value=f"{rank['icon']} {rank['name']}", inline=True)
    embed.add_field(name="Chips", value=f"{int(user_data['chips']):,} <:chips:1396988413151940629>", inline=True)
    embed.add_field(name="\u200b", value="\u200b", inline=True) # Spacer
    
    prestige_ready_suffix = " ✅" if int(user_data['current_xp']) >= MAX_CURRENT_XP else ""
    embed.add_field(name="Current XP", value=f"{int(user_data['current_xp']):,}{prestige_ready_suffix}", inline=True)
    embed.add_field(name="Total XP", value=f"{int(user_data['total_xp']):,}", inline=True)
    embed.add_field(name="\u200b", value="\u200b", inline=True) # Spacer
    
    # Progress bar for XP
    progress = user_data['current_xp'] / next_rank_xp
    progress_bar = "🟩" * int(progress * 10) + "⬜" * (10 - int(progress * 10))
    embed.add_field(name="XP to Next Rank", value=f"`{progress_bar}`\n{int(user_data['current_xp']):,} / {next_rank_xp:,}", inline=False)
    
    # Determine if wins and losses should be displayed
    target_settings = user_data.get('premium_settings', {}) or {}
    show_stats = target_settings.get('wins_losses_display', False)

    other_info = ""
    # Only show stats if the user is premium and has the setting enabled.
    if has_premium_role and show_stats:
        other_info += f"**Wins/Losses:**\n({user_data['wins']}-{user_data['losses']})"

    if other_info:
        embed.add_field(name="Other", value=other_info, inline=False)

    if interaction.guild.id != GUILD_ID:
        embed.add_field(name=" ", value="[Join for /bonus!](https://discord.gg/RK4K8tDsHB)", inline=False)
    return embed


async def create_profile_embed_with_achievements(interaction: nextcord.Interaction, user: nextcord.User, user_data: Dict[str, Any], author_data: Dict[str, Any]) -> nextcord.Embed:
    """Creates the user profile embed with achievements."""
    embed = create_profile_embed(interaction, user, user_data, author_data)
    
    # Add recent achievements
    try:
        from .database import get_user_achievements
        user_achievements = await get_user_achievements(user.id)
        
        if user_achievements:
            # Show up to 3 most recent achievements
            recent_achievements = sorted(user_achievements, key=lambda x: x['earned_at'], reverse=True)[:3]
            achievement_text = ""
            
            for achievement in recent_achievements:
                timestamp = int(achievement['earned_at'].timestamp())
                achievement_text += f"{achievement['icon']} **{achievement['name']}** (<t:{timestamp}:R>)\n"
            
            if achievement_text:
                # Add a spacer to ensure a visual line break before achievements
                embed.add_field(name="\u200b", value="\u200b", inline=False)
                embed.add_field(
                    name="🏆 Recent Achievements",
                    value=achievement_text.strip(),
                    inline=False
                )
    except Exception:
        # If there's an error fetching achievements, silently skip this section
        pass
    
    return embed


def create_achievement_notification_embed(achievements: List[Dict[str, Any]], user: nextcord.User = None) -> nextcord.Embed:
    """Creates an embed for newly earned achievements."""
    title = "🎉 Achievement Unlocked!"
    if user:
        title = f"🎉 Achievement Unlocked for {user.mention}!"
    
    embed = _create_branded_embed(
        title=title,
        color=nextcord.Color.gold()
    )
    
    total_chips = 0
    total_xp = 0
    
    for achievement in achievements:
        reward_text = []
        if achievement["chips_reward"] > 0:
            reward_text.append(f"{achievement['chips_reward']:,} chips")
            total_chips += achievement["chips_reward"]
        if achievement["xp_reward"] > 0:
            reward_text.append(f"{achievement['xp_reward']:,} XP")
            total_xp += achievement["xp_reward"]
        
        reward_str = f" • Reward: {', '.join(reward_text)}" if reward_text else ""
        
        embed.add_field(
            name=f"{achievement['icon']} {achievement['name']}",
            value=f"{achievement['description']}{reward_str}",
            inline=False
        )
    
    if total_chips > 0 or total_xp > 0:
        reward_summary = []
        if total_chips > 0:
            reward_summary.append(f"💰 {total_chips:,} chips")
        if total_xp > 0:
            reward_summary.append(f"⭐ {total_xp:,} XP")
        
        embed.add_field(
            name="🎁 Total Rewards",
            value=" • ".join(reward_summary),
            inline=False
        )
    
    return embed

async def create_leaderboard_embed(interaction: nextcord.Interaction, title: str, leaderboard_data: List[Dict[str, Any]], key: str) -> nextcord.Embed:
    """Creates the leaderboard embed."""
    embed = _create_branded_embed(title=f"🏆 Leaderboard: {title}", color=nextcord.Color.gold())
    
    description = ""
    for i, user_row in enumerate(leaderboard_data):
        user = await interaction.client.fetch_user(user_row['user_id'])
        value = user_row[key]
        
        # Add chip emoji for the 'chips' leaderboard
        display_value = f"{value:,}"
        if key == 'chips':
            display_value += " <:chips:1396988413151940629>"

        if user.discriminator == "0":
            description += f"**{i+1}.** {user.name} - {display_value}\n"
        else:
            description += f"**{i+1}.** {user.name}#{user.discriminator} - {display_value}\n"
        
    if not description:
        description = "The leaderboard is empty!"
    
    if interaction.guild.id != GUILD_ID:
        description += "\n\n[Join for /bonus!](https://discord.gg/RK4K8tDsHB)"
        
    embed.description = description
    return embed

def error_embed(message: str) -> nextcord.Embed:
    """Creates a standardized error embed."""
    embed = _create_branded_embed(title="Error", description=message, color=nextcord.Color.red())
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753046175/ER2_fwidxb.png")
    return embed

def insufficient_chips_embed(required_chips: int, current_balance: int, bet_description: str = "that bet") -> nextcord.Embed:
    """Creates an enhanced embed for insufficient chips with helpful information."""
    embed = _create_branded_embed(
        title="Not Enough Chips", 
        description=f"You don't have enough chips for {bet_description}.\n**Your balance:** {current_balance:,} <:chips:1396988413151940629>\n**Required:** {required_chips:,} <:chips:1396988413151940629>",
        color=nextcord.Color.red()
    )
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753046175/ER2_fwidxb.png")
    
    # Add helpful information on how to get more chips
    help_text = (
        "💰 **Get more chips:**\n"
        "• Use `/claimall` to claim daily, hourly, and weekly bonuses\n"
        "• Use `/vote` to vote for the bot on Top.gg for extra chips\n"
        "• Play lower stakes games to build your balance"
    )
    embed.add_field(name="How to Get More Chips", value=help_text, inline=False)
    
    return embed

def bonus_embed(amount: int, new_balance: int) -> nextcord.Embed:
    """Creates the embed for the daily bonus."""
    embed = _create_branded_embed(
        title="Bonus Claimed!",
        description=f"You have received **{amount:,}** <:chips:1396988413151940629>.",
        color=nextcord.Color.green()
    )
    embed.add_field(name="New Balance", value=f"{new_balance:,} <:chips:1396988413151940629>")
    embed.set_footer(text="Come back in 24 hours for your next bonus!")
    return embed

def rank_up_embed(new_rank_name: str, new_rank_icon: str) -> nextcord.Embed:
    """Creates the embed for a rank up."""
    embed = _create_branded_embed(
        title="🎲 Movin' On Up! 🎲",
        description="You're one step closer to becoming a High Roller!",
        color=nextcord.Color.gold()
    )
    embed.add_field(name="New Rank", value=f"**{new_rank_icon} {new_rank_name}**", inline=False)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753045654/RU2_yrvdas.png")
    return embed

def level_up_embed(new_level: int) -> nextcord.Embed:
    """Embed announcing a level up (separate from rank)."""
    embed = _create_branded_embed(
        title="🎉 Level Up!",
        description=f"You've reached **Level {new_level}**! Keep playing to climb further.",
        color=nextcord.Color.green()
    )
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753057051/SC_vbarbh.png")
    return embed

def prestige_ready_embed() -> nextcord.Embed:
    """Embed indicating the user can prestige now."""
    embed = _create_branded_embed(
        title="💠 Prestige Ready",
        description="You've reached the maximum XP for this prestige. Use `/prestige` to ascend and start anew!",
        color=nextcord.Color.purple()
    )
    return embed

def success_embed(title: str, description: str) -> nextcord.Embed:
    """Creates a standardized success embed."""
    embed = _create_branded_embed(
        title=title,
        description=description,
        color=nextcord.Color.green()
    )
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753057051/SC_vbarbh.png")
    return embed

async def create_slots_embed(state: str, reels: List[List[str]] = None, outcome_text: str = "", xp_gain: int = 0, new_balance: int = 0, bet: int = 0, user: nextcord.User = None, jackpot_amount: Optional[int] = None) -> nextcord.Embed:
    """Creates the embed for the slots game, handling different states."""
    if state == 'initial':
        title = "Slot Machine"
        description = "Spinning the reels..."
        color = nextcord.Color.blue()
    elif state == 'spinning':
        title = "Slot Machine"
        description = f"Spinning the reels...\n\n{reels}"
        color = nextcord.Color.blue()
    elif state == 'final':
        title = "Slot Machine Results"
        description = reels
        if "JACKPOT" in outcome_text:
            color = nextcord.Color.gold()
        elif "Congratulations" in outcome_text:
            color = nextcord.Color.green()
        else:
            color = nextcord.Color.red()
    else:
        # Default case, though should not be reached
        title = "Slot Machine"
        description = ""
        color = nextcord.Color.default()

    embed = _create_branded_embed(title=title, description=description, color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753050867/SL_d8ophs.png")

    if state == 'final':
        # Add a spacer field to separate reels from summary nicely
        embed.add_field(name="\u200b", value="\u200b", inline=False)
        embed.add_field(name="Outcome", value=outcome_text, inline=False)
        if jackpot_amount is not None:
            embed.add_field(name="Jackpot", value=f"{int(jackpot_amount):,} <:chips:1396988413151940629>", inline=True)
        if user and xp_gain > 0:
            user_data = await get_user(user.id)
            if user_data:
                settings = user_data['premium_settings'] or {}
                has_premium_role = any(role.id == 1396631093154943026 for role in user.roles)
                show_xp = settings.get('xp_display', False)
                if has_premium_role and show_xp:
                    embed.add_field(name="XP Gained", value=f"+{xp_gain:,} XP", inline=True)
        embed.add_field(name="New Balance", value=f"{new_balance:,} <:chips:1396988413151940629>", inline=False)
        embed.set_footer(text=f"You bet {bet:,} chips.")

    return embed

async def create_roulette_embed(state: str, bets: Dict[str, int] = None, result: Dict[str, Any] = None, new_balance: int = 0, profit: int = 0, xp_gain: int = 0, user: nextcord.User = None) -> nextcord.Embed:
    """Creates a visually appealing embed for the roulette game."""
    
    # --- Emojis and Colors ---
    color_map = {
        "red": {"emoji": "🔴", "color": nextcord.Color.red()},
        "black": {"emoji": "⚫", "color": nextcord.Color.dark_grey()},
        "green": {"emoji": "🟢", "color": nextcord.Color.green()}
    }

    if state == 'betting':
        title = "Roulette Table"
        description = "Place your bets using the buttons below."
        color = nextcord.Color.from_rgb(30, 86, 49) # Casino Green
    elif state == 'spinning':
        title = "🎡 Wheel is Spinning..."
        description = "No more bets! Good luck!"
        color = nextcord.Color.purple()
    elif state == 'final':
        win_color_info = color_map[result['color']]
        title = f"{win_color_info['emoji']} The Wheel Landed on {result['number']} {result['color'].capitalize()}! {win_color_info['emoji']}"
        description = "Let's see how you did."
        color = win_color_info['color']
    else:
        title = "Roulette"
        description = ""
        color = nextcord.Color.default()

    embed = _create_branded_embed(title=title, description=description, color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753327527/RL2_k0v99x.png")

    # --- Bet Information ---
    if bets:
        bet_str = "\n".join([f"{bet_type.capitalize()}: **{amount:,}** <:chips:1396988413151940629>" for bet_type, amount in bets.items()])
        embed.add_field(name="Your Bets", value=bet_str, inline=False)

    # --- Final State Details ---
    if state == 'final':
        win_color_info = color_map[result['color']]
        embed.add_field(
            name="Winning Number", 
            value=f"`{result['number']} {result['color'].capitalize()}` {win_color_info['emoji']}", 
            inline=False
        )

        outcome_str = ""
        if profit > 0:
            outcome_str = f"You won **{int(profit):,}** <:chips:1396988413151940629>!"
            if user and xp_gain > 0:
                user_data = await get_user(user.id)
                if user_data:
                    settings = user_data['premium_settings'] or {}
                    has_premium_role = any(role.id == 1396631093154943026 for role in user.roles)
                    show_xp = settings.get('xp_display', False)
                    if has_premium_role and show_xp:
                        embed.add_field(name="XP Gained", value=f"+{xp_gain:,} XP", inline=True)
        elif profit < 0:
            outcome_str = f"You lost **{-int(profit):,}** <:chips:1396988413151940629>."
        else:
            outcome_str = "You broke even."
        
        embed.add_field(name="Outcome", value=outcome_str, inline=True)
        embed.add_field(name="New Balance", value=f"**{new_balance:,}** <:chips:1396988413151940629>", inline=False)
    embed.set_footer(text="High Roller Club | Game Over")

    return embed

async def create_tcp_embed(
    user: nextcord.User,
    player_hand: List[str],
    dealer_hand: List[str],
    player_eval: str,
    dealer_eval: str,
    bets: Dict[str, int],
    game_over: bool = False,
    outcome_summary: str = "",
    payout_results: List[str] = None,
    final_balance: int = 0,
    profit: int = 0,
    xp_gain: int = 0,
) -> nextcord.Embed:
    """Creates a branded embed for the Three Card Poker game."""
    
    # --- Color Logic ---
    if game_over:
        if profit > 0:
            color = nextcord.Color.green()
        elif profit < 0:
            color = nextcord.Color.red()
        else: # Push
            color = nextcord.Color.light_grey()
    else:
        color = nextcord.Color.from_rgb(30, 86, 49) # Casino Green

    embed = _create_branded_embed(title="Three Card Poker", color=color)
    embed.set_author(name=user.name, icon_url=user.display_avatar.url)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1754083114/TC2_ugnpqd.png") # Placeholder, can be changed

    # --- Hand Display ---
    player_hand_str = " ".join(player_hand)
    embed.add_field(name="Your Hand", value=f"`{player_hand_str}`\n> {player_eval}", inline=True)

    if game_over:
        dealer_hand_str = " ".join(dealer_hand)
        embed.add_field(name="Dealer's Hand", value=f"`{dealer_hand_str}`\n> {dealer_eval}", inline=True)
    else:
        embed.add_field(name="Dealer's Hand", value="`?? ?? ??`", inline=True)

    # --- Game State Display ---
    if game_over:
        embed.add_field(name="\u200b", value="\u200b", inline=True) # Spacer
        embed.add_field(name="Outcome", value=outcome_summary, inline=False)
        if payout_results:
            embed.add_field(name="Payouts", value="\n".join(payout_results), inline=False)
        
        # Financial Info
        embed.add_field(name="Profit", value=f"{profit:+,} <:chips:1396988413151940629>", inline=True)
        
        # Premium XP Display
        user_data = await get_user(user.id)
        if user_data:
            # Ensure premium_settings is always a dict (DB may store NULL)
            settings = user_data.get('premium_settings') or {}
            has_premium_role = any(role.id == 1396631093154943026 for role in user.roles)
            show_xp = settings.get('xp_display', False)
            if has_premium_role and show_xp and xp_gain > 0:
                embed.add_field(name="XP Gained", value=f"{xp_gain:,} XP", inline=True)

        embed.add_field(name="New Balance", value=f"{final_balance:,} <:chips:1396988413151940629>", inline=False)
        embed.set_footer(text="High Roller Club | Game Over")
    else:
        bet_info = f"**Ante:** {bets.get('ante', 0):,}"
        if bets.get('pair_plus', 0) > 0:
            bet_info += f"\n**Pair Plus:** {bets.get('pair_plus', 0):,}"
        embed.add_field(name="\u200b", value="\u200b", inline=True) # Spacer
        embed.add_field(name="Bets", value=bet_info, inline=False)
        embed.set_footer(text="High Roller Club | Do you want to Play or Fold?")

    return embed

async def create_baccarat_embed(game_over: bool, bet: int, player_hand: List[str] = None, banker_hand: List[str] = None, player_score: int = 0, banker_score: int = 0, outcome_text: str = "", new_balance: int = 0, profit: int = 0, xp_gain: int = 0, choice: str = "", user: nextcord.User = None) -> nextcord.Embed:
    """Creates the embed for the Baccarat game."""
    if game_over:
        if "win" in outcome_text.lower():
            color = nextcord.Color.gold()
        elif "lost" in outcome_text.lower() or "tie" in outcome_text.lower():
            color = nextcord.Color.light_grey()
        else:
            color = nextcord.Color.red()
    else:
        color = nextcord.Color.from_rgb(138, 43, 226) # Blue Violet for Baccarat

    embed = _create_branded_embed(title="Baccarat", color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753058981/baccarat_icon_gq8v5g.png") # Placeholder icon

    if not game_over:
        embed.description = f"You are betting {bet:,} <:chips:1396988413151940629>.\nChoose your side."
        embed.set_footer(text="High Roller Club | Place your bet")
    else:
        player_hand_str = " ".join(player_hand)
        banker_hand_str = " ".join(banker_hand)
        embed.add_field(name=f"Player's Hand - {player_score}", value=f"`{player_hand_str}`", inline=False)
        embed.add_field(name=f"Banker's Hand - {banker_score}", value=f"`{banker_hand_str}`", inline=False)
        
        outcome_summary = f"You bet on **{choice.capitalize()}**.\n{outcome_text}"
        embed.add_field(name="Outcome", value=outcome_summary, inline=False)

        if profit > 0:
            winnings_text = f"{int(profit):,} <:chips:1396988413151940629>"
            if choice == 'banker':
                winnings_text += " `(0.95x)`"
            elif choice == 'tie':
                winnings_text += " `(8x)`"
            embed.add_field(name="Winnings", value=winnings_text, inline=True)
            if user and xp_gain > 0:
                user_data = await get_user(user.id)
                if user_data:
                    settings = user_data['premium_settings'] or {}
                    has_premium_role = any(role.id == 1396631093154943026 for role in user.roles)
                    show_xp = settings.get('xp_display', False)
                    if has_premium_role and show_xp:
                        embed.add_field(name="XP Gained", value=f"{xp_gain:,} XP", inline=True)
        elif profit < 0:
            embed.add_field(name="Losses", value=f"{-int(profit):,} <:chips:1396988413151940629>", inline=True)
        
        embed.add_field(name="New Balance", value=f"{new_balance:,} <:chips:1396988413151940629>", inline=False)
        embed.set_footer(text="High Roller Club | Game Over")

    return embed


async def create_craps_embed(
    user: nextcord.User,
    point: int,
    bet_summary: str,
    layout: str,
    roll_display: str,
    outcome_text: str = "",
    game_over: bool = False,
    xp_gain: int = 0,
) -> nextcord.Embed:
    """Creates the embed for the Craps game."""
    color = nextcord.Color.green()
    if game_over:
        if "win" in outcome_text.lower() or "hits" in outcome_text.lower():
            color = nextcord.Color.gold()
        elif "lose" in outcome_text.lower() or "seven out" in outcome_text.lower():
            color = nextcord.Color.red()

    embed = _create_branded_embed(title="🎲 Craps Table", color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753056371/CR2_ylrlyz.png")
    
    embed.add_field(name="Shooter", value=user.mention, inline=False)
    
    point_status = f"**ON** ({point})" if point else "**OFF**"
    embed.add_field(name="Point", value=point_status, inline=True)

    embed.add_field(name="Current Roll", value=roll_display, inline=True)
    embed.add_field(name="Your Bets", value=bet_summary, inline=False)
    embed.add_field(name="Layout", value=layout, inline=False)
    
    if outcome_text:
        embed.add_field(name="Outcome", value=outcome_text, inline=False)

    if user and xp_gain > 0:
        user_data = await get_user(user.id)
        if user_data:
            settings = user_data.get('premium_settings') or {}
            has_premium_role = any(role.id == 1396631093154943026 for role in user.roles)
            show_xp = settings.get('xp_display', False)
            if has_premium_role and show_xp:
                embed.add_field(name="XP Gained", value=f"{xp_gain:,} XP", inline=True)

    footer_text = "Use the buttons below to play."
    if game_over:
        footer_text = "This game has ended. Start a new one with /craps."
    embed.set_footer(text=footer_text)
    
    return embed


def create_timeout_embed(message: str = None) -> nextcord.Embed:
    """Creates a standardized timeout embed."""
    description = message if message else TIMEOUT_MESSAGE
    embed = _create_branded_embed(
        title="Timeout",
        description=description,
        color=nextcord.Color.orange()
    )
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753046175/ER2_fwidxb.png")
    return embed

def create_game_timeout_embed(bet_amount: int) -> nextcord.Embed:
    """Creates a standardized game timeout embed with chip forfeiture."""
    description = GAME_TIMEOUT_MESSAGE.format(bet_amount=bet_amount)
    embed = _create_branded_embed(
        title="Game Timeout",
        description=description,
        color=nextcord.Color.orange()
    )
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753046175/ER2_fwidxb.png")
    return embed

def create_game_cleanup_embed(bet_amount: int) -> nextcord.Embed:
    """Creates a standardized game cleanup embed with chip forfeiture."""
    description = GAME_CLEANUP_MESSAGE.format(bet_amount=bet_amount)
    embed = _create_branded_embed(
        title="Game Forfeited",
        description=description,
        color=nextcord.Color.red()
    )
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753046175/ER2_fwidxb.png")
    return embed

async def create_higher_or_lower_embed(
    state: str,
    current_card: Any,
    bet: int,
    streak: int = 0,
    winnings: int = 0,
    next_card: Any = None,
    outcome_text: str = "",
    new_balance: int = 0,
    xp_gain: int = 0,
    user: nextcord.User = None,
    multiplier: float = 0.0
) -> nextcord.Embed:
    """Creates a visually appealing embed for the Higher or Lower game."""
    
    # --- Base Setup ---
    title = "Higher or Lower"
    description = ""
    color = nextcord.Color.blue()

    if state == 'playing':
        color = nextcord.Color.from_rgb(30, 86, 49) # Casino Green
        description = "Will the next card be higher or lower?"
    elif state == 'result':
        title = "Higher or Lower - Result"
        if "win" in outcome_text.lower():
            color = nextcord.Color.green()
        elif "lost" in outcome_text.lower():
            color = nextcord.Color.red()
        else: # Push
            color = nextcord.Color.light_grey()
    elif state == 'final':
        title = "Higher or Lower - Game Over"
        if "Cashed out" in outcome_text:
            color = nextcord.Color.gold()
        else:
            color = nextcord.Color.red()

    embed = _create_branded_embed(title=title, description=description, color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753327146/HL2_oproic.png")

    # --- Card Display ---
    if state == 'playing':
        embed.add_field(name="Current Card", value=f"`{current_card}`", inline=False)
    elif state == 'result':
        embed.add_field(name="Previous Card", value=f"`{current_card}`", inline=True)
        embed.add_field(name="Next Card", value=f"`{next_card}`", inline=True)
        embed.add_field(name="\u200b", value="\u200b", inline=True) # Spacer
    elif state == 'final':
        final_card = next_card or current_card
        embed.add_field(name="Final Card", value=f"`{final_card}`", inline=False)

    # --- Game Stats ---
    game_info = (
        f"**Streak:** 🔥 {streak} wins\n"
        f"**Multiplier:** `x{multiplier}`\n"
        f"**Current Winnings:** {int(winnings):,} <:chips:1396988413151940629>"
    )
    embed.add_field(name="Game Info", value=game_info, inline=False)
    embed.add_field(name="Initial Bet", value=f"{bet:,} <:chips:1396988413151940629>", inline=False)

    # --- Final Outcome ---
    if state in ['result', 'final']:
        embed.add_field(name="Outcome", value=outcome_text, inline=False)
        if winnings > 0:
            embed.add_field(name="💰 Total Winnings", value=f"{winnings:,} <:chips:1396988413151940629>", inline=True)
            if user and xp_gain > 0:
                user_data = await get_user(user.id)
                if user_data:
                    settings = user_data['premium_settings'] or {}
                    has_premium_role = any(role.id == 1396631093154943026 for role in user.roles)
                    show_xp = settings.get('xp_display', False)
                    if has_premium_role and show_xp:
                        embed.add_field(name="✨ XP Gained", value=f"{xp_gain:,} XP", inline=True)
        embed.add_field(name="New Balance", value=f"{new_balance:,} <:chips:1396988413151940629>", inline=False)
        embed.set_footer(text="High Roller Club | Game Over")
    else:
        embed.set_footer(text="High Roller Club | Choose wisely!")
        
    return embed

async def create_mines_embed(
    game: "MinesGameLogic",
    state: str,
    outcome_text: str = "",
    profit: int = 0,
    xp_gain: int = 0,
    new_balance: int = 0,
    hit_mine_coords: tuple = None,
    user: nextcord.User = None
) -> nextcord.Embed:
    """Creates a visually appealing embed for the Mines game."""
    
    # --- Base Setup ---
    if state == 'playing':
        title = "💣 Mines"
        description = "Click the tiles to find the gems. Avoid the mines!"
        color = nextcord.Color.from_rgb(79, 84, 92) # Discord Blurple-ish
    elif state == 'final':
        title = "💣 Mines - Game Over"
        description = f"**{outcome_text}**"
        # Color based on profit
        if profit > 0:
            color = nextcord.Color.gold()
        elif profit < 0:
            color = nextcord.Color.red()
        else: # Cashed out with no profit or timed out
            color = nextcord.Color.light_grey()
    else: # Should not happen
        title = "💣 Mines"
        description = ""
        color = nextcord.Color.default()

    embed = _create_branded_embed(title=title, description=description, color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753328701/MN_vouwpt.jpg")

    # --- Game Info ---
    embed.add_field(name="Initial Bet", value=f"{game.bet:,} <:chips:1396988413151940629>", inline=True)
    embed.add_field(name="Mines", value=f"{game.mine_count} 💣", inline=True)
    
    if state == 'playing':
        embed.add_field(name="Multiplier", value=f"x{game.current_multiplier}", inline=True)
        embed.add_field(name="Potential Winnings", value=f"{game.current_winnings:,} <:chips:1396988413151940629>", inline=False)
        embed.set_footer(text="High Roller Club | Uncover the gems!")
    
    # --- Final State ---
    if state == 'final':
        # Generate the final grid display
        grid_str = ""
        for r, row_of_tiles in enumerate(game.grid):
            for c, tile in enumerate(row_of_tiles):
                if (r, c) == hit_mine_coords:
                    grid_str += "💥"
                elif tile.is_mine:
                    grid_str += "💣"
                elif not tile.is_mine and tile.is_revealed:
                    grid_str += "💎"
                else: # Not a mine, not revealed
                    grid_str += "⬛"
            grid_str += "\n"
        
        embed.add_field(name="Final Board", value=f"```\n{grid_str}```", inline=False)

        # Financial Outcome
        if profit > 0:
            embed.add_field(name="Profit", value=f"+{int(profit):,} <:chips:1396988413151940629>", inline=True)
            if user and xp_gain > 0:
                user_data = await get_user(user.id)
                if user_data:
                    settings = user_data['premium_settings'] or {}
                    has_premium_role = any(role.id == 1396631093154943026 for role in user.roles)
                    show_xp = settings.get('xp_display', False)
                    if has_premium_role and show_xp:
                        embed.add_field(name="XP Gained", value=f"+{xp_gain:,} XP", inline=True)
        elif profit < 0:
            embed.add_field(name="Loss", value=f"{-int(profit):,} <:chips:1396988413151940629>", inline=True)
        else: # Broke even
             embed.add_field(name="Profit", value="0 <:chips:1396988413151940629>", inline=True)
        
        embed.add_field(name="\u200b", value="\u200b", inline=True) # Spacer
        embed.add_field(name="New Balance", value=f"**{new_balance:,}** <:chips:1396988413151940629>", inline=False)
        embed.set_footer(text="High Roller Club | Game Over")

    return embed

async def create_crash_embed(
    state: str,
    multiplier: float = 1.00,
    players_count: int = 0,
    round_number: int = 0,
    time_remaining: int = 0,
    outcome_text: str = "",
    user_bet: int = 0,
    user_cashout: float = 0,
    user_profit: int = 0,
    user_balance: int = 0,
    xp_gain: int = 0,
    crash_point: float = None,
    user: nextcord.User = None,
    history: List[float] = None
) -> nextcord.Embed:
    """Creates embed for the Crash game in different states."""
    
    # Determine title, description and color based on state
    if state == 'betting':
        title = "🚀 Crash - Betting Phase"
        description = f"Place your bets! Game starts in **{time_remaining}s**"
        color = nextcord.Color.blurple()
    elif state == 'active':
        title = "🚀 Crash - Active"
        description = f"Current multiplier: **{multiplier:.2f}x**"
        color = nextcord.Color.green()
    elif state == 'crashed':
        title = "💥 Crash - Round Over"
        description = f"**Crashed at {crash_point:.2f}x!**"
        color = nextcord.Color.red()
    elif state == 'user_result':
        title = "🚀 Crash - Your Result"
        description = outcome_text
        if user_profit > 0:
            color = nextcord.Color.gold()
        elif user_profit < 0:
            color = nextcord.Color.red()
        else:
            color = nextcord.Color.light_grey()
    else:
        title = "🚀 Crash"
        description = ""
        color = nextcord.Color.default()

    embed = _create_branded_embed(title=title, description=description, color=color)
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753328701/crash_game.png")

    # Add fields based on state
    if state == 'betting':
        embed.add_field(name="Players Joined", value=f"{players_count} players", inline=True)
        embed.add_field(name="Round", value=f"#{round_number}", inline=True)
        embed.add_field(name="\u200b", value="\u200b", inline=True)
        
        if history:
            history_str = " • ".join([f"{h:.2f}x" for h in history[-5:]])
            embed.add_field(name="Recent Crashes", value=f"`{history_str}`", inline=False)
        
        embed.set_footer(text="High Roller Club | Place your bet and optional auto-cashout!")
    
    elif state == 'active':
        embed.add_field(name="Players Active", value=f"{players_count} players", inline=True)
        embed.add_field(name="Round", value=f"#{round_number}", inline=True)
        embed.add_field(name="Growth", value="📈", inline=True)
        
        # Show user's bet if they're playing
        if user_bet > 0:
            potential_win = int(user_bet * multiplier)
            embed.add_field(name="Your Bet", value=f"{user_bet:,} <:chips:1396988413151940629>", inline=True)
            embed.add_field(name="Potential Win", value=f"{potential_win:,} <:chips:1396988413151940629>", inline=True)
            embed.add_field(name="\u200b", value="\u200b", inline=True)
        
        embed.set_footer(text="High Roller Club | Cash out before it crashes!")
    
    elif state == 'crashed':
        embed.add_field(name="Final Multiplier", value=f"{crash_point:.2f}x", inline=True)
        embed.add_field(name="Round", value=f"#{round_number}", inline=True)
        embed.add_field(name="Next Round", value=f"In {time_remaining}s", inline=True)
        
        if history:
            history_str = " • ".join([f"{h:.2f}x" for h in history[-5:]])
            embed.add_field(name="Recent Crashes", value=f"`{history_str}`", inline=False)
        
        embed.set_footer(text="High Roller Club | Round ended")
    
    elif state == 'user_result':
        embed.add_field(name="Your Bet", value=f"{user_bet:,} <:chips:1396988413151940629>", inline=True)
        if user_cashout > 0:
            embed.add_field(name="Cashed Out At", value=f"{user_cashout:.2f}x", inline=True)
        else:
            embed.add_field(name="Result", value="Didn't cash out", inline=True)
        embed.add_field(name="\u200b", value="\u200b", inline=True)
        
        if user_profit > 0:
            embed.add_field(name="Profit", value=f"+{user_profit:,} <:chips:1396988413151940629>", inline=True)
        elif user_profit < 0:
            embed.add_field(name="Loss", value=f"{user_profit:,} <:chips:1396988413151940629>", inline=True)
        
        embed.add_field(name="New Balance", value=f"{user_balance:,} <:chips:1396988413151940629>", inline=True)
        
        if xp_gain > 0:
            embed.add_field(name="XP Gained", value=f"+{xp_gain:,} XP", inline=True)
        
        embed.set_footer(text="High Roller Club | Game Over")

    return embed

def create_crash_history_embed(history: List[Dict[str, any]], page: int = 1, total_pages: int = 1) -> nextcord.Embed:
    """Creates embed showing crash game history."""
    embed = _create_branded_embed(
        title="🚀 Crash History",
        description=f"Recent crash game results (Page {page}/{total_pages})",
        color=nextcord.Color.blurple()
    )
    
    if not history:
        embed.add_field(name="No History", value="No recent games found.", inline=False)
        return embed
    
    history_text = ""
    for i, round_data in enumerate(history, 1):
        round_num = round_data.get('round_number', 'N/A')
        crash_point = round_data.get('crash_point', 0)
        players = round_data.get('players_count', 0)
        timestamp = round_data.get('created_at', 'N/A')
        
        if hasattr(timestamp, 'strftime'):
            time_str = timestamp.strftime('%H:%M:%S')
        else:
            time_str = str(timestamp)
        
        history_text += f"**#{round_num}** - {crash_point:.2f}x ({players} players) `{time_str}`\n"
    
    embed.add_field(name="Recent Rounds", value=history_text or "No data", inline=False)
    embed.set_footer(text="High Roller Club | Round History")
    
    return embed

def create_crash_leaderboard_embed(leaderboard_data: List[Dict[str, any]], leaderboard_type: str) -> nextcord.Embed:
    """Creates embed for crash game leaderboards."""
    type_titles = {
        'highest_multiplier': '🏆 Highest Multiplier Survived',
        'biggest_win': '💰 Biggest Single Win',
        'total_winnings': '📈 Total Crash Winnings'
    }
    
    title = type_titles.get(leaderboard_type, '🚀 Crash Leaderboard')
    
    embed = _create_branded_embed(
        title=title,
        description="Top performers in the crash game",
        color=nextcord.Color.gold()
    )
    
    if not leaderboard_data:
        embed.add_field(name="No Data", value="No leaderboard data available yet.", inline=False)
        return embed
    
    leaderboard_text = ""
    for i, entry in enumerate(leaderboard_data[:10], 1):  # Top 10
        username = entry.get('username', 'Unknown')
        value = entry.get('value', 0)
        
        if leaderboard_type == 'highest_multiplier':
            value_str = f"{value:.2f}x"
        else:
            value_str = f"{int(value):,} <:chips:1396988413151940629>"
        
        medal = ["🥇", "🥈", "🥉"][i-1] if i <= 3 else f"{i}."
        leaderboard_text += f"{medal} **{username}** - {value_str}\n"
    
    embed.add_field(name="Rankings", value=leaderboard_text, inline=False)
    embed.set_footer(text="High Roller Club | Crash Leaderboards")
    
    return embed
