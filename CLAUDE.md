# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based Discord casino bot (`hrc-go`) that provides various gambling games. The project is structured with a minimal active implementation in the root directory and a comprehensive backup of game logic in the `_backup` folder.

### Architecture

The project follows a modular architecture:

**Active Code (Root Directory):**
- `main.go`: Entry point with basic Discord bot connection and health server for deployment
- `go.mod`: Module definition with `github.com/bwmarrin/discordgo v0.29.0` dependency

**Backup Code (`_backup/` directory):**
- `models/`: Data structures for users, achievements, and game state
- `utils/`: Core utilities including database operations, card handling, and embed creation
- `games/`: Individual game implementations (blackjack, baccarat, craps, etc.)

### Key Components

1. **Database Layer**: PostgreSQL with pgx driver for user data, achievements, and game state
2. **Discord Integration**: Uses discordgo library for slash commands and component interactions
3. **Game Engine**: Modular game system with base game functionality and specific implementations
4. **User Management**: Comprehensive user system with chips, XP, rankings, and premium features

## Development Commands

Since Go is not currently installed in the environment, these are the standard Go commands you would use:

```bash
# Build the application
go build -o hrc-go main.go

# Run the application
go run main.go

# Install dependencies
go mod tidy
go mod download

# Test the application
go test ./...

# Format code
go fmt ./...

# Vet code for issues
go vet ./...
```

## Environment Variables

The application requires these environment variables:
- `BOT_TOKEN`: Discord bot token (optional - bot runs without Discord if not provided)
- `DATABASE_URL`: PostgreSQL connection string (used by backup implementation)
- `PORT`: HTTP server port (defaults to 8080)

## Development Notes

- The current active implementation is minimal - most game logic is in the `_backup` directory
- The backup contains a full casino bot implementation with multiple games
- Database schema includes users, achievements, and user_achievements tables
- Games use a component-based Discord interaction system
- The project includes a health server for deployment platform integration

## Game Implementation Pattern

Each game in the backup follows this pattern:
1. Game state management with active games map
2. Discord slash command handlers
3. Component interaction handlers for game actions
4. Embed generation for game display
5. Integration with user data and achievement systems