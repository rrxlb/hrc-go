"""
Achievement system for the Discord bot.
Handles achievement definitions, progress tracking, and rewards.
(Moved from utils/achievements.py to utils/achievements_config.py to avoid name conflict with cogs/achievements.py.)
"""

from typing import List, Dict, Any, Optional
import asyncio
import logging

from .database import (
    get_user, 
    award_achievement, 
    check_achievement_progress,
    get_all_achievements,
    update_user
)

# Achievement type constants
ACHIEVEMENT_TYPES = {
    "total_wins": "Total Wins",
    "total_chips": "Total Chips",
    "total_xp": "Total XP", 
    "prestige": "Prestige Level",
    "games_played": "Games Played",
    "blackjack_wins": "Blackjack Wins",
    "poker_wins": "Poker Wins",
    "slots_wins": "Slots Wins",
    "consecutive_wins": "Win Streak",
    "big_win": "Big Win Amount",
    "daily_bonuses": "Daily Bonuses Claimed",
    "votes": "Bot Votes"
}

# Default achievements to populate the database
DEFAULT_ACHIEVEMENTS = [
    # First Steps
    {
        "name": "First Blood",
        "description": "Win your first game",
        "icon": "🎯",
        "category": "First Steps",
        "requirement_type": "total_wins",
        "requirement_value": 1,
        "chips_reward": 500,
        "xp_reward": 100
    },
    {
        "name": "Getting Started",
        "description": "Reach 5,000 chips",
        "icon": "💰",
        "category": "First Steps", 
        "requirement_type": "total_chips",
        "requirement_value": 5000,
        "chips_reward": 1000,
        "xp_reward": 250
    },
    
    # Wins
    {
        "name": "Beginner's Luck",
        "description": "Win 10 games",
        "icon": "🍀",
        "category": "Wins",
        "requirement_type": "total_wins",
        "requirement_value": 10,
        "chips_reward": 1500,
        "xp_reward": 500
    },
    {
        "name": "Lucky Streak",
        "description": "Win 50 games",
        "icon": "🎰",
        "category": "Wins",
        "requirement_type": "total_wins",
        "requirement_value": 50,
        "chips_reward": 5000,
        "xp_reward": 1500
    },
    {
        "name": "Seasoned Player",
        "description": "Win 100 games",
        "icon": "🎲",
        "category": "Wins",
        "requirement_type": "total_wins", 
        "requirement_value": 100,
        "chips_reward": 10000,
        "xp_reward": 3000
    },
    {
        "name": "Gambling Master",
        "description": "Win 500 games",
        "icon": "👑",
        "category": "Wins",
        "requirement_type": "total_wins",
        "requirement_value": 500,
        "chips_reward": 25000,
        "xp_reward": 7500
    },
    
    # Wealth
    {
        "name": "Small Fortune",
        "description": "Accumulate 25,000 chips",
        "icon": "💵",
        "category": "Wealth",
        "requirement_type": "total_chips",
        "requirement_value": 25000,
        "chips_reward": 5000,
        "xp_reward": 1000
    },
    {
        "name": "Big Money",
        "description": "Accumulate 100,000 chips",
        "icon": "💸",
        "category": "Wealth",
        "requirement_type": "total_chips",
        "requirement_value": 100000,
        "chips_reward": 15000,
        "xp_reward": 2500
    },
    {
        "name": "Millionaire",
        "description": "Accumulate 1,000,000 chips",
        "icon": "🏆",
        "category": "Wealth",
        "requirement_type": "total_chips",
        "requirement_value": 1000000,
        "chips_reward": 100000,
        "xp_reward": 10000
    },
    
    # Experience
    # Additional Experience milestones
    {
        "name": "Novice",
        "description": "Reach 1,000 total XP",
        "icon": "📘",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 1000,
        "chips_reward": 500,
        "xp_reward": 100
    },
    {
        "name": "Learner",
        "description": "Reach 5,000 total XP",
        "icon": "📗",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 5000,
        "chips_reward": 1500,
        "xp_reward": 300
    },
    {
        "name": "Rising Star",
        "description": "Reach 10,000 total XP",
        "icon": "⭐",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 10000,
        "chips_reward": 2500,
        "xp_reward": 500
    },
    {
        "name": "Adept",
        "description": "Reach 25,000 total XP",
        "icon": "⚡",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 25000,
        "chips_reward": 4000,
        "xp_reward": 800
    },
    {
        "name": "Skilled",
        "description": "Reach 50,000 total XP",
        "icon": "🧠",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 50000,
        "chips_reward": 6000,
        "xp_reward": 1500
    },
    {
        "name": "Veteran",
        "description": "Reach 100,000 total XP",
        "icon": "🌟",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 100000,
        "chips_reward": 15000,
        "xp_reward": 2500
    },
    {
        "name": "Elite",
        "description": "Reach 250,000 total XP",
        "icon": "🏅",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 250000,
        "chips_reward": 20000,
        "xp_reward": 4000
    },
    {
        "name": "Legend",
        "description": "Reach 500,000 total XP",
        "icon": "💫",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 500000,
        "chips_reward": 50000,
        "xp_reward": 10000
    },
    {
        "name": "Mythic",
        "description": "Reach 1,000,000 total XP",
        "icon": "🔥",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 1000000,
        "chips_reward": 80000,
        "xp_reward": 15000
    },
    {
        "name": "Ascendant",
        "description": "Reach 2,500,000 total XP",
        "icon": "🚀",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 2500000,
        "chips_reward": 150000,
        "xp_reward": 40000
    },
    {
        "name": "Transcendent",
        "description": "Reach 5,000,000 total XP",
        "icon": "🌌",
        "category": "Experience",
        "requirement_type": "total_xp",
        "requirement_value": 5000000,
        "chips_reward": 300000,
        "xp_reward": 80000,
        "hidden": True
    },
    
    # Prestige
    {
        "name": "Fresh Start",
        "description": "Reach Prestige Level 1",
        "icon": "🔄",
        "category": "Prestige",
        "requirement_type": "prestige",
        "requirement_value": 1,
        "chips_reward": 10000,
        "xp_reward": 5000
    },
    {
        "name": "Second Wind",
        "description": "Reach Prestige Level 3",
        "icon": "🌪️",
        "category": "Prestige",
        "requirement_type": "prestige",
        "requirement_value": 3,
        "chips_reward": 25000,
        "xp_reward": 15000
    },
    {
        "name": "Prestige Master",
        "description": "Reach Prestige Level 5",
        "icon": "👑",
        "category": "Prestige",
        "requirement_type": "prestige",
        "requirement_value": 5,
        "chips_reward": 100000,
        "xp_reward": 50000,
        "hidden": True
    },
    
    # Gaming Milestones
    {
        "name": "Century Club",
        "description": "Play 100 total games",
        "icon": "💯",
        "category": "Gaming",
        "requirement_type": "games_played",
        "requirement_value": 100,
        "chips_reward": 7500,
        "xp_reward": 2000
    },
    {
        "name": "Dedication",
        "description": "Play 500 total games",
        "icon": "🎯",
        "category": "Gaming", 
        "requirement_type": "games_played",
        "requirement_value": 500,
        "chips_reward": 20000,
        "xp_reward": 5000
    },
    {
        "name": "Addiction",
        "description": "Play 1,000 total games",
        "icon": "🎰",
        "category": "Gaming",
        "requirement_type": "games_played", 
        "requirement_value": 1000,
        "chips_reward": 50000,
        "xp_reward": 15000
    },
    
    # Special Achievements
    {
        "name": "Big Winner",
        "description": "Win 50,000 chips in a single game",
        "icon": "💸",
        "category": "Special",
        "requirement_type": "big_win",
        "requirement_value": 50000,
        "chips_reward": 25000,
        "xp_reward": 10000
    },
    {
        "name": "Whale",
        "description": "Win 100,000 chips in a single game", 
        "icon": "🐋",
        "category": "Special",
        "requirement_type": "big_win",
        "requirement_value": 100000,
        "chips_reward": 100000,
        "xp_reward": 25000
    },
    {
        "name": "Regular Visitor",
        "description": "Claim 50 daily bonuses",
        "icon": "📅",
        "category": "Loyalty",
        "requirement_type": "daily_bonuses",
        "requirement_value": 50,
        "chips_reward": 15000,
        "xp_reward": 5000
    },
    {
        "name": "Supporter",
        "description": "Vote for the bot 25 times",
        "icon": "💝",
        "category": "Loyalty",
        "requirement_type": "votes",
        "requirement_value": 25,
        "chips_reward": 10000,
        "xp_reward": 3000
    }
]


async def initialize_achievements():
    """Initialize default achievements in the database."""
    from .database import pool
    import asyncio
    
    # Wait for database pool to be available with timeout
    max_attempts = 10
    for attempt in range(max_attempts):
        if pool and not pool.is_closing():
            break
        if attempt < max_attempts - 1:
            logging.info(f"Waiting for database pool to be ready... (attempt {attempt + 1}/{max_attempts})")
            await asyncio.sleep(1)
        else:
            raise ConnectionError("Database pool is not available after waiting.")
    
    try:
        async with pool.acquire() as connection:
            logging.info(f"Initializing {len(DEFAULT_ACHIEVEMENTS)} default achievements...")
            
            for achievement in DEFAULT_ACHIEVEMENTS:
                try:
                    await connection.execute("""
                        INSERT INTO achievements 
                        (name, description, icon, category, requirement_type, 
                         requirement_value, chips_reward, xp_reward, hidden)
                        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
                        ON CONFLICT (name) DO NOTHING
                    """, 
                    achievement["name"],
                    achievement["description"], 
                    achievement["icon"],
                    achievement["category"],
                    achievement["requirement_type"],
                    achievement["requirement_value"],
                    achievement["chips_reward"],
                    achievement["xp_reward"],
                    achievement.get("hidden", False))
                    
                except Exception as e:
                    logging.error(f"Failed to insert achievement {achievement['name']}: {e}")
                    
            # Verify achievements were created
            result = await connection.fetchrow("SELECT COUNT(*) as count FROM achievements")
            logging.info(f"Achievement initialization complete. {result['count']} achievements in database.")
            
    except Exception as e:
        logging.error(f"Failed to initialize achievements: {e}")
        raise


async def check_and_award_achievements(user_id: int, achievement_types: List[str] = None) -> List[Dict[str, Any]]:
    """
    Check if user should be awarded any achievements and award them.
    Uses caching to reduce database load for frequently checked achievements.
    
    Args:
        user_id: The user's Discord ID
        achievement_types: List of achievement types to check. If None, checks all.
        
    Returns:
        List of newly awarded achievements
    """
    from .cache import get_cached_achievement_result, cache_achievement_result, invalidate_achievement_cache
    
    user_data = await get_user(user_id)
    if not user_data:
        return []
    
    newly_awarded = []
    
    # Define what user values to check for each achievement type
    check_values = {
        "total_wins": user_data.get("wins", 0),
        "total_chips": user_data.get("chips", 0), 
        "total_xp": user_data.get("total_xp", 0),
        "prestige": user_data.get("prestige", 0),
        "games_played": user_data.get("wins", 0) + user_data.get("losses", 0),
        "daily_bonuses": user_data.get("daily_bonuses_claimed", 0),
        "votes": user_data.get("votes_count", 0)
    }
    
    # Check each achievement type
    types_to_check = achievement_types or check_values.keys()
    
    for achievement_type in types_to_check:
        if achievement_type in check_values:
            current_value = check_values[achievement_type]
            
            # Try to get cached result first
            cached_result = get_cached_achievement_result(user_id, achievement_type)
            
            if cached_result is not None:
                # Use cached result, but still check if user qualifies for achievements
                achievements_to_award = cached_result
            else:
                # Get achievements user should have earned from database
                achievements_to_award = await check_achievement_progress(
                    user_id, achievement_type, current_value
                )
                
                # Cache the result for future checks
                cache_achievement_result(user_id, achievement_type, achievements_to_award)
            
            # Award each achievement
            for achievement_record in achievements_to_award:
                awarded = await award_achievement(user_id, achievement_record["id"])
                if awarded:
                    newly_awarded.append({
                        "id": awarded["id"],
                        "name": awarded["name"],
                        "description": awarded["description"],
                        "icon": awarded["icon"],
                        "chips_reward": awarded["chips_reward"],
                        "xp_reward": awarded["xp_reward"]
                    })
                    
                    # Invalidate cache when user earns an achievement to force refresh
                    invalidate_achievement_cache(user_id)
                    
                    logging.info(f"User {user_id} earned achievement: {awarded['name']}")
    
    return newly_awarded


async def get_achievement_progress(user_id: int, category: str = None) -> Dict[str, Any]:
    """
    Get user's achievement progress including completion stats.
    
    Args:
        user_id: The user's Discord ID
        category: Specific category to check (optional)
        
    Returns:
        Dictionary with progress information
    """
    from .database import get_user_achievements, get_all_achievements
    
    user_achievements = await get_user_achievements(user_id)
    all_achievements = await get_all_achievements(include_hidden=False)
    
    # Filter by category if specified
    if category:
        all_achievements = [a for a in all_achievements if a["category"].lower() == category.lower()]
    
    earned_ids = {ua["id"] for ua in user_achievements}
    
    # Group by category
    progress_by_category: Dict[str, Dict[str, Any]] = {}
    
    for achievement in all_achievements:
        cat = achievement["category"]
        if cat not in progress_by_category:
            progress_by_category[cat] = {
                "total": 0,
                "earned": 0,
                "achievements": []
            }
        
        progress_by_category[cat]["total"] += 1
        
        is_earned = achievement["id"] in earned_ids
        if is_earned:
            progress_by_category[cat]["earned"] += 1
            # Find the earned date
            earned_date = None
            for ua in user_achievements:
                if ua["id"] == achievement["id"]:
                    earned_date = ua["earned_at"]
                    break
        else:
            earned_date = None
            
        progress_by_category[cat]["achievements"].append({
            "id": achievement["id"],
            "name": achievement["name"],
            "description": achievement["description"],
            "icon": achievement["icon"],
            "earned": is_earned,
            "earned_at": earned_date,
            "chips_reward": achievement["chips_reward"],
            "xp_reward": achievement["xp_reward"],
            "requirement_type": achievement.get("requirement_type"),
            "requirement_value": achievement.get("requirement_value", 0),
        })
    
    # Sort achievements within each category from lowest to highest requirement_value, then name
    for cat, data in progress_by_category.items():
        data["achievements"].sort(key=lambda a: (a.get("requirement_value", 0), a["name"].lower()))
    
    # Calculate overall stats
    total_achievements = sum(cat["total"] for cat in progress_by_category.values())
    total_earned = sum(cat["earned"] for cat in progress_by_category.values())
    completion_rate = (total_earned / total_achievements * 100) if total_achievements > 0 else 0
    
    # Build sorted categories dict (alphabetical by category)
    sorted_categories = {k: progress_by_category[k] for k in sorted(progress_by_category.keys(), key=lambda s: s.lower())}
    
    return {
        "user_id": user_id,
        "total_achievements": total_achievements,
        "total_earned": total_earned,
        "completion_rate": completion_rate,
        "categories": sorted_categories,
        "recent_achievements": sorted([
            ua for ua in user_achievements 
            if not any(a["hidden"] for a in all_achievements if a["id"] == ua["id"]) 
        ], key=lambda x: x["earned_at"], reverse=True)[:5]
    }
