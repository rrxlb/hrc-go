import nextcord
from nextcord.ext import commands
from utils import embeds

class Help(commands.Cog):
    def __init__(self, bot: commands.Bot):
        self.bot = bot

    @nextcord.slash_command(name="help", description="Shows a list of available commands.")
    async def help_command(self, interaction: nextcord.Interaction):
        """Shows a list of available commands."""
        embed = embeds._create_branded_embed(
            title="Help",
            description="Here is a list of available commands:",
            color=nextcord.Color.blue(),
        )

        command_categories = {
            "Casino Games": ["baccarat", "blackjack", "craps", "horl", "mines", "tcpoker", "roulette", "slots"],
            "Bonuses": ["bonus", "hourly", "daily", "weekly", "vote", "claimall", "cooldowns"],
            "Profile / Rank": ["profile", "chips", "prestige", "leaderboard"],
            "Help": ["help"]
        }

        for category, command_names in command_categories.items():
            command_list = []
            for command in self.bot.get_application_commands():
                if command.name in command_names:
                    command_list.append(f"`/{command.name}` - {command.description or 'No description'}")
            
            if command_list:
                embed.add_field(
                    name=category,
                    value="\n".join(command_list),
                    inline=False
                )
        
        await interaction.response.send_message(embed=embed, ephemeral=True)

def setup(bot: commands.Bot):
    bot.add_cog(Help(bot))
