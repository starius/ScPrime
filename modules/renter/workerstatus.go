package renter

import (
	"time"

	"gitlab.com/scpcorp/ScPrime/modules"
)

// callStatus returns the status of the worker.
func (w *worker) callStatus() modules.WorkerStatus {
	w.downloadMu.Lock()
	downloadOnCoolDown := w.onDownloadCooldown()
	downloadTerminated := w.downloadTerminated
	downloadQueueSize := len(w.downloadChunks)
	var downloadCoolDownErr string
	if w.downloadRecentFailureErr != nil {
		downloadCoolDownErr = w.downloadRecentFailureErr.Error()
	}
	downloadCoolDownTime := w.downloadRecentFailure.Add(downloadFailureCooldown).Sub(time.Now())
	w.downloadMu.Unlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	uploadOnCoolDown, uploadCoolDownTime := w.onUploadCooldown()
	var uploadCoolDownErr string
	if w.uploadRecentFailureErr != nil {
		uploadCoolDownErr = w.uploadRecentFailureErr.Error()
	}

	// Update the worker cache before returning a status.
	w.staticTryUpdateCache()
	cache := w.staticCache()
	return modules.WorkerStatus{
		// Contract Information
		ContractID:      cache.staticContractID,
		ContractUtility: cache.staticContractUtility,
		HostPubKey:      w.staticHostPubKey,

		// Download information
		DownloadCoolDownError: downloadCoolDownErr,
		DownloadCoolDownTime:  downloadCoolDownTime,
		DownloadOnCoolDown:    downloadOnCoolDown,
		DownloadQueueSize:     downloadQueueSize,
		DownloadTerminated:    downloadTerminated,

		// Upload information
		UploadCoolDownError: uploadCoolDownErr,
		UploadCoolDownTime:  uploadCoolDownTime,
		UploadOnCoolDown:    uploadOnCoolDown,
		UploadQueueSize:     len(w.unprocessedChunks),
		UploadTerminated:    w.uploadTerminated,

		// Job Queues
		BackupJobQueueSize:       w.staticFetchBackupsJobQueue.managedLen(),
		DownloadRootJobQueueSize: w.staticJobQueueDownloadByRoot.managedLen(),
	}
}

// callReadJobStatus returns the status of the read job queue
func (w *worker) callReadJobStatus() modules.WorkerReadJobsStatus {
	jrq := w.staticJobReadQueue
	status := jrq.callStatus()

	var recentErrString string
	if status.recentErr != nil {
		recentErrString = status.recentErr.Error()
	}

	avgJobTimeInMs := func(l uint64) uint64 {
		if d := jrq.callAverageJobTime(l); d > 0 {
			return uint64(d.Milliseconds())
		}
		return 0
	}

	return modules.WorkerReadJobsStatus{
		AvgJobTime64k:       avgJobTimeInMs(1 << 16),
		AvgJobTime1m:        avgJobTimeInMs(1 << 20),
		AvgJobTime4m:        avgJobTimeInMs(1 << 22),
		ConsecutiveFailures: status.consecutiveFailures,
		JobQueueSize:        status.size,
		RecentErr:           recentErrString,
		RecentErrTime:       status.recentErrTime,
	}
}

// callHasSectorJobStatus returns the status of the has sector job queue
func (w *worker) callHasSectorJobStatus() modules.WorkerHasSectorJobsStatus {
	hsq := w.staticJobHasSectorQueue
	status := hsq.callStatus()

	var recentErrStr string
	if status.recentErr != nil {
		recentErrStr = status.recentErr.Error()
	}

	var avgJobTimeInMs uint64 = 0
	if d := hsq.callAverageJobTime(); d > 0 {
		avgJobTimeInMs = uint64(d.Milliseconds())
	}

	return modules.WorkerHasSectorJobsStatus{
		AvgJobTime:          avgJobTimeInMs,
		ConsecutiveFailures: status.consecutiveFailures,
		JobQueueSize:        status.size,
		RecentErr:           recentErrStr,
		RecentErrTime:       status.recentErrTime,
	}
}
