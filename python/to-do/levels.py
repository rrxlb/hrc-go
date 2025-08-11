from .constants import RANKS

def get_xp_for_level(level: int, prestige: int) -> int:
    """Calculates the XP required for a given level, factoring in prestige."""
    base_xp = RANKS[level]["xp_required"]
    if prestige == 0:
        return base_xp
    # Increase XP requirement by 20% for each prestige level, compounded
    return int(base_xp * (1.2 ** prestige))

def get_user_level(xp: int, prestige: int) -> int:
    """Calculates a user's level based on their current XP and prestige."""
    level = 0
    for lvl in sorted(RANKS.keys()):
        required_xp = get_xp_for_level(lvl, prestige)
        if xp >= required_xp:
            level = lvl
        else:
            break
    return level
