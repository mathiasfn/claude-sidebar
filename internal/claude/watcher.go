package claude

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type SessionState struct {
	Session  Session
	Data     *SessionData
	Offset   int64
	Alive    bool
	mu       sync.Mutex
}

type Watcher struct {
	cwd          string
	sessions     map[string]*SessionState // sessionID -> state
	recentDead   []*SessionState          // most recent dead sessions
	mu           sync.RWMutex
	fsWatcher    *fsnotify.Watcher
	onChange     func() // callback when data changes
	stopCh       chan struct{}
}

func NewWatcher(cwd string, onChange func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		cwd:       cwd,
		sessions:  make(map[string]*SessionState),
		fsWatcher: fsw,
		onChange:  onChange,
		stopCh:    make(chan struct{}),
	}

	return w, nil
}

func (w *Watcher) Start() {
	// Initial scan
	w.refreshSessions()

	// Watch for fsnotify events
	go w.watchFiles()

	// Periodic refresh for sessions and git
	go w.periodicRefresh()
}

func (w *Watcher) Stop() {
	close(w.stopCh)
	w.fsWatcher.Close()
}

func (w *Watcher) Sessions() []*SessionState {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var result []*SessionState
	for _, s := range w.sessions {
		if s.Alive {
			result = append(result, s)
		}
	}
	return result
}

func (w *Watcher) RecentDead() []*SessionState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.recentDead
}

func (w *Watcher) TotalTokens() Usage {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var total Usage
	for _, s := range w.sessions {
		if s.Data != nil {
			total.InputTokens += s.Data.Tokens.InputTokens
			total.CacheCreationInputTokens += s.Data.Tokens.CacheCreationInputTokens
			total.CacheReadInputTokens += s.Data.Tokens.CacheReadInputTokens
			total.OutputTokens += s.Data.Tokens.OutputTokens
		}
	}
	return total
}

func (w *Watcher) refreshSessions() {
	alive, dead, err := DiscoverSessionsWithRecent(w.cwd, 2)
	if err != nil {
		return
	}

	// Parse dead sessions
	var deadStates []*SessionState
	for _, sess := range dead {
		state := &SessionState{Session: sess, Alive: false}
		if data, err := ParseJSONL(sess.JSONLPath()); err == nil {
			state.Data = data
		}
		deadStates = append(deadStates, state)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.recentDead = deadStates

	// Mark all existing as potentially dead
	for _, s := range w.sessions {
		s.Alive = false
	}

	for _, sess := range alive {
		existing, ok := w.sessions[sess.SessionID]
		if ok {
			existing.Session = sess
			existing.Alive = true
			// Incremental parse
			w.updateSessionData(existing)
		} else {
			state := &SessionState{
				Session: sess,
				Alive:   true,
			}
			// Full parse
			jsonlPath := sess.JSONLPath()
			if data, err := ParseJSONL(jsonlPath); err == nil {
				state.Data = data
				// Get file size as offset for incremental updates
				if fi, err := os.Stat(jsonlPath); err == nil {
					state.Offset = fi.Size()
				}
			} else {
				state.Data = &SessionData{}
			}
			w.sessions[sess.SessionID] = state

			// Watch the JSONL file
			w.watchJSONL(sess)
		}
	}
}

func (w *Watcher) watchJSONL(sess Session) {
	jsonlPath := sess.JSONLPath()
	dir := filepath.Dir(jsonlPath)
	// Watch the directory (fsnotify can't watch non-existent files)
	w.fsWatcher.Add(dir)
}

func (w *Watcher) updateSessionData(state *SessionState) {
	state.mu.Lock()
	defer state.mu.Unlock()

	jsonlPath := state.Session.JSONLPath()
	newData, newOffset, err := ParseJSONLFrom(jsonlPath, state.Offset)
	if err != nil || newData == nil {
		return
	}

	if state.Data == nil {
		state.Data = newData
	} else {
		state.Data.Tokens.InputTokens += newData.Tokens.InputTokens
		state.Data.Tokens.CacheCreationInputTokens += newData.Tokens.CacheCreationInputTokens
		state.Data.Tokens.CacheReadInputTokens += newData.Tokens.CacheReadInputTokens
		state.Data.Tokens.OutputTokens += newData.Tokens.OutputTokens
		state.Data.Turns += newData.Turns
		if newData.Model != "" {
			state.Data.Model = newData.Model
		}
		if newData.Branch != "" {
			state.Data.Branch = newData.Branch
		}
		if newData.LastUpdate.After(state.Data.LastUpdate) {
			state.Data.LastUpdate = newData.LastUpdate
		}
		// Always update LastUsage and Speed to latest
		if newData.LastUsage.ContextTokens() > 0 {
			state.Data.LastUsage = newData.LastUsage
		}
		if newData.Speed != "" {
			state.Data.Speed = newData.Speed
		}
	}
	state.Offset = newOffset
}

func (w *Watcher) watchFiles() {
	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) && strings.HasSuffix(event.Name, ".jsonl") {
				// Find which session this belongs to
				base := strings.TrimSuffix(filepath.Base(event.Name), ".jsonl")
				w.mu.RLock()
				state, ok := w.sessions[base]
				w.mu.RUnlock()
				if ok {
					w.mu.Lock()
					w.updateSessionData(state)
					w.mu.Unlock()
					if w.onChange != nil {
						w.onChange()
					}
				}
			}
		case _, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) periodicRefresh() {
	sessionTicker := time.NewTicker(10 * time.Second)
	defer sessionTicker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-sessionTicker.C:
			w.refreshSessions()
			if w.onChange != nil {
				w.onChange()
			}
		}
	}
}
