import nextcord
from nextcord.ext import commands
from nextcord import Interaction, ButtonStyle
from nextcord.ui import Button, View
import asyncio
from typing import Dict, Any

from utils.database import update_user, get_user
from utils.embeds import _create_branded_embed
from utils.constants import PREMIUM_ROLE_ID

# Define premium features
PREMIUM_FEATURES = {
    "xp_display": {
        "name": "XP Display",
        "description": "Show XP gained in game results"
    },
    "wins_losses_display": {
        "name": "Wins & Losses",
        "description": "Show wins and losses in your profile"
    }
}

class PremiumFeatureButton(Button):
    """Button for toggling a premium feature."""
    def __init__(self, feature_key: str, is_enabled: bool):
        self.feature_key = feature_key
        self.is_enabled = is_enabled
        feature_info = PREMIUM_FEATURES[feature_key]
        
        super().__init__(
            style=ButtonStyle.success if is_enabled else ButtonStyle.danger,
            label=feature_info["name"],
            custom_id=f"premium_{feature_key}"
        )
        
        self.disabled = False  # Always enabled for toggling

    async def callback(self, interaction: Interaction):
        # Toggle the feature state
        new_state = not self.is_enabled
        
        # Update the user's premium settings
        user_data = await get_user(interaction.user.id)
        if not user_data:
            await interaction.response.send_message("Error: User data not found.", ephemeral=True)
            return
            
        settings = user_data['premium_settings'] or {}
        settings[self.feature_key] = new_state
        
        await update_user(interaction.user.id, premium_settings=settings)
        
        # Update button style
        self.style = ButtonStyle.success if new_state else ButtonStyle.danger
        
        # Update the embed
        embed = await generate_premium_embed(interaction.user)
        await interaction.response.edit_message(embed=embed, view=self.view)

class PremiumView(View):
    """View containing all premium feature buttons."""
    def __init__(self, user_id: int):
        super().__init__(timeout=None)  # Persistent view
        self.user_id = user_id
        self.buttons = []
    
    async def initialize_buttons(self):
        """Initialize buttons based on user's current settings."""
        # Clear existing buttons
        self.clear_items()
        
        # Get user data to determine current settings
        user_data = await get_user(self.user_id)
        if not user_data:
            return
            
        settings = user_data['premium_settings'] or {}
        
        # Create a button for each premium feature
        for feature_key in PREMIUM_FEATURES.keys():
            is_enabled = settings.get(feature_key, False)
            button = PremiumFeatureButton(feature_key, is_enabled)
            self.add_item(button)

async def generate_premium_embed(user) -> nextcord.Embed:
    """Generate the premium features embed."""
    user_data = await get_user(user.id)
    if not user_data:
        embed = _create_branded_embed(
            title="Premium Features",
            description="Error: Could not load user data.",
            color=nextcord.Color.red()
        )
        return embed
    
    settings = user_data['premium_settings'] or {}
    
    # Check if user has premium role
    has_premium_role = any(role.id == PREMIUM_ROLE_ID for role in user.roles)
    
    embed = _create_branded_embed(
        title="💎 Premium Features",
        color=nextcord.Color.gold() if has_premium_role else nextcord.Color.red()
    )
    embed.set_thumbnail(url="https://res.cloudinary.com/dfoeiotel/image/upload/v1753753476/PR2_oxsxaa.png")
    
    if has_premium_role:
        embed.description = "Toggle your premium features on or off:"
        
        # Show status for each feature
        for feature_key, feature_info in PREMIUM_FEATURES.items():
            is_enabled = settings.get(feature_key, False)
            status = "✅ Enabled" if is_enabled else "❌ Disabled"
            embed.add_field(
                name=feature_info["name"],
                value=f"{status}\n{feature_info['description']}",
                inline=False
            )
    else:
        embed.description = "You need to be a Patreon member to access premium features.\n\nVisit our Patreon page to subscribe and unlock these features!"
        embed.add_field(
            name="Available Features",
            value="• XP Display in game results\n• Wins & Losses in profile\n• Future exclusive features",
            inline=False
        )
    
    return embed

class Premium(commands.Cog):
    """Cog for managing premium features."""
    def __init__(self, bot: commands.Bot):
        self.bot = bot

    @nextcord.slash_command(
        name="premium",
        description="Manage your premium features"
    )
    async def premium(self, interaction: Interaction):
        # Check if user has premium role
        has_premium_role = any(role.id == PREMIUM_ROLE_ID for role in interaction.user.roles)
        
        # Generate the embed
        embed = await generate_premium_embed(interaction.user)
        
        # Create and initialize the view
        view = PremiumView(interaction.user.id)
        if has_premium_role:
            await view.initialize_buttons()
        
        # Send the response
        await interaction.response.send_message(embed=embed, view=view, ephemeral=True)

def setup(bot: commands.Bot):
    bot.add_cog(Premium(bot))
