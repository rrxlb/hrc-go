import asyncpg
from typing import Optional, Any, List, Dict
import os
import json
import asyncio
from dotenv import load_dotenv
from .constants import STARTING_CHIPS, MAX_CURRENT_XP
import logging
from .cache import get_cached_user, cache_user, invalidate_user_cache

load_dotenv()

DATABASE_URL = os.getenv("DATABASE_URL")

pool: Optional[asyncpg.Pool] = None

async def setup_database():
    """Initializes the database and creates/updates the users table."""
    global pool
    if pool and not pool.is_closing():
        return

    if not DATABASE_URL:
        raise ValueError("DATABASE_URL not set in .env file")
    
    try:
        # Optimized connection pool settings for better performance
        pool = await asyncpg.create_pool(
            DATABASE_URL, 
            min_size=10,  # Increased minimum connections for better availability
            max_size=20,  # Increased maximum for handling load spikes
            max_queries=50000,  # Limit queries per connection to prevent issues
            max_inactive_connection_lifetime=300,  # Close idle connections after 5 minutes
            init=lambda conn: conn.set_type_codec(
                'jsonb',
                encoder=json.dumps,
                decoder=json.loads,
                schema='pg_catalog'
            )
        )
        if not pool:
            raise ConnectionError("Database connection could not be established.")
        logging.info(f"Database pool created successfully with {pool.get_min_size()}-{pool.get_max_size()} connections")
    except Exception as e:
        logging.error(f"Error creating database pool: {e}")
        pool = None
        raise

    async with pool.acquire() as connection:
        # Create table if it doesn't exist
        await connection.execute(f"""
            CREATE TABLE IF NOT EXISTS users (
                user_id BIGINT PRIMARY KEY,
                chips BIGINT NOT NULL DEFAULT {STARTING_CHIPS},
                total_xp BIGINT NOT NULL DEFAULT 0,
                current_xp BIGINT NOT NULL DEFAULT 0,
                prestige INTEGER NOT NULL DEFAULT 0,
                wins INTEGER NOT NULL DEFAULT 0,
                losses INTEGER NOT NULL DEFAULT 0,
                daily_bonuses_claimed INTEGER NOT NULL DEFAULT 0,
                votes_count INTEGER NOT NULL DEFAULT 0,
                last_hourly TIMESTAMPTZ,
                last_daily TIMESTAMPTZ,
                last_weekly TIMESTAMPTZ,
                last_vote TIMESTAMPTZ,
                last_bonus TIMESTAMPTZ,
                premium_settings JSONB
            )
        """)

        # Add new columns to existing tables if they don't exist
        await connection.execute("""
            DO $$ 
            BEGIN 
                BEGIN
                    ALTER TABLE users ADD COLUMN daily_bonuses_claimed INTEGER NOT NULL DEFAULT 0;
                EXCEPTION
                    WHEN duplicate_column THEN NULL;
                END;
                BEGIN
                    ALTER TABLE users ADD COLUMN votes_count INTEGER NOT NULL DEFAULT 0;
                EXCEPTION
                    WHEN duplicate_column THEN NULL;
                END;
            END $$;
        """)

        # Create indexes for frequently queried columns
        await connection.execute("CREATE INDEX IF NOT EXISTS users_chips_idx ON users (chips DESC)")
        await connection.execute("CREATE INDEX IF NOT EXISTS users_total_xp_idx ON users (total_xp DESC)")
        await connection.execute("CREATE INDEX IF NOT EXISTS users_prestige_idx ON users (prestige DESC, total_xp DESC)")

        # Create achievements table
        await connection.execute("""
            CREATE TABLE IF NOT EXISTS achievements (
                id SERIAL PRIMARY KEY,
                name VARCHAR(100) NOT NULL UNIQUE,
                description TEXT NOT NULL,
                icon VARCHAR(50) NOT NULL,
                category VARCHAR(50) NOT NULL,
                requirement_type VARCHAR(50) NOT NULL,
                requirement_value BIGINT NOT NULL DEFAULT 0,
                chips_reward BIGINT NOT NULL DEFAULT 0,
                xp_reward BIGINT NOT NULL DEFAULT 0,
                hidden BOOLEAN NOT NULL DEFAULT FALSE,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
        """)

        # Create user_achievements table
        await connection.execute("""
            CREATE TABLE IF NOT EXISTS user_achievements (
                id SERIAL PRIMARY KEY,
                user_id BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
                achievement_id INTEGER NOT NULL REFERENCES achievements(id) ON DELETE CASCADE,
                earned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                UNIQUE(user_id, achievement_id)
            )
        """)

        # Create indexes for achievement queries
        await connection.execute("CREATE INDEX IF NOT EXISTS user_achievements_user_id_idx ON user_achievements (user_id)")
        await connection.execute("CREATE INDEX IF NOT EXISTS user_achievements_achievement_id_idx ON user_achievements (achievement_id)")
        await connection.execute("CREATE INDEX IF NOT EXISTS user_achievements_earned_at_idx ON user_achievements (earned_at DESC)")


async def get_user(user_id: int) -> Optional[asyncpg.Record]:
    """
    Retrieves a user's data from the database, utilizing a cache.
    If the user doesn't exist, they are created atomically.
    """
    cached_user = get_cached_user(user_id)
    if cached_user:
        return cached_user

    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")

    for attempt in range(3):
        try:
            async with pool.acquire() as connection:
                # This query atomically gets a user or creates them if they don't exist.
                query = """
                    WITH s AS (
                        SELECT * FROM users WHERE user_id = $1
                    ), i AS (
                        INSERT INTO users (user_id, chips)
                        SELECT $1, $2
                        WHERE NOT EXISTS (SELECT 1 FROM s)
                        RETURNING *
                    )
                    SELECT * FROM i
                    UNION ALL
                    SELECT * FROM s;
                """
                user_data = await connection.fetchrow(query, user_id, STARTING_CHIPS)
                
                if user_data:
                    cache_user(user_id, user_data)
                    return user_data
                return None
        except (asyncpg.exceptions.ConnectionDoesNotExistError, 
                asyncpg.exceptions.ConnectionFailureError) as e:
            if attempt < 2:
                backoff_delay = min(0.5 * (2 ** attempt), 5.0)  # Exponential backoff, max 5s
                logging.warning(f"Database connection failed on attempt {attempt + 1}. Retrying in {backoff_delay}s...")
                await asyncio.sleep(backoff_delay)
            else:
                logging.error(f"Database query failed after 3 attempts: {e}")
                raise
    return None

async def get_multiple_users(user_ids: List[int]) -> Dict[int, asyncpg.Record]:
    """
    Retrieves data for multiple users from the database.
    """
    if not user_ids:
        return {}

    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")

    query = "SELECT * FROM users WHERE user_id = ANY($1)"
    async with pool.acquire() as connection:
        records = await connection.fetch(query, user_ids)
        return {record['user_id']: record for record in records}

async def update_user(user_id: int, **kwargs: Any) -> Optional[asyncpg.Record]:
    """
    Updates a user's data in the database, invalidates the cache, and returns the updated record.
    """
    global pool
    if not kwargs or not pool:
        return None

    set_clauses = []
    values = []
    i = 1
    for key, value in kwargs.items():
        if key.endswith("_increment"):
            column = key[:-10]
            set_clauses.append(f"{column} = {column} + ${i}")
        else:
            set_clauses.append(f"{key} = ${i}")
        
        values.append(value)
        i += 1
    
    set_clause_str = ", ".join(set_clauses)
    
    for attempt in range(3):
        try:
            async with pool.acquire() as connection:
                query = f"UPDATE users SET {set_clause_str} WHERE user_id = ${len(values) + 1} RETURNING *"
                updated_user = await connection.fetchrow(query, *values, user_id)

                if updated_user:
                    invalidate_user_cache(user_id)
                    return updated_user
                return None
        except (asyncpg.exceptions.ConnectionDoesNotExistError, 
                asyncpg.exceptions.ConnectionFailureError) as e:
            if attempt < 2:
                backoff_delay = min(0.5 * (2 ** attempt), 5.0)  # Exponential backoff, max 5s
                logging.warning(f"Database update failed on attempt {attempt + 1}. Retrying in {backoff_delay}s...")
                await asyncio.sleep(backoff_delay)
            else:
                logging.error(f"Database update failed after 3 attempts: {e}")
                raise
    return None

async def batch_update_users(updates: List[Dict[str, Any]]):
    """
    Atomically updates multiple users' data in a single transaction using executemany.
    Each dict in the list should have 'user_id' and the columns to update.
    """
    global pool
    if not updates or not pool:
        return

    # Prepare data for executemany, using .get() for safety
    update_data = [
        (
            u.get('chips_increment', 0),
            u.get('wins_increment', 0),
            u.get('losses_increment', 0),
            u.get('total_xp_increment', 0),
            u.get('current_xp_increment', 0),
            u['user_id'] # user_id is required
        ) for u in updates
    ]

    query = """
        UPDATE users
        SET
            chips = chips + $1,
            wins = wins + $2,
            losses = losses + $3,
            total_xp = total_xp + $4,
            current_xp = current_xp + $5
        WHERE user_id = $6
    """
    
    for attempt in range(3):
        try:
            async with pool.acquire() as connection:
                async with connection.transaction():
                    await connection.executemany(query, update_data)
            
            # Invalidate cache for all updated users
            for u in updates:
                invalidate_user_cache(u['user_id'])
            return
        except (asyncpg.exceptions.ConnectionDoesNotExistError, 
                asyncpg.exceptions.ConnectionFailureError) as e:
            if attempt < 2:
                backoff_delay = min(0.5 * (2 ** attempt), 5.0)  # Exponential backoff, max 5s
                logging.warning(f"Database batch update failed on attempt {attempt + 1}. Retrying in {backoff_delay}s...")
                await asyncio.sleep(backoff_delay)
            else:
                logging.error(f"Database batch update failed after 3 attempts: {e}")
                raise


async def get_leaderboard(sort_by: str, limit: int = 10) -> List[asyncpg.Record]:
    """
    Retrieves leaderboard data securely.
    """
    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")

    # Whitelist allowed columns for sorting to prevent SQL injection
    allowed_sort_columns = ["chips", "total_xp", "prestige", "wins", "losses"]
    if sort_by not in allowed_sort_columns:
        raise ValueError(f"Invalid sort key: {sort_by}")

    order_by_clause = f"ORDER BY {sort_by} DESC"
    if sort_by == 'prestige':
        order_by_clause += ", total_xp DESC"

    query = f"SELECT * FROM users {order_by_clause} LIMIT $1"
    
    async with pool.acquire() as connection:
        return await connection.fetch(query, limit)

async def close_database():
    """Closes the database connection pool."""
    global pool
    if pool:
        await pool.close()
        pool = None


# Achievement-related functions
async def get_all_achievements(include_hidden: bool = False) -> List[asyncpg.Record]:
    """Get all achievements from the database."""
    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")
    
    query = "SELECT * FROM achievements"
    if not include_hidden:
        query += " WHERE hidden = FALSE"
    # Order by category then by numeric difficulty (lowest to highest) then by name
    query += " ORDER BY category, requirement_value ASC, name"
    
    async with pool.acquire() as connection:
        return await connection.fetch(query)


async def get_achievement_by_id(achievement_id: int) -> Optional[asyncpg.Record]:
    """Get a specific achievement by ID."""
    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")
    
    query = "SELECT * FROM achievements WHERE id = $1"
    async with pool.acquire() as connection:
        return await connection.fetchrow(query, achievement_id)


async def get_user_achievements(user_id: int) -> List[asyncpg.Record]:
    """Get all achievements earned by a user with achievement details."""
    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")
    
    query = """
        SELECT a.*, ua.earned_at
        FROM user_achievements ua
        JOIN achievements a ON ua.achievement_id = a.id
        WHERE ua.user_id = $1
        ORDER BY ua.earned_at DESC
    """
    async with pool.acquire() as connection:
        return await connection.fetch(query, user_id)


async def award_achievement(user_id: int, achievement_id: int) -> Optional[asyncpg.Record]:
    """Award an achievement to a user if they don't already have it."""
    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")
    
    async with pool.acquire() as connection:
        # Check if user already has the achievement
        existing = await connection.fetchrow(
            "SELECT 1 FROM user_achievements WHERE user_id = $1 AND achievement_id = $2",
            user_id, achievement_id
        )
        
        if existing:
            return None  # User already has this achievement
        
        # Award the achievement
        await connection.execute(
            "INSERT INTO user_achievements (user_id, achievement_id) VALUES ($1, $2)",
            user_id, achievement_id
        )
        
        # Get the achievement details for rewards
        achievement = await connection.fetchrow(
            "SELECT * FROM achievements WHERE id = $1", achievement_id
        )
        
        if achievement and (achievement['chips_reward'] > 0 or achievement['xp_reward'] > 0):
            # Award chips and XP rewards (cap current_xp at MAX_CURRENT_XP)
            await connection.execute("""
                UPDATE users 
                SET chips = chips + $2, 
                    total_xp = total_xp + $3, 
                    current_xp = LEAST(current_xp + $3, $4)
                WHERE user_id = $1
            """, user_id, achievement['chips_reward'], achievement['xp_reward'], MAX_CURRENT_XP)
            
            # Invalidate cache
            invalidate_user_cache(user_id)
        
        return achievement


async def check_achievement_progress(user_id: int, achievement_type: str, current_value: int) -> List[asyncpg.Record]:
    """Check if user should be awarded any achievements based on their progress."""
    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")
    
    async with pool.acquire() as connection:
        # Get achievements of the specified type that user hasn't earned yet
        query = """
            SELECT a.*
            FROM achievements a
            WHERE a.requirement_type = $1 
              AND a.requirement_value <= $2
              AND a.id NOT IN (
                  SELECT ua.achievement_id 
                  FROM user_achievements ua 
                  WHERE ua.user_id = $3
              )
            ORDER BY a.requirement_value ASC
        """
        return await connection.fetch(query, achievement_type, current_value, user_id)


async def get_achievement_categories() -> List[str]:
    """Get all unique achievement categories."""
    global pool
    if not pool:
        raise ConnectionError("Database pool is not available.")
    
    query = "SELECT DISTINCT category FROM achievements ORDER BY category"
    async with pool.acquire() as connection:
        records = await connection.fetch(query)
        return [record['category'] for record in records]

async def parse_bet(bet_input: str, user_chips: int) -> int:
    """
    Parses a bet input string ('all', 'half', or a number) into an integer.
    Raises ValueError for invalid inputs.
    """
    bet_input = bet_input.lower()
    if bet_input == 'all':
        return user_chips
    elif bet_input == 'half':
        return user_chips // 2
    
    try:
        bet_amount = int(bet_input)
    except ValueError:
        raise ValueError("Invalid bet. Please enter a number, 'all', or 'half'.")

    if bet_amount <= 0:
        raise ValueError("Bet must be a positive number.")
        
    return bet_amount
