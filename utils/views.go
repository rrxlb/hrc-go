package utils

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ComponentHandler represents a function that handles component interactions
type ComponentHandler func(*discordgo.Session, *discordgo.InteractionCreate) error

// ComponentManager manages interactive message components and their handlers
type ComponentManager struct {
	handlers map[string]ComponentHandler
}

// Global component manager
var Components = &ComponentManager{
	handlers: make(map[string]ComponentHandler),
}

// RegisterHandler registers a component handler for a specific custom ID
func (cm *ComponentManager) RegisterHandler(customID string, handler ComponentHandler) {
	cm.handlers[customID] = handler
}

// HandleInteraction handles a component interaction
func (cm *ComponentManager) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	customID := i.MessageComponentData().CustomID

	handler, exists := cm.handlers[customID]
	if !exists {
		return fmt.Errorf("no handler registered for component: %s", customID)
	}

	return handler(s, i)
}

// GameView represents a Discord message view with interactive components
type GameView struct {
	Embed      *discordgo.MessageEmbed
	Components []discordgo.MessageComponent
	Timeout    time.Duration
}

// NewGameView creates a new game view
func NewGameView(embed *discordgo.MessageEmbed, timeout time.Duration) *GameView {
	return &GameView{
		Embed:      embed,
		Components: make([]discordgo.MessageComponent, 0),
		Timeout:    timeout,
	}
}

// AddComponent adds a component to the view
func (gv *GameView) AddComponent(component discordgo.MessageComponent) {
	gv.Components = append(gv.Components, component)
}

// CreateActionRow creates an action row with buttons
func CreateActionRow(buttons ...discordgo.MessageComponent) discordgo.MessageComponent {
	return discordgo.ActionsRow{
		Components: buttons,
	}
}

// CreateButton creates a button component
func CreateButton(customID, label string, style discordgo.ButtonStyle, disabled bool, emoji *discordgo.ComponentEmoji) discordgo.MessageComponent {
	button := discordgo.Button{
		CustomID: customID,
		Label:    label,
		Style:    style,
		Disabled: disabled,
	}

	if emoji != nil {
		button.Emoji = emoji
	}

	return button
}

// CreateSelectMenu creates a select menu component
func CreateSelectMenu(customID, placeholder string, options []discordgo.SelectMenuOption, minValues, maxValues *int) discordgo.MessageComponent {
	selectMenu := discordgo.SelectMenu{
		CustomID:    customID,
		Placeholder: placeholder,
		Options:     options,
	}

	if minValues != nil {
		selectMenu.MinValues = minValues
	}

	if maxValues != nil {
		selectMenu.MaxValues = *maxValues
	}

	return selectMenu
}

// BlackjackView creates a view for blackjack game
type BlackjackView struct {
	UserID    int64
	GameID    string
	CanHit    bool
	CanStand  bool
	CanDouble bool
	CanSplit  bool
	CanInsure bool
}

// NewBlackjackView creates a new blackjack view
func NewBlackjackView(userID int64, gameID string) *BlackjackView {
	return &BlackjackView{
		UserID:    userID,
		GameID:    gameID,
		CanHit:    true,
		CanStand:  true,
		CanDouble: false,
		CanSplit:  false,
		CanInsure: false,
	}
}

// GetComponents returns the components for the blackjack view
func (bv *BlackjackView) GetComponents() []discordgo.MessageComponent {
	var buttons []discordgo.MessageComponent

	// Hit button
	hitButton := CreateButton(
		"blackjack_hit",
		"Hit",
		discordgo.PrimaryButton,
		!bv.CanHit,
		&discordgo.ComponentEmoji{Name: "🃏"},
	)
	buttons = append(buttons, hitButton)

	// Stand button
	standButton := CreateButton(
		"blackjack_stand",
		"Stand",
		discordgo.SecondaryButton,
		!bv.CanStand,
		&discordgo.ComponentEmoji{Name: "✋"},
	)
	buttons = append(buttons, standButton)

	// Double button
	if bv.CanDouble {
		doubleButton := CreateButton(
			"blackjack_double",
			"Double Down",
			discordgo.SuccessButton,
			false,
			&discordgo.ComponentEmoji{Name: "💰"},
		)
		buttons = append(buttons, doubleButton)
	}

	// Split button
	if bv.CanSplit {
		splitButton := CreateButton(
			"blackjack_split",
			"Split",
			discordgo.SuccessButton,
			false,
			&discordgo.ComponentEmoji{Name: "✂️"},
		)
		buttons = append(buttons, splitButton)
	}

	// Insurance button
	if bv.CanInsure {
		insuranceButton := CreateButton(
			"blackjack_insurance",
			"Insurance",
			discordgo.SecondaryButton,
			false,
			&discordgo.ComponentEmoji{Name: "🛡️"},
		)
		buttons = append(buttons, insuranceButton)
	}

	return []discordgo.MessageComponent{CreateActionRow(buttons...)}
}

// DisableAllButtons disables all buttons in the view
func (bv *BlackjackView) DisableAllButtons() []discordgo.MessageComponent {
	bv.CanHit = false
	bv.CanStand = false
	bv.CanDouble = false
	bv.CanSplit = false
	bv.CanInsure = false

	return bv.GetComponents()
}

// TimeoutView creates a view for timeout messages
func TimeoutView() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		CreateActionRow(
			CreateButton(
				"timeout_acknowledge",
				"Acknowledged",
				discordgo.SecondaryButton,
				true, // Disabled
				&discordgo.ComponentEmoji{Name: "⏰"},
			),
		),
	}
}

// ErrorView creates a view for error messages
func ErrorView() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		CreateActionRow(
			CreateButton(
				"error_dismiss",
				"Dismiss",
				discordgo.DangerButton,
				false,
				&discordgo.ComponentEmoji{Name: "❌"},
			),
		),
	}
}

// ConfirmationView creates a view for confirmation dialogs
func ConfirmationView(confirmID, cancelID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		CreateActionRow(
			CreateButton(
				confirmID,
				"Confirm",
				discordgo.SuccessButton,
				false,
				&discordgo.ComponentEmoji{Name: "✅"},
			),
			CreateButton(
				cancelID,
				"Cancel",
				discordgo.DangerButton,
				false,
				&discordgo.ComponentEmoji{Name: "❌"},
			),
		),
	}
}

// PaginationView creates a view for paginated content
func PaginationView(prevID, nextID string, currentPage, totalPages int) []discordgo.MessageComponent {
	prevDisabled := currentPage <= 1
	nextDisabled := currentPage >= totalPages

	return []discordgo.MessageComponent{
		CreateActionRow(
			CreateButton(
				prevID,
				"Previous",
				discordgo.SecondaryButton,
				prevDisabled,
				&discordgo.ComponentEmoji{Name: "⬅️"},
			),
			CreateButton(
				"page_info",
				fmt.Sprintf("%d/%d", currentPage, totalPages),
				discordgo.SecondaryButton,
				true, // Always disabled - just for info
				nil,
			),
			CreateButton(
				nextID,
				"Next",
				discordgo.SecondaryButton,
				nextDisabled,
				&discordgo.ComponentEmoji{Name: "➡️"},
			),
		),
	}
}

// SendInteractionResponse sends an interaction response with embed and components
func SendInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool) error {
	// Use optimized send with timeout
	return SendInteractionResponseWithTimeout(s, i, embed, components, ephemeral, 5*time.Second)
}

// SendInteractionResponseWithTimeout sends an interaction response with timeout control and optimization
func SendInteractionResponseWithTimeout(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool, timeout time.Duration) error {
	data := &discordgo.InteractionResponseData{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	}
	if ephemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}

	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	}

	// Use optimized Discord API call with performance monitoring
	return DiscordOpt.OptimizedInteractionRespond(s, i.Interaction, response, timeout)
}

// UpdateInteractionResponse updates an interaction response with new embed and components
func UpdateInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	// Use optimized update with timeout
	return UpdateInteractionResponseWithTimeout(s, i, embed, components, 5*time.Second)
}

// UpdateInteractionResponseWithTimeout updates an interaction response with timeout control and optimization
func UpdateInteractionResponseWithTimeout(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, timeout time.Duration) error {
	edit := &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	}

	// Use optimized Discord API call with performance monitoring
	_, err := DiscordOpt.OptimizedInteractionResponseEdit(s, i.Interaction, edit, timeout)
	return err
}

// SendFollowupMessage sends a followup message
func SendFollowupMessage(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool) error {
	params := &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	}
	if ephemeral {
		params.Flags = discordgo.MessageFlagsEphemeral
	}

	_, err := s.FollowupMessageCreate(i.Interaction, true, params)

	return err
}

// DeferInteractionResponse defers an interaction response
func DeferInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, ephemeral bool) error {
	data := &discordgo.InteractionResponseData{}
	if ephemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}

	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: data,
	}

	return s.InteractionRespond(i.Interaction, response)
}

// UpdateComponentInteraction updates a component interaction
func UpdateComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	// Use optimized update with timeout
	return UpdateComponentInteractionWithTimeout(s, i, embed, components, 5*time.Second)
}

// UpdateComponentInteractionWithTimeout updates a component interaction with timeout control and optimization
func UpdateComponentInteractionWithTimeout(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, timeout time.Duration) error {
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	}

	// Use optimized Discord API call with performance monitoring
	return DiscordOpt.OptimizedInteractionRespond(s, i.Interaction, response, timeout)
}

// AcknowledgeComponentInteraction acknowledges a component interaction without updating the message
func AcknowledgeComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}

	return s.InteractionRespond(i.Interaction, response)
}

// IsUserAuthorized checks if the user is authorized to interact with a component
func IsUserAuthorized(i *discordgo.InteractionCreate, authorizedUserID string) bool {
	return i.Member.User.ID == authorizedUserID
}

// GetComponentTimeout returns the default timeout for interactive components
func GetComponentTimeout() time.Duration {
	return 5 * time.Minute // 5 minutes default timeout
}

// ParseUserID converts a Discord user ID string to int64 (exported for game packages)
func ParseUserID(id string) (int64, error) { return strconv.ParseInt(id, 10, 64) }

// EditOriginalInteraction edits the original interaction response (slash command message)
func EditOriginalInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	edit := &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	}
	_, err := s.InteractionResponseEdit(i.Interaction, edit)
	return err
}

// GetOriginalResponseMessage fetches the original interaction response message.
// It performs a no-op edit to retrieve the message object without changing content.
func GetOriginalResponseMessage(s *discordgo.Session, i *discordgo.InteractionCreate) (*discordgo.Message, error) {
	edit := &discordgo.WebhookEdit{}
	return s.InteractionResponseEdit(i.Interaction, edit)
}

// BotLogf provides centralized formatted logging for component/game issues
func BotLogf(area string, format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	message := fmt.Sprintf(format, args...)
	fmt.Printf("[%s] [%s] %s\n", timestamp, area, message)
}

// TryEphemeralFollowup attempts to send a small ephemeral notice if an update failed.
// It ignores errors (e.g., if the token interaction no longer valid).
func TryEphemeralFollowup(s *discordgo.Session, i *discordgo.InteractionCreate, content string) error {
	params := &discordgo.WebhookParams{Content: content, Flags: discordgo.MessageFlagsEphemeral}
	_, err := s.FollowupMessageCreate(i.Interaction, true, params)
	return err
}

// IsDiscordAPIError checks if an error is a Discord API error and returns error details
func IsDiscordAPIError(err error) (isDiscordError bool, code int, message string) {
	if err == nil {
		return false, 0, ""
	}

	if restErr, ok := err.(*discordgo.RESTError); ok {
		return true, restErr.Message.Code, restErr.Message.Message
	}

	return false, 0, err.Error()
}

// IsWebhookExpired checks for expired webhook tokens (fast-fail pattern)
func IsWebhookExpired(err error) bool {
	if err == nil {
		return false
	}

	// Fast string checks for webhook expiration
	errMsg := err.Error()
	return errMsg == "Unknown Webhook" || 
		   errMsg == "The provided webhook does not exist." ||
		   ContainsAny(errMsg, []string{
			   "\"code\": 10015", // Unknown Webhook
			   "\"code\":10015",  // Unknown Webhook (no space)
			   "404",             // Not Found
			   "token is invalid",
		   })
}

// ContainsAny checks if a string contains any of the provided substrings
func ContainsAny(s string, substrings []string) bool {
	for _, substr := range substrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// IsRateLimited checks if error indicates rate limiting
func IsRateLimited(err error) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}

	isDiscord, code, _ := IsDiscordAPIError(err)
	if isDiscord && code == 429 { // Too Many Requests
		// Return suggested retry delay (simplified to 1 second)
		return true, 1 * time.Second
	}

	return false, 0
}

// CircuitBreaker implements fast-fail pattern for Discord API calls
type CircuitBreaker struct {
	maxFailures    int
	resetTimeout   time.Duration
	failureCount   int
	lastFailure    time.Time
	state          string // "closed", "open", "half-open"
	mutex          sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        "closed",
	}
}

// CanExecute checks if the circuit breaker allows execution
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if cb.state == "closed" {
		return true
	}

	if cb.state == "open" {
		// Check if we should transition to half-open
		if time.Since(cb.lastFailure) >= cb.resetTimeout {
			cb.state = "half-open"
			return true
		}
		return false
	}

	// half-open state allows one test request
	return true
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount = 0
	cb.state = "closed"
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount++
	cb.lastFailure = time.Now()

	if cb.failureCount >= cb.maxFailures {
		cb.state = "open"
	}
}

// Global circuit breakers for different Discord API operations
var (
	InteractionCircuitBreaker = NewCircuitBreaker(5, 30*time.Second) // 5 failures, 30s reset
	WebhookCircuitBreaker     = NewCircuitBreaker(3, 15*time.Second) // 3 failures, 15s reset
)

// FastUpdateInteractionResponse provides immediate response for time-sensitive operations
// Returns a channel that will receive the actual update result asynchronously
func FastUpdateInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) <-chan error {
	resultChan := make(chan error, 1)
	
	// Execute Discord API call asynchronously
	go func() {
		defer close(resultChan)
		err := UpdateInteractionResponseWithTimeout(s, i, embed, components, 3*time.Second)
		resultChan <- err
	}()
	
	return resultChan
}

// ImmediateResponsePattern sends a basic response immediately, then updates with full content
// This pattern eliminates "Bot is thinking..." by ensuring Discord gets a response quickly
func ImmediateResponsePattern(s *discordgo.Session, i *discordgo.InteractionCreate, fullEmbed *discordgo.MessageEmbed, fullComponents []discordgo.MessageComponent) error {
	// Send minimal immediate response to prevent "Bot is thinking..."
	loadingEmbed := &discordgo.MessageEmbed{
		Title: "🎮 Starting Game...",
		Color: 0x7289DA, // Discord blue
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Loading game state...",
		},
	}
	
	// Send immediate response
	err := SendInteractionResponseWithTimeout(s, i, loadingEmbed, nil, false, 1*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send immediate response: %w", err)
	}
	
	// Update with full content asynchronously (non-blocking)
	go func() {
		// Brief delay to ensure Discord processed the first response
		time.Sleep(50 * time.Millisecond)
		
		// Update with full game content
		updateErr := UpdateInteractionResponseWithTimeout(s, i, fullEmbed, fullComponents, 3*time.Second)
		if updateErr != nil {
			BotLogf("DISCORD_API", "Failed to update with full content: %v", updateErr)
		}
	}()
	
	return nil
}

// OptimizedGameStart provides the fastest possible game start response pattern
// Specifically designed for game commands that need immediate feedback
func OptimizedGameStart(s *discordgo.Session, i *discordgo.InteractionCreate, gameEmbed *discordgo.MessageEmbed, gameComponents []discordgo.MessageComponent) error {
	// For deferred interactions, update immediately without intermediate loading
	if i.Type == discordgo.InteractionApplicationCommand {
		// This is a deferred slash command - update directly
		return UpdateInteractionResponseWithTimeout(s, i, gameEmbed, gameComponents, 2*time.Second)
	}
	
	// For component interactions, use immediate update
	return UpdateComponentInteractionWithTimeout(s, i, gameEmbed, gameComponents, 2*time.Second)
}
