import nextcord
from nextcord.ext import commands
from utils import database as db
from utils import embeds

# Define the allowed user ID
LOG_CHANNEL_ID = 1396996421340626954
GUILD_ID = 1396567190102347776
ROLE_ID = 1396615290015453195

class Admin(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot

    @nextcord.slash_command(
        name="addchips",
        description="Add chips to a user's balance. Usable by HRC Admins only.",
        guild_ids=[GUILD_ID],
    )
    async def add_chips(
        self,
        interaction: nextcord.Interaction,
        user: nextcord.User,
        amount: int,
        reason: str,
    ):
        """Adds a specified amount of chips to a user's account."""
        # Manual guild and role check
        if interaction.guild.id != GUILD_ID:
            await interaction.response.send_message(
                embed=embeds.error_embed("This command cannot be used in this server."),
                ephemeral=True
            )
            return

        required_role = interaction.guild.get_role(ROLE_ID)
        if required_role not in interaction.user.roles:
            await interaction.response.send_message(
                embed=embeds.error_embed("You do not have permission to use this command."),
                ephemeral=True
            )
            return

        if amount <= 0:
            await interaction.response.send_message(
                embed=embeds.error_embed("The amount must be positive."),
                ephemeral=True
            )
            return

        # Update the user's balance and get the new data in one call
        updated_user = await db.update_user(user.id, chips_increment=amount)
        new_balance = updated_user['chips']

        # Send a confirmation message
        await interaction.response.send_message(
            embed=embeds.success_embed(
                title="Chips Added",
                description=f"Successfully added {amount:,} chips to {user.mention} for: {reason}."
            ),
            ephemeral=True,
        )

        # Log the transaction
        log_channel = self.bot.get_channel(LOG_CHANNEL_ID)
        if log_channel:
            embed = embeds._create_branded_embed(
                title="Chip Transaction Log",
                description=f"**Reason:** {reason}",
                color=nextcord.Color.green(),
            )
            embed.add_field(name="Moderator", value=interaction.user.mention, inline=False)
            embed.add_field(name="User", value=user.mention, inline=False)
            embed.add_field(name="Amount Added", value=f"{amount:,}", inline=False)
            embed.add_field(name="New Balance", value=f"{new_balance:,}", inline=False)
            embed.set_footer(text=f"User ID: {user.id}")
            await log_channel.send(embed=embed)

def setup(bot: commands.Bot):
    bot.add_cog(Admin(bot))
