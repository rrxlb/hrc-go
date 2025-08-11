"""
Utility functions for managing cooldowns and data updates across cogs.
This consolidates repetitive cooldown check logic and data update patterns.
"""

from datetime import datetime, timedelta
from typing import Dict, Any, Optional, List, Tuple
import asyncio
import time

from .database import get_user, update_user
from .cache import get_cached_cooldown_data, cache_cooldown_data, invalidate_cooldown_cache


class CooldownManager:
    """Centralized cooldown management with intelligent caching."""
    
    def __init__(self):
        self._batch_updates: Dict[int, Dict[str, Any]] = {}
        self._batch_lock = asyncio.Lock()
        self._batch_timeout = 1.0  # Batch updates for 1 second
        
    async def check_cooldown(self, user_id: int, cooldown_type: str, 
                           cooldown_duration: timedelta) -> Tuple[bool, Optional[timedelta]]:
        """
        Check if a cooldown has expired for a user.
        
        Returns:
            (is_available, time_remaining)
        """
        # Try cache first
        cached_data = get_cached_cooldown_data(user_id)
        
        if cached_data and cooldown_type in cached_data:
            last_used = cached_data[cooldown_type]
        else:
            # Fetch from database
            user_data = await get_user(user_id)
            cooldown_field = f"last_{cooldown_type}"
            last_used = user_data.get(cooldown_field)
            
            # Cache the cooldown data
            cooldown_data = {cooldown_type: last_used}
            cache_cooldown_data(user_id, cooldown_data)
        
        if not last_used:
            return True, None
            
        current_time = datetime.utcnow()
        next_available = last_used + cooldown_duration
        
        if current_time >= next_available:
            return True, None
        else:
            time_remaining = next_available - current_time
            return False, time_remaining
    
    async def update_cooldown(self, user_id: int, cooldown_type: str, 
                            additional_updates: Optional[Dict[str, Any]] = None,
                            batch: bool = True) -> Dict[str, Any]:
        """
        Update a cooldown timestamp and optionally other user data.
        
        Args:
            user_id: User ID to update
            cooldown_type: Type of cooldown (e.g., 'hourly', 'daily')
            additional_updates: Additional fields to update
            batch: Whether to batch this update with others
            
        Returns:
            Updated user data
        """
        current_time = datetime.utcnow()
        cooldown_field = f"last_{cooldown_type}"
        
        update_data = {cooldown_field: current_time}
        if additional_updates:
            update_data.update(additional_updates)
        
        if batch:
            return await self._add_to_batch(user_id, update_data)
        else:
            updated_user = await update_user(user_id, **update_data)
            invalidate_cooldown_cache(user_id)
            return updated_user
    
    async def _add_to_batch(self, user_id: int, update_data: Dict[str, Any]) -> Dict[str, Any]:
        """Add update to batch and process if needed."""
        async with self._batch_lock:
            if user_id not in self._batch_updates:
                self._batch_updates[user_id] = {}
                
            # Merge updates (later updates override earlier ones)
            for key, value in update_data.items():
                if key.endswith('_increment'):
                    # Handle incremental updates by summing them
                    base_key = key[:-10]  # Remove '_increment'
                    if key in self._batch_updates[user_id]:
                        self._batch_updates[user_id][key] += value
                    else:
                        self._batch_updates[user_id][key] = value
                else:
                    self._batch_updates[user_id][key] = value
            
            # Schedule batch processing if this is the first update
            if len(self._batch_updates) == 1:
                asyncio.create_task(self._process_batch_after_delay())
        
        # Return current user data (will be updated in batch)
        return await get_user(user_id)
    
    async def _process_batch_after_delay(self):
        """Process batched updates after a delay."""
        await asyncio.sleep(self._batch_timeout)
        await self.flush_batch()
    
    async def flush_batch(self):
        """Process all batched updates immediately."""
        async with self._batch_lock:
            if not self._batch_updates:
                return
                
            # Process all batched updates
            update_tasks = []
            users_to_clear = list(self._batch_updates.keys())
            
            for user_id, updates in self._batch_updates.items():
                task = update_user(user_id, **updates)
                update_tasks.append(task)
            
            # Clear batch before processing to avoid blocking new updates
            self._batch_updates.clear()
            
            # Process updates concurrently
            if update_tasks:
                await asyncio.gather(*update_tasks, return_exceptions=True)
                
            # Clear caches for all updated users
            for user_id in users_to_clear:
                invalidate_cooldown_cache(user_id)


# Global cooldown manager instance
cooldown_manager = CooldownManager()


async def check_and_update_cooldown(user_id: int, cooldown_type: str, 
                                   cooldown_duration: timedelta,
                                   additional_updates: Optional[Dict[str, Any]] = None) -> Tuple[bool, Optional[timedelta], Optional[Dict[str, Any]]]:
    """
    Convenience function to check cooldown and update if available.
    
    Returns:
        (is_available, time_remaining, updated_user_data)
    """
    is_available, time_remaining = await cooldown_manager.check_cooldown(
        user_id, cooldown_type, cooldown_duration
    )
    
    if is_available:
        updated_user = await cooldown_manager.update_cooldown(
            user_id, cooldown_type, additional_updates
        )
        return True, None, updated_user
    else:
        return False, time_remaining, None


async def batch_cooldown_updates(updates: List[Tuple[int, str, Dict[str, Any]]]):
    """
    Batch multiple cooldown updates for efficiency.
    
    Args:
        updates: List of (user_id, cooldown_type, additional_updates) tuples
    """
    tasks = []
    for user_id, cooldown_type, additional_updates in updates:
        task = cooldown_manager.update_cooldown(user_id, cooldown_type, additional_updates, batch=True)
        tasks.append(task)
    
    if tasks:
        await asyncio.gather(*tasks, return_exceptions=True)
        await cooldown_manager.flush_batch()