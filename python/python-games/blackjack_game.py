import nextcord
from nextcord.ext import commands, tasks
from nextcord import Interaction, SlashOption
import asyncio
import time
from typing import List, Dict, Any, Optional

from utils.database import get_user, parse_bet
from utils import embeds
from utils.views import BlackjackView, ResumeView
from utils.cards import Card, Deck
from utils.game import Game
from utils.constants import (
    DECK_COUNT,
    DEALER_STAND_VALUE,
    BLACKJACK_PAYOUT,
    FIVE_CARD_CHARLIE_PAYOUT,
    XP_PER_PROFIT,
)

# --- Game State Management ---
active_games: Dict[int, "BlackjackGameLogic"] = {}

# --- Game Logic ---
class BlackjackGameLogic(Game):
    def __init__(self, interaction: Interaction, bet: int):
        super().__init__(interaction, bet)
        self.bets: List[int] = [bet]
        self.deck = Deck(num_decks=DECK_COUNT, game='blackjack')
        self.player_hands: List[List[Card]] = [[]]
        self.dealer_hand: List[Card] = []
        self.player_scores = [0]
        self.dealer_score = 0
        self.view = BlackjackView(self.user, self)
        self.current_hand_index = 0
        self.insurance_bet = 0
        self.results = []
        self.created_at = time.time()  # For cleanup

    async def start_game(self):
        self.player_hands[0].extend([self.deck.deal(), self.deck.deal()])
        self.dealer_hand.extend([self.deck.deal(), self.deck.deal()])
        self.update_scores()

        if self.player_scores[0] == 21:
            # Natural blackjack is an auto win - skip dealer turn and end immediately
            await self.end_game()
        else:
            await self.update_view()

    def update_scores(self):
        self.player_scores = [self._calculate_hand_value(hand) for hand in self.player_hands]
        self.dealer_score = self._calculate_hand_value(self.dealer_hand)

    def _calculate_hand_value(self, hand: List[Card]) -> int:
        value = sum(card.value for card in hand)
        num_aces = sum(1 for card in hand if card.rank == "A")
        while value > 21 and num_aces:
            value -= 10
            num_aces -= 1
        return value

    async def handle_hit(self, interaction: Interaction):
        await interaction.response.defer()
        
        current_hand = self.player_hands[self.current_hand_index]
        current_hand.append(self.deck.deal())
        self.update_scores()

        player_score = self.player_scores[self.current_hand_index]
        
        if current_hand[0].rank == 'A' and len(self.player_hands) > 1:
            await self.handle_stand(interaction, deferred=True)
            return

        if player_score > 21:
            await self.handle_stand(interaction, is_bust=True, deferred=True)
        elif len(current_hand) == 5:
            # 5-card charlie is an auto win - skip dealer turn and end immediately
            await self.handle_five_card_charlie(interaction)
        elif player_score == 21:
            await self.handle_stand(interaction, deferred=True)
        else:
            await self.update_view()

    async def handle_five_card_charlie(self, interaction: Interaction):
        """Handle 5-card charlie as an immediate auto win."""
        if len(self.player_hands) > 1 and self.current_hand_index < len(self.player_hands) - 1:
            # If there are multiple hands, move to the next hand
            self.current_hand_index += 1
            await self.update_view()
        else:
            # End the game immediately - 5-card charlie is an auto win
            await self.end_game()

    async def handle_stand(self, interaction: Interaction, is_bust: bool = False, deferred: bool = False):
        if not is_bust and not deferred:
            await interaction.response.defer()

        if len(self.player_hands) > 1 and self.current_hand_index < len(self.player_hands) - 1:
            self.current_hand_index += 1
            await self.update_view()
        else:
            
            if any(score <= 21 for score in self.player_scores):
                await self.dealer_turn()
            else:
                await self.end_game()

    async def handle_double_down(self, interaction: Interaction):
        await interaction.response.defer()
        
        bet_to_double = self.bets[self.current_hand_index]
        total_bet_required = sum(self.bets) + bet_to_double
        
        if self.user_data['chips'] < total_bet_required:
            await interaction.followup.send(
                embed=embeds.insufficient_chips_embed(
                    required_chips=total_bet_required,
                    current_balance=self.user_data['chips'],
                    bet_description="double down"
                ), 
                ephemeral=True
            )
            return

        self.bets[self.current_hand_index] += bet_to_double
        self.player_hands[self.current_hand_index].append(self.deck.deal())
        self.update_scores()
        await self.handle_stand(interaction, deferred=True)

    async def handle_split(self, interaction: Interaction):
        await interaction.response.defer()
        
        bet_cost = self.bets[0]
        total_bet_required = sum(self.bets) + bet_cost
        
        if self.user_data['chips'] < total_bet_required:
            await interaction.followup.send(
                embed=embeds.insufficient_chips_embed(
                    required_chips=total_bet_required,
                    current_balance=self.user_data['chips'],
                    bet_description="split"
                ), 
                ephemeral=True
            )
            return

        self.player_hands.append([self.player_hands[0].pop(1)])
        self.bets.append(self.bets[0])

        self.player_hands[0].append(self.deck.deal())
        self.player_hands[1].append(self.deck.deal())
        
        self.update_scores()
        await self.update_view()

    async def handle_insurance(self, interaction: Interaction):
        await interaction.response.defer()
        insurance_cost = self.bets[0] / 2
        total_bet_required = sum(self.bets) + insurance_cost

        if self.user_data['chips'] < total_bet_required:
            await interaction.followup.send(
                embed=embeds.insufficient_chips_embed(
                    required_chips=int(total_bet_required),
                    current_balance=self.user_data['chips'],
                    bet_description="insurance"
                ), 
                ephemeral=True
            )
            return

        self.insurance_bet = insurance_cost
        await self.update_view()
        await interaction.followup.send(
            f"You have taken insurance for {int(self.insurance_bet)} chips.",
            ephemeral=True
        )

    async def dealer_turn(self):
        await self.update_view(game_over=True)
        # Reduced delay for better performance - configurable based on server load
        await asyncio.sleep(0.5)

        while self.dealer_score < DEALER_STAND_VALUE:
            self.dealer_hand.append(self.deck.deal())
            self.update_scores()
            await self.update_view(game_over=True)
            # Minimal delay for card draws to maintain game flow without excessive lag
            await asyncio.sleep(0.3)
        
        await self.end_game()

    async def update_view(self, game_over: bool = False, **kwargs):
        if game_over:
            self.view.disable_all_buttons()
        else:
            player_hand = self.player_hands[self.current_hand_index]
            player_score = self.player_scores[self.current_hand_index]
            can_double_down = len(player_hand) == 2 and player_score in [9, 10, 11]
            can_split = len(player_hand) == 2 and player_hand[0].rank == player_hand[1].rank and len(self.player_hands) == 1
            can_insure = self.dealer_hand[0].rank == "A" and len(player_hand) == 2 and self.insurance_bet == 0

            for item in self.view.children:
                custom_id = getattr(item, 'custom_id', None)
                if custom_id == 'double_down':
                    item.disabled = not can_double_down
                elif custom_id == 'split':
                    item.disabled = not can_split
                elif custom_id == 'insurance':
                    item.disabled = not can_insure
                elif custom_id in ['hit', 'stand']:
                    item.disabled = False # Ensure hit/stand are enabled
        
        player_hands_data = []
        for i, hand in enumerate(self.player_hands):
            is_active = i == self.current_hand_index and not game_over
            hand_title = f"Hand {i+1}" if len(self.player_hands) > 1 else "Your Hand"
            if is_active and len(self.player_hands) > 1:
                hand_title = f"▶️ {hand_title}"

            player_hands_data.append({
                "hand": [str(card) for card in hand],
                "score": self.player_scores[i],
                "is_active": is_active,
                "hand_title": hand_title
            })

        dealer_hand_display = [str(self.dealer_hand[0]), "??"] if not game_over else [str(card) for card in self.dealer_hand]
        dealer_score_display = self._calculate_hand_value([self.dealer_hand[0]]) if not game_over else self.dealer_score

        embed = await embeds.create_game_embed(
            player_hands=player_hands_data,
            dealer_hand=dealer_hand_display,
            dealer_value=dealer_score_display,
            bet=sum(self.bets),
            game_over=game_over,
            **kwargs
        )
        
        if not self.view.message:
            self.view.message = await self.interaction.original_message()

        await self.view.message.edit(embed=embed, view=self.view)

    async def end_game(self, folded: bool = False):
        if self.is_game_over:
            return
            
        total_profit = 0
        
        if folded:
            total_profit = -sum(self.bets)
            self.results.append("You forfeited your bet.")
        else:
            for i, hand in enumerate(self.player_hands):
                player_score = self.player_scores[i]
                bet = self.bets[i]
                result_text, payout = self._get_hand_result(hand, player_score, bet)
                
                if len(self.player_hands) > 1:
                    self.results.append(f"Hand {i+1}: {result_text}")
                else:
                    self.results.append(result_text)
                total_profit += payout

            if self.insurance_bet > 0:
                is_dealer_blackjack = self.dealer_score == 21 and len(self.dealer_hand) == 2
                if is_dealer_blackjack:
                    insurance_payout = self.insurance_bet * 2
                    total_profit += insurance_payout
                    self.results.append(f"Insurance pays out {int(insurance_payout)}!")
                else:
                    total_profit -= self.insurance_bet
                    self.results.append("Insurance lost.")

        final_user_data = await super().end_game(profit=total_profit)
        
        new_balance = final_user_data['chips'] if final_user_data else self.user_data['chips']
        xp_gain = int(total_profit * XP_PER_PROFIT) if total_profit > 0 else 0

        if final_user_data and xp_gain > 0:
            old_rank = embeds.get_current_rank(self.user_data['current_xp'])
            new_rank = embeds.get_current_rank(final_user_data['current_xp'])
            if new_rank['name'] != old_rank['name']:
                await self.interaction.followup.send(embed=embeds.rank_up_embed(new_rank['name'], new_rank['icon']), ephemeral=True)

        await self.update_view(game_over=True, outcome_text="\n".join(self.results), new_balance=new_balance, profit=total_profit, xp_gain=xp_gain, user=self.user)
        self._cleanup()

    def _get_hand_result(self, hand, player_score, bet):
        is_player_blackjack = player_score == 21 and len(hand) == 2
        is_dealer_blackjack = self.dealer_score == 21 and len(self.dealer_hand) == 2

        if player_score > 21:
            return "Bust! You lost.", -bet
        if len(hand) == 5 and player_score <= 21:
            return "5-Card Charlie! You win!", int(bet * FIVE_CARD_CHARLIE_PAYOUT)
        if is_player_blackjack and not is_dealer_blackjack:
            return "Blackjack! You win!", int(bet * BLACKJACK_PAYOUT)
        if is_dealer_blackjack and not is_player_blackjack:
            return "Dealer has Blackjack. You lose.", -bet
        if self.dealer_score > 21:
            return "Dealer busts! You win!", bet
        if player_score > self.dealer_score:
            return "You win!", bet
        if self.dealer_score > player_score:
            return "Dealer wins.", -bet
        return "Push.", 0

    def _cleanup(self):
        """Handles the cleanup of the game state and memory."""
        try:
            if self.view:
                # Disable all buttons to prevent further interactions
                self.view.disable_all_buttons()
                self.view.stop()
                
            # Clear references to prevent memory leaks
            self.deck = None
            self.player_hands = []
            self.dealer_hand = []
            self.results = []
            
            # Remove from active games
            if self.user.id in active_games:
                del active_games[self.user.id]
                
        except Exception as e:
            import logging
            logging.error(f"Error during game cleanup: {e}")

    async def handle_timeout(self):
        self.view.disable_all_buttons()
        resume_view = ResumeView(self.user, self, self.view)
        timeout_embed = embeds.create_timeout_embed("Your game has timed out. Would you like to resume?")
        await self.view.message.edit(embed=timeout_embed, view=resume_view)

    async def resume_game(self, interaction: nextcord.Interaction):
        await interaction.response.defer()
        self.view = BlackjackView(self.user, self)
        await self.update_view()

    async def quit_game(self, interaction: nextcord.Interaction):
        await interaction.response.defer()
        await self.end_game(folded=True)

# --- Cog Class ---
class BlackjackGame(commands.Cog):
    def __init__(self, bot: commands.Bot) -> None:
        self.bot = bot
        self.cleanup_old_games.start()

    def cog_unload(self) -> None:
        self.cleanup_old_games.cancel()

    @tasks.loop(minutes=10)  # Run every 10 minutes for better memory management
    async def cleanup_old_games(self) -> None:
        """Remove games older than 30 minutes to prevent memory leaks and improve performance."""
        current_time = time.time()
        to_remove = []
        
        for user_id, game in list(active_games.items()):
            # Remove games older than 30 minutes (reduced from 1 hour)
            if (current_time - game.created_at) > 1800:
                to_remove.append(user_id)
        
        for user_id in to_remove:
            game_to_remove = active_games.pop(user_id, None)
            if game_to_remove:
                # Calculate total bet amount for forfeiture message
                total_bet = sum(game_to_remove.bets) if hasattr(game_to_remove, 'bets') else game_to_remove.bet
                
                # Create cleanup embed with chip forfeiture message
                cleanup_embed = embeds.create_game_cleanup_embed(total_bet)
                
                # Disable all buttons and update message
                if hasattr(game_to_remove, 'view') and game_to_remove.view:
                    for child in game_to_remove.view.children:
                        child.disabled = True
                    
                    try:
                        await game_to_remove.view.message.edit(embed=cleanup_embed, view=game_to_remove.view)
                    except (nextcord.HTTPException, nextcord.NotFound):
                        pass  # Message was deleted or couldn't be edited
                    
                    try:
                        game_to_remove.view.stop()
                    except Exception:
                        pass  # Ignore errors during view stop
        
        if to_remove:
            print(f"Cleaned up {len(to_remove)} old blackjack games. Active games: {len(active_games)}")
            
        # Memory monitoring - warn if too many active games
        if len(active_games) > 100:
            import logging
            logging.warning(f"High number of active blackjack games: {len(active_games)}")

    @cleanup_old_games.before_loop
    async def before_cleanup(self) -> None:
        await self.bot.wait_until_ready()

    @nextcord.slash_command(name="blackjack", description="Start a game of Blackjack.")
    async def blackjack(self, interaction: Interaction, bet: str = SlashOption(description="Chips to wager.", required=True)):
        player_id = interaction.user.id

        if player_id in active_games:
            await interaction.response.send_message(embed=embeds.error_embed("You already have an active game."), ephemeral=True)
            return

        await interaction.response.defer()
        
        game = BlackjackGameLogic(interaction, 0) # Temp bet
        
        if not await game.validate_bet():
            return
            
        try:
            bet_amount = await parse_bet(bet, game.user_data['chips'])
        except ValueError as e:
            await interaction.followup.send(embed=embeds.error_embed(str(e)), ephemeral=True)
            return
            
        game.bet = bet_amount
        game.bets = [bet_amount]

        if game.user_data['chips'] < bet_amount:
            await interaction.followup.send(
                embed=embeds.insufficient_chips_embed(
                    required_chips=bet_amount,
                    current_balance=game.user_data['chips'],
                    bet_description=f"that bet ({bet_amount:,} chips)"
                ), 
                ephemeral=True
            )
            return
            
        active_games[player_id] = game
        await game.start_game()
        
def setup(bot: commands.Bot):
    bot.add_cog(BlackjackGame(bot))
