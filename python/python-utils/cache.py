from cachetools import TTLCache
from typing import Optional, Any, Dict, Tuple, List
from asyncpg import Record
from collections import OrderedDict
from datetime import datetime, timedelta
import time

# Enhanced LRU cache with OrderedDict for O(1) operations
class LRUTTLCache:
    """Enhanced cache with LRU eviction and TTL support for O(1) operations."""
    
    def __init__(self, maxsize: int = 1000, ttl: int = 300):
        self.maxsize = maxsize
        self.ttl = ttl
        self._cache: OrderedDict[Any, Tuple[Any, float]] = OrderedDict()
        self._last_cleanup = time.time()
        self._cleanup_interval = 60  # Cleanup every minute
    
    def get(self, key: Any) -> Optional[Any]:
        """Get item from cache, return None if expired or not found."""
        current_time = time.time()
        
        if key in self._cache:
            value, timestamp = self._cache[key]
            if current_time - timestamp <= self.ttl:
                # Move to end (most recently used)
                self._cache.move_to_end(key)
                return value
            else:
                # Expired, remove it
                del self._cache[key]
        
        # Periodic cleanup
        if current_time - self._last_cleanup > self._cleanup_interval:
            self._cleanup_expired()
            
        return None
    
    def set(self, key: Any, value: Any) -> None:
        """Set item in cache with current timestamp."""
        current_time = time.time()
        
        # Remove if already exists
        if key in self._cache:
            del self._cache[key]
        
        # Add to end
        self._cache[key] = (value, current_time)
        
        # Evict oldest if over limit
        if len(self._cache) > self.maxsize:
            self._cache.popitem(last=False)  # Remove oldest (FIFO when at capacity)
    
    def delete(self, key: Any) -> bool:
        """Remove item from cache. Returns True if item was removed."""
        if key in self._cache:
            del self._cache[key]
            return True
        return False
    
    def _cleanup_expired(self) -> None:
        """Remove expired entries."""
        current_time = time.time()
        expired_keys = []
        
        for key, (_, timestamp) in self._cache.items():
            if current_time - timestamp > self.ttl:
                expired_keys.append(key)
        
        for key in expired_keys:
            del self._cache[key]
            
        self._last_cleanup = current_time
    
    def size(self) -> int:
        """Return current cache size."""
        return len(self._cache)
    
    def clear(self) -> None:
        """Clear all cache entries."""
        self._cache.clear()

# Cache user data for 5 minutes with enhanced LRU
user_cache: LRUTTLCache = LRUTTLCache(maxsize=1000, ttl=300)

# Specialized cache for cooldown data (shorter TTL, smaller size)
cooldown_cache: LRUTTLCache = LRUTTLCache(maxsize=500, ttl=120)

# Achievement cache to reduce database queries
achievement_cache: LRUTTLCache = LRUTTLCache(maxsize=200, ttl=300)  # 5 minute TTL

def get_cached_user(user_id: int) -> Optional[Record]:
    """
    Retrieves a user's data from the cache.
    """
    return user_cache.get(user_id)

def cache_user(user_id: int, user_data: Record) -> None:
    """
    Caches a user's data.
    """
    user_cache.set(user_id, user_data)

def invalidate_user_cache(user_id: int) -> bool:
    """
    Removes a user's data from the cache.
    Returns True if user was cached and removed.
    """
    return user_cache.delete(user_id)

def get_cached_cooldown_data(user_id: int) -> Optional[Dict[str, Any]]:
    """
    Get cached cooldown data for a user.
    """
    return cooldown_cache.get(user_id)

def cache_cooldown_data(user_id: int, cooldown_data: Dict[str, Any]) -> None:
    """
    Cache cooldown data for a user.
    """
    cooldown_cache.set(user_id, cooldown_data)

def invalidate_cooldown_cache(user_id: int) -> bool:
    """
    Remove cooldown data from cache.
    """
    return cooldown_cache.delete(user_id)

def get_cached_achievement_result(user_id: int, achievement_type: str) -> Optional[List[Dict[str, Any]]]:
    """
    Get cached achievement check result for a user and type.
    """
    cache_key = f"{user_id}:{achievement_type}"
    return achievement_cache.get(cache_key)

def cache_achievement_result(user_id: int, achievement_type: str, result: List[Dict[str, Any]]) -> None:
    """
    Cache achievement check result for a user and type.
    """
    cache_key = f"{user_id}:{achievement_type}"
    achievement_cache.set(cache_key, result)

def invalidate_achievement_cache(user_id: int) -> None:
    """
    Invalidate all achievement cache entries for a user.
    """
    # Get all keys that start with the user_id
    keys_to_remove = []
    for key in achievement_cache._cache.keys():
        if isinstance(key, str) and key.startswith(f"{user_id}:"):
            keys_to_remove.append(key)
    
    for key in keys_to_remove:
        achievement_cache.delete(key)

def get_cache_stats() -> Dict[str, Any]:
    """
    Get cache statistics for monitoring.
    """
    return {
        "user_cache_size": user_cache.size(),
        "cooldown_cache_size": cooldown_cache.size(),
        "achievement_cache_size": achievement_cache.size(),
        "user_cache_maxsize": user_cache.maxsize,
        "cooldown_cache_maxsize": cooldown_cache.maxsize,
        "achievement_cache_maxsize": achievement_cache.maxsize
    }