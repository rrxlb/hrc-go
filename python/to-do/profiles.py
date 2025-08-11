import nextcord
from nextcord.ext import commands, application_checks
from nextcord import Interaction, SlashOption
from datetime import datetime, timedelta
from typing import Optional

from utils.database import get_user, get_leaderboard
from utils import embeds
from utils.constants import STARTING_CHIPS, RANKS, GUILD_ID
from utils.levels import get_user_level, get_xp_for_level
from utils.views import PrestigeConfirmation, ProfileView

class Profiles(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot

    @nextcord.slash_command(name="profile", description="View your or another user's profile.")
    async def profile(self, interaction: Interaction, user: Optional[nextcord.Member] = None):
        target_user = user or interaction.user
        
        if target_user.id == interaction.user.id:
            user_data = await get_user(target_user.id)
            author_data = user_data
        else:
            user_data = await get_user(target_user.id)
            author_data = await get_user(interaction.user.id)
        
        if not user_data:
            # This should theoretically not be hit due to get_user creating a profile
            await interaction.response.send_message(embed=embeds.error_embed("Could not find profile for this user."), ephemeral=True)
            return

        embed = await embeds.create_profile_embed_with_achievements(interaction, target_user, user_data, author_data)
        view = ProfileView(interaction.user, target_user)
        await interaction.response.send_message(embed=embed, view=view)

    @nextcord.slash_command(name="chips", description="Check your chips balance.")
    async def chips(self, interaction: Interaction):
        user_data = await get_user(interaction.user.id)
        await interaction.response.send_message(f"You have {user_data['chips']:,} <:chips:1396988413151940629>.", ephemeral=True)

    @nextcord.slash_command(name="leaderboard", description="View the server leaderboards.")
    async def leaderboard(self, interaction: Interaction):
        pass # Base command for subcommands

    @leaderboard.subcommand(name="chips", description="Top 10 users by Chips.")
    async def lb_chips(self, interaction: Interaction):
        lb_data = await get_leaderboard('chips')
        embed = await embeds.create_leaderboard_embed(interaction, "High Rollers", lb_data, 'chips')
        await interaction.response.send_message(embed=embed)

    @leaderboard.subcommand(name="xp", description="Top 10 users by total XP.")
    async def lb_xp(self, interaction: Interaction):
        lb_data = await get_leaderboard('total_xp')
        embed = await embeds.create_leaderboard_embed(interaction, "Total XP", lb_data, 'total_xp')
        await interaction.response.send_message(embed=embed)

    @leaderboard.subcommand(name="prestige", description="Top 10 users by prestige.")
    async def lb_prestige(self, interaction: Interaction):
        lb_data = await get_leaderboard('prestige')
        embed = await embeds.create_leaderboard_embed(interaction, "Prestige", lb_data, 'prestige')
        await interaction.response.send_message(embed=embed)

    @nextcord.slash_command(name="prestige", description="Reset your rank to gain a prestige level.")
    async def prestige(self, interaction: Interaction):
        user_data = await get_user(interaction.user.id)
        
        # The level required to prestige is the highest rank available.
        prestige_ready_level = max(RANKS.keys())
        required_xp = get_xp_for_level(prestige_ready_level, user_data['prestige'])

        if user_data['current_xp'] < required_xp:
            await interaction.response.send_message(
                embed=embeds.error_embed(f"You are not yet eligible to prestige. You need {required_xp:,} XP."),
                ephemeral=True
            )
            return

        embed = nextcord.Embed(
            title="<:chips:1396988413151940629> Prestige Confirmation",
            description="Prestige has a price: Every chip you've collected will be reset; you'll have to rank up again to be a High Roller. Only your total XP will be unaffected.",
            color=nextcord.Color.orange()
        )
        
        view = PrestigeConfirmation(interaction.user)
        await interaction.response.send_message(embed=embed, view=view)
        view.message = await interaction.original_message()


def setup(bot: commands.Bot):
    bot.add_cog(Profiles(bot))
