package main

import (
	"time"
)

const (
	// OutputRefreshRate is the rate at which spc will update something like a
	// progress meter when displaying a continuous action like a download.
	OutputRefreshRate = 250 * time.Millisecond

	// RenterDownloadTimeout is the amount of time that needs to elapse before
	// the download command gives up on finding a download in the download list.
	RenterDownloadTimeout = time.Minute

	// SpeedEstimationWindow is the size of the window which we use to
	// determine download speeds.
	SpeedEstimationWindow = 60 * time.Second

	// moduleNotReadyStatus is the error message displayed when an API call error
	// suggests that a modules is not yet ready for usage.
	moduleNotReadyStatus = "Module not loaded or still starting up"
)
