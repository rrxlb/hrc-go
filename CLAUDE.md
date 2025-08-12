# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based Discord casino bot (`hrc-go`) that provides various gambling games. The project has evolved from a Python implementation, with active Go implementations of all major games and utilities.

### Architecture

The codebase follows a modular architecture with two key directories:

**Active Implementation (Root Directory):**
- `main.go`: Application entry point with Discord bot setup, slash command registration, and health server
- `games/`: Complete game implementations (blackjack, baccarat, craps, slots, roulette, etc.)
- `utils/`: Core utilities (database, caching, achievements, bonuses, embeds, card handling)
- `models/`: Data structures for users, achievements, and game state

**Backup Reference (`_backup/` directory):**
- Historical reference implementation from migration
- Used for comparing implementations during development

### Key Components

1. **Discord Integration**: Uses `discordgo v0.29.0` for slash commands, component interactions, and embeds
2. **Database Layer**: PostgreSQL with `pgx v5.7.1` driver for persistent user data and game state
3. **Caching System**: In-memory cache with TTL for performance optimization
4. **User Management**: Comprehensive system with chips, XP, prestige levels, achievements, and premium features
5. **Game Engine**: Modular design where each game implements common interfaces for state management
6. **Bonus System**: Time-based bonuses (hourly, daily, weekly) with cooldown management
7. **Achievement System**: Dynamic achievement tracking with configurable criteria

## Development Commands

```bash
# Build the application
go build -o hrc-go main.go

# Run the application (requires BOT_TOKEN environment variable)
go run main.go

# Install and update dependencies
go mod tidy

# Test the application
go test ./...

# Format code
go fmt ./...

# Check for issues
go vet ./...

# Run specific game tests
go test ./games/blackjack
go test ./utils
```

## Environment Variables

Required environment variables:
- `BOT_TOKEN` (or `DISCORD_TOKEN`, `DISCORD_BOT_TOKEN`, `HRC_BOT_TOKEN`): Discord bot token
- `DATABASE_URL`: PostgreSQL connection string for persistent data
- `PORT`: HTTP health server port (defaults to 8080)

## Database Schema

The PostgreSQL database includes:
- `users`: Core user data (chips, XP, prestige, premium settings, bonus cooldowns)
- `achievements`: Achievement definitions and criteria
- `user_achievements`: User achievement progress and completion tracking

Key user fields:
- `chips`: Current chip balance
- `total_xp`, `current_xp`: Experience points system
- `prestige`: Prestige level (allows rank reset for progression)
- `premium_settings`: JSONB for premium feature toggles
- Bonus cooldown timestamps (`last_hourly`, `last_daily`, `last_weekly`)

## Game Implementation Pattern

Each game follows a consistent structure:
1. **Command Registration**: Games register slash commands in `main.go`
2. **State Management**: Active games stored in memory with automatic cleanup
3. **Component Interactions**: Button/modal handlers with prefixed custom IDs
4. **Embed Generation**: Rich Discord embeds for game display and results
5. **User Integration**: Automatic chip deduction/rewards and XP/achievement tracking

Game files include:
- Command handler functions (`HandleGameCommand`)
- Interaction handlers for buttons/modals
- Game state structures and logic
- Embed builders for game display

## Code Patterns

### User Data Access
```go
// Get user with caching
user, err := utils.GetUser(userID)

// Update user with increments
updated, err := utils.UpdateCachedUser(userID, utils.UserUpdateData{
    ChipsIncrement: winAmount,
    TotalXPIncrement: xpGained,
})
```

### Game State Management
- Games use maps to track active game sessions by user ID
- Automatic cleanup routines remove expired/finished games
- Thread-safe operations with mutex locks

### Premium Features
- Role-based premium access checking
- JSONB settings for feature toggles (XP display, win/loss tracking)
- Premium-specific UI components and data visibility

## Development Notes

- The active implementation is fully functional with all games operational
- Games use a component-based Discord interaction system with custom ID prefixes
- Achievement system supports dynamic criteria matching and progress tracking
- Bonus system includes streak multipliers and premium bonuses
- Cache system optimizes database access with configurable TTL
- Health server endpoint supports deployment monitoring