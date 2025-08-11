"""Notification suppression utilities to prevent duplicate spam messages.

In-memory tracking only (resets on bot restart). Designed to be lightweight.
"""
from typing import Dict

# Track highest level announced per user to avoid repeats for same level
_last_level_announced: Dict[int, int] = {}
# Track prestige value for which a prestige-ready notification was last sent
_last_prestige_ready: Dict[int, int] = {}

def should_announce_level_up(user_id: int, new_level: int) -> bool:
    """Return True if we should announce this level for this user."""
    prev = _last_level_announced.get(user_id, 0)
    if new_level > prev:
        _last_level_announced[user_id] = new_level
        return True
    return False

def should_announce_prestige_ready(user_id: int, prestige: int) -> bool:
    """Return True if we should announce prestige readiness for this prestige tier."""
    prev = _last_prestige_ready.get(user_id)
    if prev != prestige:
        _last_prestige_ready[user_id] = prestige
        return True
    return False

def reset_prestige_ready(user_id: int):
    """Call after a user actually prestiges to allow future notification."""
    if user_id in _last_prestige_ready:
        del _last_prestige_ready[user_id]
