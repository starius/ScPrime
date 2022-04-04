package gui

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

var (
	// The headless flag is a way to identify if the wallet was launched from spd or scp-ui.
	headless int32 = 0
	// The heartbeat variable is a way to track when the last time the UpdateHeartbeat functionwas
	// called. This allows the wallet to automatically shutdown if the heartbeat gets too old.
	heartbeat = time.Now()
)

// Gui is a struct that tracks global settings
type Gui struct {
	apiAddr  string
	status   string
	sessions []*Session
}

// Session is a struct that tracks session settings
type Session struct {
	id            string
	collapseMenu  bool
	txHistoryPage int
	cachedPage    string
}

// New creates a new Gui.
func New(apiAddr string) *Gui {
	gui := &Gui{}
	gui.apiAddr = apiAddr
	gui.status = ""
	return gui
}

// AddHead adds a head to the GUI.
func AddHead() {
	atomic.AddInt32(&headless, 1)
}

// Headless returns true when the GUI is headless.
func Headless() bool {
	return atomic.LoadInt32(&headless) == headless
}

// UpdateHeartbeat updates and returns the heartbeat time.
func UpdateHeartbeat() time.Time {
	heartbeat = time.Now()
	return heartbeat
}

// Heartbeat returns the heartbeat time.
func Heartbeat() time.Time {
	return heartbeat
}

// Status returns the GUI's status.
func (gui *Gui) Status() string {
	return gui.status
}

// SetStatus sets the GUI's status.
func (gui *Gui) SetStatus(status string) {
	gui.status = status
}

// AddSessionId adds a new session ID to memory.
func (gui *Gui) AddSessionId() string {
	b := make([]byte, 16) //32 characters long
	rand.Read(b)
	session := &Session{}
	session.id = hex.EncodeToString(b)
	session.collapseMenu = true
	session.txHistoryPage = 1
	session.cachedPage = ""
	gui.sessions = append(gui.sessions, session)
	return session.id
}

// SessionIdExists returns true when the supplied session ID exists in memory.
func (gui *Gui) SessionIdExists(sessionId string) bool {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			return true
		}
	}
	return false
}

// CollapseMenu sets the menu state to collapsed and returns true
func (gui *Gui) CollapseMenu(sessionId string) bool {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			session.collapseMenu = true
		}
	}
	return true
}

// ExpandMenu sets the menu state to expanded and returns true
func (gui *Gui) ExpandMenu(sessionId string) bool {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			session.collapseMenu = false
		}
	}
	return true
}

// MenuIsCollapsed returns true when the menu state is collapsed
func (gui *Gui) MenuIsCollapsed(sessionId string) bool {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			return session.collapseMenu
		}
	}
	// default to the menu being expanded just in case
	return false
}

// SetTxHistoryPage sets the session's transaction history page and returns true.
func (gui *Gui) SetTxHistoryPage(txHistoryPage int, sessionId string) bool {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			session.txHistoryPage = txHistoryPage
		}
	}
	return true
}

// GetTxHistoryPage returns the session's transaction history page or -1 when no session is found.
func (gui *Gui) GetTxHistoryPage(sessionId string) int {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			return session.txHistoryPage
		}
	}
	return -1
}

// CachedPage caches the page without the menu and returns true.
func (gui *Gui) CachedPage(cachedPage string, sessionId string) bool {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			session.cachedPage = cachedPage
		}
	}
	return true
}

// GetCachedPage returns the session's cached page.
func (gui *Gui) GetCachedPage(sessionId string) string {
	for _, session := range gui.sessions {
		if session.id == sessionId {
			return session.cachedPage
		}
	}
	return ""
}

// Close will shut down the GUI, giving the module enough time to
// run any required closing routines.
func (gui *Gui) Close() error {
	return nil
}
