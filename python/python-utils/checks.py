import nextcord
from nextcord.ext import commands
from nextcord import Interaction
from typing import Callable, Any
import functools

from .constants import DEVELOPER_ROLE_ID
from .embeds import _create_branded_embed

def is_developer():
    """
    A proper application command check that verifies if the user has the developer role.
    If the user is not a developer, it sends an "under development" embed.
    This decorator dynamically fetches fresh role data to avoid caching issues.
    """
    def decorator(func: Callable) -> Callable:
        @functools.wraps(func)
        async def wrapper(self, interaction: Interaction, *args, **kwargs):
            # Check if this is a guild interaction
            if not interaction.guild:
                await send_under_development_embed(interaction)
                return

            try:
                # Fetch fresh member data to avoid role caching issues
                fresh_member = await interaction.guild.fetch_member(interaction.user.id)
                
                # Check if the user has the developer role
                developer_role = nextcord.utils.get(fresh_member.roles, id=DEVELOPER_ROLE_ID)
                if developer_role is None:
                    await send_under_development_embed(interaction)
                    return
                    
            except nextcord.NotFound:
                # User is not a member of the guild
                await send_under_development_embed(interaction)
                return
            except nextcord.Forbidden:
                # Bot doesn't have permission to fetch member data, fallback to cached data
                if not hasattr(interaction.user, 'roles') or not interaction.user.roles:
                    await send_under_development_embed(interaction)
                    return

                developer_role = nextcord.utils.get(interaction.user.roles, id=DEVELOPER_ROLE_ID)
                if developer_role is None:
                    await send_under_development_embed(interaction)
                    return
            except Exception:
                # Any other error, fallback to cached data
                if not hasattr(interaction.user, 'roles') or not interaction.user.roles:
                    await send_under_development_embed(interaction)
                    return

                developer_role = nextcord.utils.get(interaction.user.roles, id=DEVELOPER_ROLE_ID)
                if developer_role is None:
                    await send_under_development_embed(interaction)
                    return
                
            # User is a developer, proceed with the command
            return await func(self, interaction, *args, **kwargs)
        
        return wrapper
    return decorator

async def send_under_development_embed(interaction: Interaction):
    """Sends a standardized "under development" embed."""
    embed = _create_branded_embed(
        title="🛠️ Under Development",
        description="This feature is currently being worked on and is not yet available to the public. Please watch the announcements channel for updates!",
        color=nextcord.Color.orange()
    )
    # Check if the interaction is already responded to
    if not interaction.response.is_done():
        await interaction.response.send_message(embed=embed, ephemeral=True)


async def ensure_user_exists(user_id: int):
    """Ensure a user exists in the database (create if they don't)."""
    from .database import get_user
    await get_user(user_id)  # This will create the user if they don't exist