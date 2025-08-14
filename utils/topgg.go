package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// TopGGClient handles Top.gg API interactions
type TopGGClient struct {
	apiToken string
	botID    string
	client   *http.Client
}

// TopGGVoteResponse represents the response from Top.gg vote check API
type TopGGVoteResponse struct {
	Voted int `json:"voted"` // 0 = hasn't voted, 1 = has voted
}

// NewTopGGClient creates a new Top.gg client
func NewTopGGClient(botID string) *TopGGClient {
	apiToken := os.Getenv("TOPGG_API_TOKEN")
	if apiToken == "" {
		return nil
	}

	return &TopGGClient{
		apiToken: apiToken,
		botID:    botID,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CheckUserVote checks if a user has voted for the bot on Top.gg
func (c *TopGGClient) CheckUserVote(ctx context.Context, userID string) (bool, error) {
	if c == nil || c.apiToken == "" {
		return false, fmt.Errorf("Top.gg client not configured")
	}

	url := fmt.Sprintf("https://top.gg/api/bots/%s/check?userId=%s", c.botID, userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Top.gg API returned status %d", resp.StatusCode)
	}

	var voteResponse TopGGVoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&voteResponse); err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	return voteResponse.Voted == 1, nil
}

// GetVoteURL returns the Top.gg voting URL for the bot
func (c *TopGGClient) GetVoteURL() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("https://top.gg/bot/%s/vote", c.botID)
}

// Global Top.gg client instance
var GlobalTopGGClient *TopGGClient

// InitializeTopGGClient initializes the global Top.gg client
func InitializeTopGGClient(botID string) {
	GlobalTopGGClient = NewTopGGClient(botID)
	if GlobalTopGGClient != nil {
	}
}
