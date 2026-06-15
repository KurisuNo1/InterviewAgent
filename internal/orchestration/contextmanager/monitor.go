package contextmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ContextMonitor records and aggregates context usage across LLM calls,
// supporting both per-session and global aggregation.
type ContextMonitor interface {
	RecordUsage(sessionID, callSite string, usage ContextUsage)
	Snapshot() MonitorSnapshot
	SnapshotSession(sessionID string) MonitorSnapshot
}

// MonitorSnapshot is a point-in-time aggregate of context usage.
type MonitorSnapshot struct {
	TotalCalls      int       `json:"total_calls"`
	AvgUsagePercent float64   `json:"avg_usage_percent"`
	MaxUsagePercent float64   `json:"max_usage_percent"`
	WarningCount    int       `json:"warning_count"`
	CriticalCount   int       `json:"critical_count"`
	LastWarningTime time.Time `json:"last_warning_time,omitempty"`
}

// sessionStats holds per-session aggregated metrics.
type sessionStats struct {
	totalCalls      int
	totalPercent    float64
	maxPercent      float64
	warningCount    int
	criticalCount   int
	lastWarningTime time.Time
	lastUpdated     time.Time
}

// persistData is the JSON-serializable form of DefaultMonitor state.
type persistData struct {
	Sessions map[string]persistSession `json:"sessions"`
}

type persistSession struct {
	TotalCalls      int     `json:"total_calls"`
	TotalPercent    float64 `json:"total_percent"`
	MaxPercent      float64 `json:"max_percent"`
	WarningCount    int     `json:"warning_count"`
	CriticalCount   int     `json:"critical_count"`
	LastWarningTime int64   `json:"last_warning_unix,omitempty"`
	LastUpdated     int64   `json:"last_updated_unix"`
}

// DefaultMonitor implements ContextMonitor with in-memory per-session aggregation,
// periodic file persistence, and log output.
type DefaultMonitor struct {
	mu          sync.RWMutex
	sessions    map[string]*sessionStats
	filePath    string
	persistTimer *time.Timer
	persistMu   sync.Mutex
}

const persistDebounce = 5 * time.Second
const sessionCleanupTTL = 24 * time.Hour

// NewDefaultMonitor creates a new DefaultMonitor.
// If filePath is non-empty, state is loaded from that file and persisted on changes.
func NewDefaultMonitor(filePath string) *DefaultMonitor {
	m := &DefaultMonitor{
		sessions: make(map[string]*sessionStats),
		filePath: filePath,
	}
	if filePath != "" {
		m.loadFromFile()
	}
	return m
}

func (m *DefaultMonitor) getOrCreate(key string) *sessionStats {
	s, ok := m.sessions[key]
	if !ok {
		s = &sessionStats{}
		m.sessions[key] = s
	}
	return s
}

// RecordUsage records a single call's usage. sessionID can be "" for global-only tracking.
func (m *DefaultMonitor) RecordUsage(sessionID, callSite string, usage ContextUsage) {
	m.mu.Lock()

	pct := usage.UsagePercent()
	now := time.Now()

	// Update per-session stats
	if sessionID != "" {
		ss := m.getOrCreate(sessionID)
		ss.totalCalls++
		ss.totalPercent += pct
		if pct > ss.maxPercent {
			ss.maxPercent = pct
		}
		ss.lastUpdated = now
		if usage.IsCritical() {
			ss.criticalCount++
			ss.lastWarningTime = now
		} else if usage.IsWarning(0.80) {
			ss.warningCount++
			ss.lastWarningTime = now
		}
	}

	// Update global stats
	gs := m.getOrCreate("")
	gs.totalCalls++
	gs.totalPercent += pct
	if pct > gs.maxPercent {
		gs.maxPercent = pct
	}
	gs.lastUpdated = now

	// Logging
	if usage.IsCritical() {
		gs.criticalCount++
		gs.lastWarningTime = now
		log.Printf("[ContextMonitor] CRITICAL session=%s call_site=%s %s", sessionID, callSite, formatUsageLog(usage))
	} else if usage.IsWarning(0.80) {
		gs.warningCount++
		gs.lastWarningTime = now
		log.Printf("[ContextMonitor] WARNING session=%s call_site=%s %s", sessionID, callSite, formatUsageLog(usage))
	} else {
		log.Printf("[ContextMonitor] session=%s call_site=%s total=%d limit=%d percent=%.1f%%",
			sessionID, callSite, usage.TotalTokens, usage.WindowLimit, pct)
	}

	m.mu.Unlock()

	// Debounced persist (outside lock)
	m.schedulePersist()
}

func (m *DefaultMonitor) schedulePersist() {
	if m.filePath == "" {
		return
	}
	m.persistMu.Lock()
	defer m.persistMu.Unlock()
	if m.persistTimer != nil {
		m.persistTimer.Stop()
	}
	m.persistTimer = time.AfterFunc(persistDebounce, func() {
		m.persistToFile()
	})
}

// persistToFile writes current state to the JSON file. Caller must not hold mu.
func (m *DefaultMonitor) persistToFile() {
	m.mu.RLock()
	pd := persistData{Sessions: make(map[string]persistSession, len(m.sessions))}
	for k, v := range m.sessions {
		ps := persistSession{
			TotalCalls:    v.totalCalls,
			TotalPercent:  v.totalPercent,
			MaxPercent:    v.maxPercent,
			WarningCount:  v.warningCount,
			CriticalCount: v.criticalCount,
			LastUpdated:   v.lastUpdated.Unix(),
		}
		if !v.lastWarningTime.IsZero() {
			ps.LastWarningTime = v.lastWarningTime.Unix()
		}
		pd.Sessions[k] = ps
	}
	m.mu.RUnlock()

	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[ContextMonitor] failed to create persist dir %s: %v", dir, err)
		return
	}

	data, err := json.Marshal(pd)
	if err != nil {
		log.Printf("[ContextMonitor] failed to marshal state: %v", err)
		return
	}

	tmpPath := m.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		log.Printf("[ContextMonitor] failed to write persist file: %v", err)
		return
	}
	if err := os.Rename(tmpPath, m.filePath); err != nil {
		log.Printf("[ContextMonitor] failed to rename persist file: %v", err)
		return
	}
}

// loadFromFile loads state from the JSON file on startup.
func (m *DefaultMonitor) loadFromFile() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[ContextMonitor] failed to read persist file: %v", err)
		}
		return
	}

	var pd persistData
	if err := json.Unmarshal(data, &pd); err != nil {
		log.Printf("[ContextMonitor] failed to unmarshal persist file: %v", err)
		return
	}

	now := time.Now()
	cutoff := now.Add(-sessionCleanupTTL)
	loaded := 0
	skipped := 0

	for k, v := range pd.Sessions {
		lastUpdated := time.Unix(v.LastUpdated, 0)
		if lastUpdated.Before(cutoff) {
			skipped++
			continue
		}
		ss := &sessionStats{
			totalCalls:    v.TotalCalls,
			totalPercent:  v.TotalPercent,
			maxPercent:    v.MaxPercent,
			warningCount:  v.WarningCount,
			criticalCount: v.CriticalCount,
			lastUpdated:   lastUpdated,
		}
		if v.LastWarningTime > 0 {
			ss.lastWarningTime = time.Unix(v.LastWarningTime, 0)
		}
		m.sessions[k] = ss
		loaded++
	}

	log.Printf("[ContextMonitor] loaded %d sessions from %s (skipped %d expired)", loaded, m.filePath, skipped)
}

func snapshotFromStats(s *sessionStats) MonitorSnapshot {
	if s == nil {
		return MonitorSnapshot{}
	}
	avg := 0.0
	if s.totalCalls > 0 {
		avg = s.totalPercent / float64(s.totalCalls)
	}
	return MonitorSnapshot{
		TotalCalls:      s.totalCalls,
		AvgUsagePercent: avg,
		MaxUsagePercent: s.maxPercent,
		WarningCount:    s.warningCount,
		CriticalCount:   s.criticalCount,
		LastWarningTime: s.lastWarningTime,
	}
}

// Snapshot returns global aggregated usage statistics.
func (m *DefaultMonitor) Snapshot() MonitorSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return snapshotFromStats(m.sessions[""])
}

// SnapshotSession returns usage statistics for a specific session.
func (m *DefaultMonitor) SnapshotSession(sessionID string) MonitorSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return snapshotFromStats(m.sessions[sessionID])
}

func formatUsageLog(u ContextUsage) string {
	return logEntry(u).String()
}

type logEntry ContextUsage

func (e logEntry) String() string {
	u := ContextUsage(e)
	return fmt.Sprintf("total=%d limit=%d percent=%.1f%% system=%d history=%d rag=%d tool_result=%d input=%d",
		e.TotalTokens, e.WindowLimit, u.UsagePercent(),
		e.SystemPromptTokens, e.HistoryTokens, e.RAGDocTokens, e.ToolResultTokens, e.InputTokens)
}
