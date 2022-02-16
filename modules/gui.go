package modules

type (
	// The Gui renders an interactive view of the daemon for the user.
	Gui interface {
		// returns the GUI's status.
		Status() string
		// sets the GUI's status.
		SetStatus(status string)
		// returns the value of a newly added session ID
		AddSessionId() string
		// returns true when the supplied session ID exists
		SessionIdExists(sessionId string) bool
		// sets the menu state to collapsed and returns true
		CollapseMenu(sessionId string) bool
		// sets the menu state to expanded and returns true
		ExpandMenu(sessionId string) bool
		// returns true when the menu state is collapsed
		MenuIsCollapsed(sessionId string) bool
		// returns true when the session's transaction history page is set
		SetTxHistoryPage(txHistoryPage int, sessionId string) bool
		// returns the session's transaction history page
		GetTxHistoryPage(sessionId string) int
		// caches the page without the menu and returns true
		CachedPage(cachedPage string, sessionId string) bool
		// returns the session's cached page.
		GetCachedPage(sessionId string) string
		// Close will shut down the GUI, giving the module enough time to
		// run any required closing routines.
		Close() error
	}
)
