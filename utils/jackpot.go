package utils

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// JackpotType represents different types of jackpots
type JackpotType string

const (
	JackpotSlots   JackpotType = "slots"
	JackpotGeneral JackpotType = "general"
	JackpotSpecial JackpotType = "special"
)

// Jackpot represents a progressive jackpot
type Jackpot struct {
	ID               int         `json:"id"`
	Type             JackpotType `json:"type"`
	Amount           int64       `json:"amount"`
	SeedAmount       int64       `json:"seed_amount"`
	ContributionRate float64     `json:"contribution_rate"`
	LastWinner       *int64      `json:"last_winner,omitempty"`
	LastWinAmount    *int64      `json:"last_win_amount,omitempty"`
	LastWinTime      *time.Time  `json:"last_win_time,omitempty"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// JackpotManager manages progressive jackpots
type JackpotManager struct {
	jackpots map[JackpotType]*Jackpot
	mutex    sync.RWMutex
}

// Global jackpot manager
var JackpotMgr *JackpotManager

// Jackpot configuration constants
const (
	DefaultSlotsJackpot     = 2500    // 100k starting jackpot for slots
	DefaultGeneralJackpot   = 2500    // 50k starting jackpot for general games
	SlotsContributionRate   = 0.10    // 10% of each slots bet goes to jackpot
	GeneralContributionRate = 0.005   // 0.5% of other game bets goes to jackpot
	MinimumJackpotAmount    = 10000   // Minimum jackpot before reset
	JackpotWinThreshold     = 1000000 // Jackpot win probability threshold
)

// InitializeJackpotManager sets up the jackpot system
func InitializeJackpotManager() error {
	log.Println("[jackpot] InitializeJackpotManager start")
	JackpotMgr = &JackpotManager{jackpots: make(map[JackpotType]*Jackpot)}
	if err := JackpotMgr.createJackpotsTable(); err != nil {
		return fmt.Errorf("failed to create jackpots table: %w", err)
	}
	if err := JackpotMgr.loadJackpots(); err != nil {
		log.Printf("[jackpot] loadJackpots error: %v", err)
		return err
	}
	log.Println("[jackpot] InitializeJackpotManager complete")
	return nil
}

// createJackpotsTable creates the jackpots table if it doesn't exist
func (jm *JackpotManager) createJackpotsTable() error {
	if DB == nil {
		return nil // Skip in offline mode
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	query := `
		CREATE TABLE IF NOT EXISTS jackpots (
			id SERIAL PRIMARY KEY,
			type VARCHAR(50) NOT NULL UNIQUE,
			amount BIGINT NOT NULL DEFAULT 0,
			seed_amount BIGINT NOT NULL DEFAULT 0,
			contribution_rate DECIMAL(5,4) NOT NULL DEFAULT 0.01,
			last_winner BIGINT,
			last_win_amount BIGINT,
			last_win_time TIMESTAMP WITH TIME ZONE,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)`

	log.Println("[jackpot] Creating jackpots table (timeout 3s)...")
	_, err := DB.Exec(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout creating jackpots table: %w", err)
		}
		return fmt.Errorf("failed to create jackpots table: %w", err)
	}

	log.Println("jackpots table created/verified successfully")
	return nil
}

// loadJackpots loads existing jackpots from database or creates defaults
func (jm *JackpotManager) loadJackpots() error {
	// Initialize defaults into memory first
	jm.initializeDefaultJackpots()

	if DB == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	query := `
		SELECT id, type, amount, seed_amount, contribution_rate,
			   last_winner, last_win_amount, last_win_time, updated_at
		FROM jackpots`

	log.Println("[jackpot] Loading jackpots from database (timeout 3s)...")
	rows, err := DB.Query(ctx, query)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout querying jackpots: %w", err)
		}
		if err == pgx.ErrNoRows { // none exist yet
			return jm.saveDefaultJackpots()
		}
		return fmt.Errorf("failed to query jackpots: %w", err)
	}
	defer rows.Close()

	loaded := 0
	temp := make(map[JackpotType]*Jackpot)
	for rows.Next() {
		var jackpot Jackpot
		var jackpotType string
		if err := rows.Scan(
			&jackpot.ID,
			&jackpotType,
			&jackpot.Amount,
			&jackpot.SeedAmount,
			&jackpot.ContributionRate,
			&jackpot.LastWinner,
			&jackpot.LastWinAmount,
			&jackpot.LastWinTime,
			&jackpot.UpdatedAt,
		); err != nil {
			log.Printf("Failed to scan jackpot: %v", err)
			continue
		}
		jackpot.Type = JackpotType(jackpotType)
		temp[jackpot.Type] = &jackpot
		loaded++
	}

	if loaded == 0 { // nothing persisted yet; write defaults
		log.Println("[jackpot] No jackpots found in DB; saving defaults (no rows loaded)")
		if err := jm.saveDefaultJackpots(); err != nil {
			return err
		}
		log.Println("[jackpot] Default jackpots saved")
		jm.logRowCount()
		return nil
	}

	// Swap in loaded jackpots
	jm.mutex.Lock()
	for k, v := range temp {
		jm.jackpots[k] = v
	}
	jm.mutex.Unlock()

	log.Printf("Loaded %d jackpots from database", loaded)
	jm.logRowCount()
	return nil
}

// initializeDefaultJackpots sets up default jackpot configurations
func (jm *JackpotManager) initializeDefaultJackpots() {
	now := time.Now()

	defaultJackpots := []*Jackpot{
		{
			Type:             JackpotSlots,
			Amount:           DefaultSlotsJackpot,
			SeedAmount:       DefaultSlotsJackpot,
			ContributionRate: SlotsContributionRate,
			UpdatedAt:        now,
		},
		{
			Type:             JackpotGeneral,
			Amount:           DefaultGeneralJackpot,
			SeedAmount:       DefaultGeneralJackpot,
			ContributionRate: GeneralContributionRate,
			UpdatedAt:        now,
		},
	}

	jm.mutex.Lock()
	defer jm.mutex.Unlock()

	for _, jackpot := range defaultJackpots {
		jm.jackpots[jackpot.Type] = jackpot
	}

	log.Printf("Initialized %d default jackpots", len(defaultJackpots))
}

// saveDefaultJackpots saves default jackpots to database
func (jm *JackpotManager) saveDefaultJackpots() error {
	log.Println("[jackpot] saveDefaultJackpots start")
	if DB == nil {
		log.Println("[jackpot] DB nil; skipping saveDefaultJackpots (running in memory-only mode)")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use write lock because we mutate jackpot.ID fields
	jm.mutex.Lock()
	defer jm.mutex.Unlock()

	inserted := 0
	for _, jackpot := range jm.jackpots {
		log.Printf("[jackpot] upserting jackpot type=%s seed=%d amount=%d rate=%.4f", jackpot.Type, jackpot.SeedAmount, jackpot.Amount, jackpot.ContributionRate)
		if ctx.Err() != nil {
			return fmt.Errorf("context expired before insert: %w", ctx.Err())
		}
		query := `
			INSERT INTO jackpots (type, amount, seed_amount, contribution_rate, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (type) DO UPDATE SET
				amount = EXCLUDED.amount,
				seed_amount = EXCLUDED.seed_amount,
				contribution_rate = EXCLUDED.contribution_rate,
				updated_at = EXCLUDED.updated_at
			RETURNING id, amount`
		var id int
		var persistedAmount int64
		err := DB.QueryRow(ctx, query,
			string(jackpot.Type),
			jackpot.Amount,
			jackpot.SeedAmount,
			jackpot.ContributionRate,
			jackpot.UpdatedAt,
		).Scan(&id, &persistedAmount)
		if err != nil {
			return fmt.Errorf("failed to save jackpot %s: %w", jackpot.Type, err)
		}
		jackpot.ID = id
		jackpot.Amount = persistedAmount // ensure memory matches DB
		inserted++
	}
	log.Printf("[jackpot] Saved/updated %d jackpots (expected=%d)", inserted, len(jm.jackpots))
	jm.logRowCount()
	// Extra verification pass
	jm.ensurePersisted()
	return nil
}

// ensurePersisted re-selects jackpots to verify they exist and logs discrepancies.
func (jm *JackpotManager) ensurePersisted() {
	if DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rows, err := DB.Query(ctx, `SELECT type, amount FROM jackpots`)
	if err != nil {
		log.Printf("[jackpot] ensurePersisted query error: %v", err)
		return
	}
	defer rows.Close()
	found := map[string]int64{}
	for rows.Next() {
		var t string
		var amt int64
		if err := rows.Scan(&t, &amt); err != nil {
			log.Printf("[jackpot] ensurePersisted scan err: %v", err)
			continue
		}
		found[t] = amt
	}
	for jt := range jm.jackpots {
		if _, ok := found[string(jt)]; !ok {
			log.Printf("[jackpot] ensurePersisted MISSING jackpot type=%s in DB after save", jt)
		}
	}
	log.Printf("[jackpot] ensurePersisted rows=%d detailed=%v", len(found), found)
}

// logRowCount prints current jackpot row count for diagnostics
func (jm *JackpotManager) logRowCount() {
	if DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var cnt int
	if err := DB.QueryRow(ctx, "SELECT COUNT(*) FROM jackpots").Scan(&cnt); err != nil {
		log.Printf("[jackpot] row count query error: %v", err)
		return
	}
	log.Printf("[jackpot] jackpots table row count=%d", cnt)
}

// ContributeToJackpot adds a contribution to the specified jackpot
func (jm *JackpotManager) ContributeToJackpot(jackpotType JackpotType, betAmount int64) (int64, error) {
	// First: update in-memory state under lock quickly
	jm.mutex.Lock()
	jackpot, exists := jm.jackpots[jackpotType]
	if !exists {
		jm.mutex.Unlock()
		return 0, fmt.Errorf("jackpot type %s not found", jackpotType)
	}
	contribution := int64(float64(betAmount) * jackpot.ContributionRate)
	if contribution <= 0 {
		jm.mutex.Unlock()
		return 0, nil
	}
	jackpot.Amount += contribution
	jackpot.UpdatedAt = time.Now()
	// Copy values needed for DB write
	snapshot := *jackpot
	jm.mutex.Unlock()

	// Second: perform DB write outside lock
	if DB != nil {
		if err := jm.updateJackpotInDB(&snapshot); err != nil {
			log.Printf("[jackpot] ContributeToJackpot db update err type=%s: %v", jackpotType, err)
		}
	}
	log.Printf("[jackpot] Added %d chips to %s jackpot (bet: %d, rate: %.4f). New total: %d",
		contribution, jackpotType, betAmount, snapshot.ContributionRate, snapshot.Amount)
	return contribution, nil
}

// updateJackpotInDB updates a jackpot in the database
func (jm *JackpotManager) updateJackpotInDB(jackpot *Jackpot) error {
	if DB == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	query := `
		UPDATE jackpots 
		SET amount = $1, last_winner = $2, last_win_amount = $3, 
			last_win_time = $4, updated_at = $5
		WHERE type = $6`
	ct, err := DB.Exec(ctx, query,
		jackpot.Amount,
		jackpot.LastWinner,
		jackpot.LastWinAmount,
		jackpot.LastWinTime,
		jackpot.UpdatedAt,
		string(jackpot.Type),
	)
	if err != nil {
		return err
	}
	rows := ct.RowsAffected()
	if rows == 0 {
		log.Printf("[jackpot] updateJackpotInDB affected 0 rows for type=%s (will attempt insert)", jackpot.Type)
		// attempt insert (race where row missing)
		ins := `INSERT INTO jackpots (type, amount, seed_amount, contribution_rate, updated_at) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (type) DO NOTHING`
		if _, ierr := DB.Exec(ctx, ins, string(jackpot.Type), jackpot.Amount, jackpot.SeedAmount, jackpot.ContributionRate, jackpot.UpdatedAt); ierr != nil {
			log.Printf("[jackpot] fallback insert failed for type=%s err=%v", jackpot.Type, ierr)
		} else {
			log.Printf("[jackpot] fallback insert success for missing type=%s", jackpot.Type)
		}
	}
	return nil
}

// TryWinJackpot attempts to win the jackpot based on probability
func (jm *JackpotManager) TryWinJackpot(jackpotType JackpotType, userID int64, betAmount int64, probability float64) (bool, int64, error) {
	jm.mutex.Lock()
	defer jm.mutex.Unlock()

	jackpot, exists := jm.jackpots[jackpotType]
	if !exists {
		return false, 0, fmt.Errorf("jackpot type %s not found", jackpotType)
	}

	// Check minimum jackpot amount
	if jackpot.Amount < MinimumJackpotAmount {
		return false, 0, nil
	}

	// Calculate win probability (higher bets = slightly better odds)
	betMultiplier := 1.0 + (float64(betAmount)/float64(jackpot.Amount))*0.1
	adjustedProbability := probability * betMultiplier

	// Simulate random chance (in production, you'd use proper RNG)
	// For now, use simple probability check
	// adjustedProbability * 1000000 would be threshold if using integer RNG; kept implicit

	// Simplified win condition - in production you'd use crypto/rand
	timestamp := time.Now().UnixNano()
	randomValue := float64(timestamp%1000000) / 1000000.0

	if randomValue <= adjustedProbability {
		// JACKPOT WON!
		winAmount := jackpot.Amount

		// Record the win
		now := time.Now()
		jackpot.LastWinner = &userID
		jackpot.LastWinAmount = &winAmount
		jackpot.LastWinTime = &now

		// Reset jackpot to seed amount
		jackpot.Amount = jackpot.SeedAmount
		jackpot.UpdatedAt = now

		// Update database
		if DB != nil {
			if err := jm.updateJackpotInDB(jackpot); err != nil {
				log.Printf("Failed to update jackpot after win: %v", err)
			}
		}

		log.Printf("🎉 JACKPOT WON! User %d won %d chips from %s jackpot (bet: %d, probability: %.8f)",
			userID, winAmount, jackpotType, betAmount, adjustedProbability)

		return true, winAmount, nil
	}

	return false, 0, nil
}

// GetJackpot returns the current jackpot for a specific type
func (jm *JackpotManager) GetJackpot(jackpotType JackpotType) (*Jackpot, error) {
	jm.mutex.RLock()
	defer jm.mutex.RUnlock()

	jackpot, exists := jm.jackpots[jackpotType]
	if !exists {
		return nil, fmt.Errorf("jackpot type %s not found", jackpotType)
	}

	// Return a copy to prevent external modification
	jackpotCopy := *jackpot
	return &jackpotCopy, nil
}

// GetAllJackpots returns all current jackpots
func (jm *JackpotManager) GetAllJackpots() map[JackpotType]*Jackpot {
	jm.mutex.RLock()
	defer jm.mutex.RUnlock()

	result := make(map[JackpotType]*Jackpot)
	for jackpotType, jackpot := range jm.jackpots {
		// Return copies to prevent external modification
		jackpotCopy := *jackpot
		result[jackpotType] = &jackpotCopy
	}

	return result
}

// GetJackpotAmount returns the current amount for a specific jackpot type
func (jm *JackpotManager) GetJackpotAmount(jackpotType JackpotType) (int64, error) {
	jackpot, err := jm.GetJackpot(jackpotType)
	if err != nil {
		return 0, err
	}
	return jackpot.Amount, nil
}

// ResetJackpot resets a jackpot to its seed amount (admin function)
func (jm *JackpotManager) ResetJackpot(jackpotType JackpotType) error {
	jm.mutex.Lock()
	defer jm.mutex.Unlock()

	jackpot, exists := jm.jackpots[jackpotType]
	if !exists {
		return fmt.Errorf("jackpot type %s not found", jackpotType)
	}

	oldAmount := jackpot.Amount
	jackpot.Amount = jackpot.SeedAmount
	jackpot.LastWinner = nil
	jackpot.LastWinAmount = nil
	jackpot.LastWinTime = nil
	jackpot.UpdatedAt = time.Now()

	// Update database
	if DB != nil {
		if err := jm.updateJackpotInDB(jackpot); err != nil {
			return fmt.Errorf("failed to reset jackpot in database: %w", err)
		}
	}

	log.Printf("Reset %s jackpot from %d to %d chips", jackpotType, oldAmount, jackpot.Amount)
	return nil
}

// AddJackpotAmount manually adds amount to jackpot (admin function)
func (jm *JackpotManager) AddJackpotAmount(jackpotType JackpotType, amount int64) error {
	jm.mutex.Lock()
	jackpot, exists := jm.jackpots[jackpotType]
	if !exists {
		jm.mutex.Unlock()
		return fmt.Errorf("jackpot type %s not found", jackpotType)
	}
	jackpot.Amount += amount
	jackpot.UpdatedAt = time.Now()
	snapshot := *jackpot
	jm.mutex.Unlock()

	if DB != nil {
		if err := jm.updateJackpotInDB(&snapshot); err != nil {
			return fmt.Errorf("failed to update jackpot in database: %w", err)
		}
	}
	log.Printf("[jackpot] Added %d chips to %s jackpot. New total: %d", amount, jackpotType, snapshot.Amount)
	return nil
}

// GetJackpotStats returns statistics about all jackpots
type JackpotStats struct {
	TotalAmount    int64                 `json:"total_amount"`
	JackpotCount   int                   `json:"jackpot_count"`
	LastWinInfo    *JackpotWinInfo       `json:"last_win_info,omitempty"`
	JackpotAmounts map[JackpotType]int64 `json:"jackpot_amounts"`
}

type JackpotWinInfo struct {
	Type    JackpotType `json:"type"`
	Winner  int64       `json:"winner"`
	Amount  int64       `json:"amount"`
	WinTime time.Time   `json:"win_time"`
}

func (jm *JackpotManager) GetJackpotStats() *JackpotStats {
	jm.mutex.RLock()
	defer jm.mutex.RUnlock()

	stats := &JackpotStats{
		JackpotCount:   len(jm.jackpots),
		JackpotAmounts: make(map[JackpotType]int64),
	}

	var lastWin *JackpotWinInfo
	var lastWinTime time.Time

	for jackpotType, jackpot := range jm.jackpots {
		stats.TotalAmount += jackpot.Amount
		stats.JackpotAmounts[jackpotType] = jackpot.Amount

		// Track most recent win
		if jackpot.LastWinTime != nil && (lastWin == nil || jackpot.LastWinTime.After(lastWinTime)) {
			lastWin = &JackpotWinInfo{
				Type:    jackpotType,
				Winner:  *jackpot.LastWinner,
				Amount:  *jackpot.LastWinAmount,
				WinTime: *jackpot.LastWinTime,
			}
			lastWinTime = *jackpot.LastWinTime
		}
	}

	stats.LastWinInfo = lastWin
	return stats
}
