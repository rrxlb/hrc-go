"""
Profiler integration utilities for performance monitoring and optimization validation.
Provides cProfile and memory profiling capabilities for Discord bot operations.
"""

import cProfile
import pstats
import io
import time
import tracemalloc
import asyncio
import functools
from typing import Dict, Any, Optional, Callable, List, Tuple
from contextlib import contextmanager, asynccontextmanager
from dataclasses import dataclass, field
from collections import defaultdict
try:
    import psutil
    PSUTIL_AVAILABLE = True
except ImportError:
    PSUTIL_AVAILABLE = False

@dataclass
class ProfileResult:
    """Container for profiling results."""
    name: str
    duration: float
    function_calls: int
    memory_peak: Optional[int] = None
    memory_current: Optional[int] = None
    stats: Optional[pstats.Stats] = None
    
@dataclass 
class PerformanceMetrics:
    """Container for performance metrics tracking."""
    operation_counts: Dict[str, int] = field(default_factory=lambda: defaultdict(int))
    operation_times: Dict[str, List[float]] = field(default_factory=lambda: defaultdict(list))
    memory_usage: List[Tuple[float, int]] = field(default_factory=list)
    error_counts: Dict[str, int] = field(default_factory=lambda: defaultdict(int))

class PerformanceProfiler:
    """Advanced performance profiler for Discord bot operations."""
    
    def __init__(self, enable_memory_tracking: bool = True):
        self.enable_memory_tracking = enable_memory_tracking
        self.metrics = PerformanceMetrics()
        self._is_tracing = False
        
        if enable_memory_tracking:
            tracemalloc.start()
            self._is_tracing = True
    
    def __del__(self):
        """Clean up memory tracking on deletion."""
        if self._is_tracing:
            try:
                tracemalloc.stop()
            except:
                pass  # Ignore errors during cleanup
    
    @contextmanager
    def profile_function(self, name: str = "anonymous", enable_cprofile: bool = False):
        """Context manager for profiling synchronous functions."""
        start_time = time.perf_counter()
        memory_before = None
        memory_peak = None
        pr = None
        
        if self.enable_memory_tracking and self._is_tracing:
            memory_before = tracemalloc.get_traced_memory()[0]
        
        if enable_cprofile:
            pr = cProfile.Profile()
            pr.enable()
        
        try:
            yield
        except Exception as e:
            self.metrics.error_counts[f"{name}_error"] += 1
            raise
        finally:
            end_time = time.perf_counter()
            duration = end_time - start_time
            
            if enable_cprofile and pr:
                pr.disable()
            
            if self.enable_memory_tracking and self._is_tracing:
                memory_current, memory_peak = tracemalloc.get_traced_memory()
                if memory_before:
                    memory_peak = memory_peak - memory_before
                    memory_current = memory_current - memory_before
            
            # Record metrics
            self.metrics.operation_counts[name] += 1
            self.metrics.operation_times[name].append(duration)
            
            if memory_peak:
                self.metrics.memory_usage.append((time.time(), memory_peak))
    
    @asynccontextmanager
    async def profile_async_function(self, name: str = "async_anonymous", enable_cprofile: bool = False):
        """Context manager for profiling asynchronous functions."""
        start_time = time.perf_counter()
        memory_before = None
        memory_peak = None
        pr = None
        
        if self.enable_memory_tracking and self._is_tracing:
            memory_before = tracemalloc.get_traced_memory()[0]
        
        if enable_cprofile:
            pr = cProfile.Profile()
            pr.enable()
        
        try:
            yield
        except Exception as e:
            self.metrics.error_counts[f"{name}_error"] += 1
            raise
        finally:
            end_time = time.perf_counter()
            duration = end_time - start_time
            
            if enable_cprofile and pr:
                pr.disable()
            
            if self.enable_memory_tracking and self._is_tracing:
                memory_current, memory_peak = tracemalloc.get_traced_memory()
                if memory_before:
                    memory_peak = memory_peak - memory_before
                    memory_current = memory_current - memory_before
            
            # Record metrics
            self.metrics.operation_counts[name] += 1
            self.metrics.operation_times[name].append(duration)
            
            if memory_peak:
                self.metrics.memory_usage.append((time.time(), memory_peak))
    
    def profile_decorator(self, name: Optional[str] = None, enable_cprofile: bool = False):
        """Decorator for profiling functions."""
        def decorator(func):
            func_name = name or f"{func.__module__}.{func.__name__}"
            
            if asyncio.iscoroutinefunction(func):
                @functools.wraps(func)
                async def async_wrapper(*args, **kwargs):
                    async with self.profile_async_function(func_name, enable_cprofile):
                        return await func(*args, **kwargs)
                return async_wrapper
            else:
                @functools.wraps(func)
                def sync_wrapper(*args, **kwargs):
                    with self.profile_function(func_name, enable_cprofile):
                        return func(*args, **kwargs)
                return sync_wrapper
        return decorator
    
    def get_performance_summary(self) -> Dict[str, Any]:
        """Get a comprehensive performance summary."""
        summary = {
            "total_operations": sum(self.metrics.operation_counts.values()),
            "operation_breakdown": dict(self.metrics.operation_counts),
            "average_times": {},
            "total_errors": sum(self.metrics.error_counts.values()),
            "error_breakdown": dict(self.metrics.error_counts),
            "system_metrics": self._get_system_metrics()
        }
        
        # Calculate average execution times
        for operation, times in self.metrics.operation_times.items():
            if times:
                summary["average_times"][operation] = {
                    "avg": sum(times) / len(times),
                    "min": min(times),
                    "max": max(times),
                    "count": len(times)
                }
        
        return summary
    
    def _get_system_metrics(self) -> Dict[str, Any]:
        """Get current system performance metrics."""
        if not PSUTIL_AVAILABLE:
            return {"error": "psutil not available"}
        
        try:
            process = psutil.Process()
            
            return {
                "cpu_percent": process.cpu_percent(),
                "memory_mb": process.memory_info().rss / 1024 / 1024,
                "memory_percent": process.memory_percent(),
                "threads": process.num_threads(),
                "open_files": len(process.open_files()) if hasattr(process, 'open_files') else 0
            }
        except Exception as e:
            return {"error": f"Failed to get system metrics: {e}"}
    
    def get_top_operations(self, limit: int = 10, sort_by: str = "count") -> List[Tuple[str, Dict[str, Any]]]:
        """Get top operations by count or average time."""
        if sort_by == "count":
            sorted_ops = sorted(self.metrics.operation_counts.items(), key=lambda x: x[1], reverse=True)
        elif sort_by == "avg_time":
            sorted_ops = []
            for op, times in self.metrics.operation_times.items():
                if times:
                    avg_time = sum(times) / len(times)
                    sorted_ops.append((op, avg_time))
            sorted_ops.sort(key=lambda x: x[1], reverse=True)
        else:
            raise ValueError("sort_by must be 'count' or 'avg_time'")
        
        return sorted_ops[:limit]
    
    def reset_metrics(self):
        """Reset all collected metrics."""
        self.metrics = PerformanceMetrics()
    
    def export_detailed_profile(self, operation_name: str) -> Optional[str]:
        """Export detailed cProfile results for a specific operation."""
        # This would need to be implemented with more sophisticated tracking
        # For now, return a summary
        if operation_name in self.metrics.operation_times:
            times = self.metrics.operation_times[operation_name]
            count = self.metrics.operation_counts[operation_name]
            
            report = f"""
Detailed Profile for: {operation_name}
=====================================
Total calls: {count}
Total time: {sum(times):.4f}s
Average time: {sum(times)/len(times):.4f}s
Min time: {min(times):.4f}s
Max time: {max(times):.4f}s
"""
            return report
        return None

# Global profiler instance
profiler = PerformanceProfiler()

# Convenience decorators
def profile_performance(name: Optional[str] = None, enable_cprofile: bool = False):
    """Convenience decorator for performance profiling."""
    return profiler.profile_decorator(name, enable_cprofile)

def profile_database_operation(func):
    """Specific decorator for database operations."""
    return profiler.profile_decorator(f"db_{func.__name__}", enable_cprofile=False)(func)

def profile_game_operation(func):
    """Specific decorator for game operations."""  
    return profiler.profile_decorator(f"game_{func.__name__}", enable_cprofile=False)(func)

def profile_cache_operation(func):
    """Specific decorator for cache operations."""
    return profiler.profile_decorator(f"cache_{func.__name__}", enable_cprofile=False)(func)

# Context managers for specific operations
@contextmanager
def profile_command_execution(command_name: str):
    """Profile Discord command execution."""
    with profiler.profile_function(f"command_{command_name}"):
        yield

@asynccontextmanager  
async def profile_async_command_execution(command_name: str):
    """Profile async Discord command execution."""
    async with profiler.profile_async_function(f"command_{command_name}"):
        yield