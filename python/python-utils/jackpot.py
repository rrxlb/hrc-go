import os
from typing import Optional

import asyncpg

from . import database as db

JACKPOT_SEED_DEFAULT = int(os.getenv("JACKPOT_SEED", "2500"))

TABLE_SQL = """
CREATE TABLE IF NOT EXISTS jackpots (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    amount BIGINT NOT NULL
);
"""

async def _ensure_table(conn: asyncpg.Connection) -> None:
    await conn.execute(TABLE_SQL)

async def ensure_jackpot_seeded(seed: Optional[int] = None) -> None:
    seed_value = JACKPOT_SEED_DEFAULT if seed is None else seed
    if not db.pool:
        raise ConnectionError("Database pool is not available.")
    async with db.pool.acquire() as conn:
        await _ensure_table(conn)
        await conn.execute(
            """
            INSERT INTO jackpots (id, amount) VALUES (1, $1)
            ON CONFLICT (id) DO NOTHING
            """,
            seed_value,
        )

async def get_jackpot_amount() -> int:
    if not db.pool:
        raise ConnectionError("Database pool is not available.")
    async with db.pool.acquire() as conn:
        await _ensure_table(conn)
        row = await conn.fetchrow("SELECT amount FROM jackpots WHERE id = 1")
        if row is None:
            await conn.execute("INSERT INTO jackpots (id, amount) VALUES (1, $1)", JACKPOT_SEED_DEFAULT)
            return JACKPOT_SEED_DEFAULT
        return int(row["amount"])

async def contribute_to_jackpot(amount: int) -> None:
    if amount <= 0:
        return
    if not db.pool:
        raise ConnectionError("Database pool is not available.")
    async with db.pool.acquire() as conn:
        await _ensure_table(conn)
        await conn.execute(
            "UPDATE jackpots SET amount = amount + $1 WHERE id = 1",
            amount,
        )

async def win_and_reset_jackpot(seed: Optional[int] = None) -> int:
    seed_value = JACKPOT_SEED_DEFAULT if seed is None else seed
    if not db.pool:
        raise ConnectionError("Database pool is not available.")
    async with db.pool.acquire() as conn:
        await _ensure_table(conn)
        async with conn.transaction():
            row = await conn.fetchrow("SELECT amount FROM jackpots WHERE id = 1 FOR UPDATE")
            current = int(row["amount"]) if row else seed_value
            await conn.execute("UPDATE jackpots SET amount = $1 WHERE id = 1", seed_value)
            return current
