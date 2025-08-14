package utils

import (
	"context"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// AnimationFrame represents a single frame in an animation sequence
type AnimationFrame struct {
	Embed      *discordgo.MessageEmbed
	Components []discordgo.MessageComponent
	Delay      time.Duration
}

// AnimationSequence manages a sequence of animation frames
type AnimationSequence struct {
	ID      string
	Frames  []AnimationFrame
	Context context.Context
	Cancel  context.CancelFunc
}

// AnimationManager manages background animation sequences
type AnimationManager struct {
	sequences map[string]*AnimationSequence
	mutex     sync.RWMutex
}

// Global animation manager
var Animations = &AnimationManager{
	sequences: make(map[string]*AnimationSequence),
}

// StartAnimation starts a new animation sequence in background
func (am *AnimationManager) StartAnimation(
	id string,
	session *discordgo.Session,
	channelID, messageID string,
	frames []AnimationFrame,
) {
	am.mutex.Lock()
	defer am.mutex.Unlock()

	// Cancel existing animation with same ID
	if existing, exists := am.sequences[id]; exists {
		existing.Cancel()
	}

	// Create new animation context
	ctx, cancel := context.WithCancel(context.Background())
	sequence := &AnimationSequence{
		ID:      id,
		Frames:  frames,
		Context: ctx,
		Cancel:  cancel,
	}

	am.sequences[id] = sequence

	// Start animation in background
	go am.runAnimation(sequence, session, channelID, messageID)
}

// runAnimation executes the animation sequence
func (am *AnimationManager) runAnimation(
	sequence *AnimationSequence,
	session *discordgo.Session,
	channelID, messageID string,
) {
	defer func() {
		// Clean up after animation completes
		am.mutex.Lock()
		delete(am.sequences, sequence.ID)
		am.mutex.Unlock()
	}()

	for i, frame := range sequence.Frames {
		// Check if animation was cancelled
		select {
		case <-sequence.Context.Done():
			return
		default:
		}

		// Wait for frame delay (except first frame)
		if i > 0 && frame.Delay > 0 {
			select {
			case <-time.After(frame.Delay):
			case <-sequence.Context.Done():
				return
			}
		}

		// Update message with frame content
		edit := &discordgo.MessageEdit{
			ID:         messageID,
			Channel:    channelID,
			Embeds:     &[]*discordgo.MessageEmbed{frame.Embed},
			Components: &frame.Components,
		}

		_, err := session.ChannelMessageEditComplex(edit)
		if err != nil {
			BotLogf("ANIMATION", "Failed to update animation frame %d for sequence %s: %v", i, sequence.ID, err)
			// Continue with animation even if one frame fails
		}
	}
}

// CancelAnimation cancels a running animation sequence
func (am *AnimationManager) CancelAnimation(id string) {
	am.mutex.RLock()
	sequence, exists := am.sequences[id]
	am.mutex.RUnlock()

	if exists {
		sequence.Cancel()
	}
}

// IsAnimationRunning checks if an animation is currently running
func (am *AnimationManager) IsAnimationRunning(id string) bool {
	am.mutex.RLock()
	defer am.mutex.RUnlock()
	_, exists := am.sequences[id]
	return exists
}

// EditAfterResponse implements the edit-after-response pattern for smooth game updates
// Sends immediate basic response, then updates with full content after a brief delay
func EditAfterResponse(
	session *discordgo.Session,
	interaction *discordgo.InteractionCreate,
	immediateEmbed *discordgo.MessageEmbed,
	finalEmbed *discordgo.MessageEmbed,
	finalComponents []discordgo.MessageComponent,
	updateDelay time.Duration,
) error {
	// Send immediate response
	err := SendInteractionResponseWithTimeout(session, interaction, immediateEmbed, nil, false, 1*time.Second)
	if err != nil {
		return err
	}

	// Schedule update with final content
	go func() {
		time.Sleep(updateDelay)
		UpdateInteractionResponseWithTimeout(session, interaction, finalEmbed, finalComponents, 2*time.Second)
	}()

	return nil
}

// QuickGameResponse provides the fastest possible response for game commands
// Uses minimal loading embed, then updates with game content
func QuickGameResponse(
	session *discordgo.Session,
	interaction *discordgo.InteractionCreate,
	gameEmbed *discordgo.MessageEmbed,
	gameComponents []discordgo.MessageComponent,
	gameTitle string,
) error {
	// Create minimal loading embed
	loadingEmbed := &discordgo.MessageEmbed{
		Title: "🎮 " + gameTitle,
		Color: 0x7289DA,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Preparing game...",
		},
	}

	// Use edit-after-response pattern with 100ms delay
	return EditAfterResponse(
		session,
		interaction,
		loadingEmbed,
		gameEmbed,
		gameComponents,
		100*time.Millisecond,
	)
}