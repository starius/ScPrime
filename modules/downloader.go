package modules

type (
	// The Downloader module that can be used to grab external resources.
	Downloader interface {
		// Close will shut down the downloader, giving the module enough time to
		// run any required closing routines.
		Close() error
	}
)
