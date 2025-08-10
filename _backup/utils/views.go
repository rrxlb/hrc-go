package utils

import (
	"fmt"
	"log"
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
	log.Printf("Registered component handler for: %s", customID)
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
		button.Emoji = *emoji
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
	
	return []discordgo.MessageComponent{CreateActionRow(buttons...)}
}

// DisableAllButtons disables all buttons in the view
func (bv *BlackjackView) DisableAllButtons() []discordgo.MessageComponent {
	bv.CanHit = false
	bv.CanStand = false
	bv.CanDouble = false
	bv.CanSplit = false
	
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
	flags := uint64(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
			Flags:      flags,
		},
	}
	
	return s.InteractionRespond(i.Interaction, response)
}

// UpdateInteractionResponse updates an interaction response with new embed and components
func UpdateInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	edit := &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &components,
	}
	
	_, err := s.InteractionResponseEdit(i.Interaction, edit)
	return err
}

// SendFollowupMessage sends a followup message
func SendFollowupMessage(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, ephemeral bool) error {
	flags := uint64(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
		Flags:      flags,
	})
	
	return err
}

// DeferInteractionResponse defers an interaction response
func DeferInteractionResponse(s *discordgo.Session, i *discordgo.InteractionCreate, ephemeral bool) error {
	flags := uint64(0)
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	
	response := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: flags,
		},
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