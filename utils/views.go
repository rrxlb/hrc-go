package utils

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
// Optimized version with timeout handling
func SendInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool) error {
	return SendInteractionResponseWithTimeout(s, i, embed, components, ephemeral, 100*time.Millisecond)
}

// SendInteractionResponseWithTimeout sends an interaction response with configurable timeout
func SendInteractionResponseWithTimeout(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool, timeout time.Duration) error {
	start := time.Now()
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

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultCh := make(chan error, 1)

	// Execute the API call in a goroutine
	go func() {
		err := s.InteractionRespond(i.Interaction, response)
		select {
		case resultCh <- err:
		default:
		}
	}()

	// Wait for either success, error, or timeout
	select {
	case err := <-resultCh:
		duration := time.Since(start)
		if err != nil {
			BotLogf("DISCORD_API", "SendInteractionResponse failed: %v", err)
			TrackPerformance("SendInteractionResponse", duration, false, false)
		} else {
			TrackPerformance("SendInteractionResponse", duration, true, false)
		}
		return err
	case <-ctx.Done():
		duration := time.Since(start)
		BotLogf("DISCORD_API", "SendInteractionResponse timed out after %v", timeout)
		TrackPerformance("SendInteractionResponse", duration, false, true)
		return ctx.Err()
	}
}

// UpdateInteractionResponse updates an interaction response with new embed and components
// Optimized version with timeout, retry logic, and fallback mechanisms
func UpdateInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	return UpdateInteractionResponseWithRetry(s, i, embed, components, 100*time.Millisecond, 2)
}

// UpdateInteractionResponseWithRetry updates an interaction response with configurable timeout and retry logic
func UpdateInteractionResponseWithRetry(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, timeout time.Duration, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: wait 50ms * 2^(attempt-1)
			backoff := time.Duration(50*attempt*attempt) * time.Millisecond
			if backoff > 500*time.Millisecond {
				backoff = 500 * time.Millisecond // Cap at 500ms
			}
			time.Sleep(backoff)
		}

		err := updateInteractionResponseAttempt(s, i, embed, components, timeout)
		if err == nil {
			if attempt > 0 {
				BotLogf("DISCORD_API", "UpdateInteractionResponse succeeded on attempt %d", attempt+1)
			}
			return nil
		}

		lastErr = err

		// Don't retry on certain errors that won't succeed
		if isNonRetryableError(err) {
			break
		}

		BotLogf("DISCORD_API", "UpdateInteractionResponse attempt %d failed: %v", attempt+1, err)
	}

	// All retries failed, try fallback methods
	BotLogf("DISCORD_API", "All retry attempts failed, trying fallbacks")
	return tryInteractionResponseFallback(s, i, embed, components, lastErr)
}

// updateInteractionResponseAttempt performs a single attempt to update the interaction response
func updateInteractionResponseAttempt(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, timeout time.Duration) error {
	start := time.Now()
	edit := &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Channel to receive the result
	resultCh := make(chan error, 1)

	// Execute the API call in a goroutine
	go func() {
		_, err := s.InteractionResponseEdit(i.Interaction, edit)
		select {
		case resultCh <- err:
		default:
			// Context was cancelled, ignore result
		}
	}()

	// Wait for either success, error, or timeout
	select {
	case err := <-resultCh:
		duration := time.Since(start)
		TrackPerformance("UpdateInteractionResponse", duration, err == nil, false)
		return err
	case <-ctx.Done():
		duration := time.Since(start)
		TrackPerformance("UpdateInteractionResponse", duration, false, true)
		return ctx.Err()
	}
}

// isNonRetryableError checks if an error should not be retried
func isNonRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Unknown Webhook") ||
		strings.Contains(msg, "\"code\": 10015") ||
		strings.Contains(msg, "Unknown interaction") ||
		strings.Contains(msg, "400") // Bad request won't get better with retry
}

// tryInteractionResponseFallback attempts fallback methods when primary interaction response fails
func tryInteractionResponseFallback(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, originalErr error) error {
	// Attempt 1: Try followup message if webhook still valid
	if !isWebhookExpiredError(originalErr) {
		if err := SendFollowupMessage(s, i, embed, components, false); err == nil {
			BotLogf("DISCORD_API", "Successfully used followup message as fallback")
			return nil
		}
	}

	// Attempt 2: Try direct channel message if we have channel info
	if i.ChannelID != "" {
		if err := sendDirectChannelMessage(s, i.ChannelID, embed, components); err == nil {
			BotLogf("DISCORD_API", "Successfully used direct channel message as fallback")
			return nil
		}
	}

	// All fallbacks failed, return original error
	return fmt.Errorf("interaction response failed with all fallbacks: %w", originalErr)
}

// sendDirectChannelMessage sends a message directly to a channel
func sendDirectChannelMessage(s *discordgo.Session, channelID string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	message := &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultCh := make(chan error, 1)
	go func() {
		_, err := s.ChannelMessageSendComplex(channelID, message)
		select {
		case resultCh <- err:
		default:
		}
	}()

	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isWebhookExpiredError checks if the error indicates an expired webhook
func isWebhookExpiredError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Unknown Webhook") ||
		strings.Contains(msg, "\"code\": 10015") ||
		strings.Contains(msg, "404") ||
		strings.Contains(msg, "Unknown interaction")
}

// SendFollowupMessage sends a followup message with timeout handling
func SendFollowupMessage(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool) error {
	params := &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	}
	if ephemeral {
		params.Flags = discordgo.MessageFlagsEphemeral
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultCh := make(chan error, 1)

	// Execute the API call in a goroutine
	go func() {
		_, err := s.FollowupMessageCreate(i.Interaction, true, params)
		select {
		case resultCh <- err:
		default:
		}
	}()

	// Wait for either success, error, or timeout
	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	}

	return s.InteractionRespond(i.Interaction, response)
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

// UpdateInteractionResponseAsync updates an interaction response asynchronously (fire-and-forget)
// This is useful when you don't need to wait for the response and want maximum speed
func UpdateInteractionResponseAsync(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	go func() {
		err := UpdateInteractionResponseWithRetry(s, i, embed, components, 100*time.Millisecond, 1)
		if err != nil {
			BotLogf("DISCORD_API", "Async UpdateInteractionResponse failed: %v", err)
		}
	}()
}

// OptimizeEmbedPayload ensures embed payload is minimal and efficiently structured
func OptimizeEmbedPayload(embed *discordgo.MessageEmbed) *discordgo.MessageEmbed {
	if embed == nil {
		return embed
	}

	// Create optimized copy
	optimized := &discordgo.MessageEmbed{
		Title:       strings.TrimSpace(embed.Title),
		Description: strings.TrimSpace(embed.Description),
		Color:       embed.Color,
		Timestamp:   embed.Timestamp,
	}

	// Only include footer if it has content
	if embed.Footer != nil && strings.TrimSpace(embed.Footer.Text) != "" {
		optimized.Footer = &discordgo.MessageEmbedFooter{
			Text:    strings.TrimSpace(embed.Footer.Text),
			IconURL: embed.Footer.IconURL,
		}
	}

	// Only include thumbnail if URL is present
	if embed.Thumbnail != nil && embed.Thumbnail.URL != "" {
		optimized.Thumbnail = embed.Thumbnail
	}

	// Only include image if URL is present
	if embed.Image != nil && embed.Image.URL != "" {
		optimized.Image = embed.Image
	}

	// Optimize fields - remove empty ones and trim whitespace
	if len(embed.Fields) > 0 {
		for _, field := range embed.Fields {
			if field != nil && strings.TrimSpace(field.Name) != "" && strings.TrimSpace(field.Value) != "" {
				optimized.Fields = append(optimized.Fields, &discordgo.MessageEmbedField{
					Name:   strings.TrimSpace(field.Name),
					Value:  strings.TrimSpace(field.Value),
					Inline: field.Inline,
				})
			}
		}
	}

	return optimized
}

// UpdateInteractionResponseOptimized updates an interaction response with payload optimization
func UpdateInteractionResponseOptimized(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	optimizedEmbed := OptimizeEmbedPayload(embed)
	return UpdateInteractionResponse(s, i, optimizedEmbed, components)
}

// PerformanceMetrics tracks Discord API response performance
type PerformanceMetrics struct {
	TotalCalls    int64
	SuccessfulCalls int64
	FailedCalls   int64
	TimeoutCalls  int64
	TotalDuration time.Duration
	MaxDuration   time.Duration
	MinDuration   time.Duration
}

var discordAPIMetrics = &PerformanceMetrics{
	MinDuration: time.Hour, // Start with high value
}

// TrackPerformance records performance metrics for Discord API calls
func TrackPerformance(operation string, duration time.Duration, success bool, timedOut bool) {
	discordAPIMetrics.TotalCalls++
	discordAPIMetrics.TotalDuration += duration
	
	if success {
		discordAPIMetrics.SuccessfulCalls++
	} else {
		discordAPIMetrics.FailedCalls++
	}
	
	if timedOut {
		discordAPIMetrics.TimeoutCalls++
	}
	
	if duration > discordAPIMetrics.MaxDuration {
		discordAPIMetrics.MaxDuration = duration
	}
	
	if duration < discordAPIMetrics.MinDuration && duration > 0 {
		discordAPIMetrics.MinDuration = duration
	}
	
	// Log concerning performance
	if duration > 200*time.Millisecond {
		BotLogf("DISCORD_PERF", "SLOW %s: %dms (target: <100ms)", operation, duration.Milliseconds())
	}
}

// GetPerformanceMetrics returns current Discord API performance metrics
func GetPerformanceMetrics() PerformanceMetrics {
	return *discordAPIMetrics
}

// ResetPerformanceMetrics resets the performance tracking metrics
func ResetPerformanceMetrics() {
	discordAPIMetrics.TotalCalls = 0
	discordAPIMetrics.SuccessfulCalls = 0
	discordAPIMetrics.FailedCalls = 0
	discordAPIMetrics.TimeoutCalls = 0
	discordAPIMetrics.TotalDuration = 0
	discordAPIMetrics.MaxDuration = 0
	discordAPIMetrics.MinDuration = time.Hour
}

// LogPerformanceStats logs current Discord API performance statistics
func LogPerformanceStats() {
	if discordAPIMetrics.TotalCalls == 0 {
		BotLogf("DISCORD_PERF", "No Discord API calls recorded yet")
		return
	}

	avgDuration := discordAPIMetrics.TotalDuration / time.Duration(discordAPIMetrics.TotalCalls)
	successRate := float64(discordAPIMetrics.SuccessfulCalls) / float64(discordAPIMetrics.TotalCalls) * 100
	timeoutRate := float64(discordAPIMetrics.TimeoutCalls) / float64(discordAPIMetrics.TotalCalls) * 100

	BotLogf("DISCORD_PERF", "Discord API Stats - Calls: %d, Success: %.1f%%, Timeout: %.1f%%, Avg: %dms, Min: %dms, Max: %dms",
		discordAPIMetrics.TotalCalls, successRate, timeoutRate,
		avgDuration.Milliseconds(), discordAPIMetrics.MinDuration.Milliseconds(), discordAPIMetrics.MaxDuration.Milliseconds())
}
