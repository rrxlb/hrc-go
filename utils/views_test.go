package utils

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestOptimizeEmbedPayload(t *testing.T) {
	// Test with nil embed
	if result := OptimizeEmbedPayload(nil); result != nil {
		t.Errorf("Expected nil for nil input, got %v", result)
	}

	// Test with empty embed
	embed := &discordgo.MessageEmbed{}
	result := OptimizeEmbedPayload(embed)
	if result == nil {
		t.Error("Expected non-nil result for empty embed")
	}

	// Test with populated embed
	embed = &discordgo.MessageEmbed{
		Title:       "  Test Title  ",
		Description: "  Test Description  ",
		Color:       0xFF0000,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "  Footer Text  ",
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: "https://example.com/thumb.png",
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "  Field 1  ",
				Value:  "  Value 1  ",
				Inline: true,
			},
			{
				Name:   "",
				Value:  "Empty Name",
				Inline: false,
			},
			{
				Name:   "Field 3",
				Value:  "",
				Inline: false,
			},
		},
	}

	result = OptimizeEmbedPayload(embed)

	// Check that whitespace was trimmed
	if result.Title != "Test Title" {
		t.Errorf("Expected 'Test Title', got '%s'", result.Title)
	}

	if result.Description != "Test Description" {
		t.Errorf("Expected 'Test Description', got '%s'", result.Description)
	}

	// Check that color was preserved
	if result.Color != 0xFF0000 {
		t.Errorf("Expected color 0xFF0000, got %d", result.Color)
	}

	// Check that footer was preserved and trimmed
	if result.Footer == nil || result.Footer.Text != "Footer Text" {
		t.Errorf("Expected trimmed footer text 'Footer Text', got %v", result.Footer)
	}

	// Check that thumbnail was preserved
	if result.Thumbnail == nil || result.Thumbnail.URL != "https://example.com/thumb.png" {
		t.Errorf("Expected thumbnail URL, got %v", result.Thumbnail)
	}

	// Check that only valid fields were included (first field only)
	if len(result.Fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(result.Fields))
	}

	if len(result.Fields) > 0 && result.Fields[0].Name != "Field 1" {
		t.Errorf("Expected field name 'Field 1', got '%s'", result.Fields[0].Name)
	}
}

func TestIsNonRetryableError(t *testing.T) {
	// Test nil error
	if isNonRetryableError(nil) {
		t.Error("Expected false for nil error")
	}

	// Test retryable errors
	retryableErrors := []string{
		"network timeout",
		"connection refused",
		"500 internal server error",
	}

	for _, errMsg := range retryableErrors {
		err := &MockError{Message: errMsg}
		if isNonRetryableError(err) {
			t.Errorf("Expected error '%s' to be retryable", errMsg)
		}
	}

	// Test non-retryable errors
	nonRetryableErrors := []string{
		"Unknown Webhook",
		"\"code\": 10015",
		"Unknown interaction",
		"400 bad request",
	}

	for _, errMsg := range nonRetryableErrors {
		err := &MockError{Message: errMsg}
		if !isNonRetryableError(err) {
			t.Errorf("Expected error '%s' to be non-retryable", errMsg)
		}
	}
}

func TestIsWebhookExpiredError(t *testing.T) {
	// Test nil error
	if isWebhookExpiredError(nil) {
		t.Error("Expected false for nil error")
	}

	// Test webhook expired errors
	expiredErrors := []string{
		"Unknown Webhook",
		"\"code\": 10015",
		"404 not found",
		"Unknown interaction",
	}

	for _, errMsg := range expiredErrors {
		err := &MockError{Message: errMsg}
		if !isWebhookExpiredError(err) {
			t.Errorf("Expected error '%s' to be webhook expired", errMsg)
		}
	}

	// Test non-expired errors
	normalErrors := []string{
		"network timeout",
		"500 internal server error",
		"connection refused",
	}

	for _, errMsg := range normalErrors {
		err := &MockError{Message: errMsg}
		if isWebhookExpiredError(err) {
			t.Errorf("Expected error '%s' to not be webhook expired", errMsg)
		}
	}
}

// MockError for testing
type MockError struct {
	Message string
}

func (e *MockError) Error() string {
	return e.Message
}