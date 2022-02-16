package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"

	"gitlab.com/scpcorp/ScPrime/build"
)

var (
	// httpServerTimeout defines the maximum amount of time before an HTTP call
	// will timeout and an error will be returned.
	httpServerTimeout = build.Select(build.Var{
		Standard: 24 * time.Hour,
		Dev:      1 * time.Hour,
		Testing:  1 * time.Minute,
	}).(time.Duration)
)

// buildHttpRoutes sets up and returns an * httprouter.Router.
// it connected the Router to the given api using the required
// parameters: requiredUserAgent and requiredPassword
func (api *API) buildHTTPRoutes() {
	router := httprouter.New()
	requiredPassword := api.requiredPassword
	requiredUserAgent := api.requiredUserAgent

	router.NotFound = http.HandlerFunc(api.UnrecognizedCallHandler)
	router.RedirectTrailingSlash = false

	// Daemon API Calls
	router.GET("/daemon/alerts", api.daemonAlertsHandlerGET)
	router.GET("/daemon/constants", api.daemonConstantsHandler)
	router.GET("/daemon/settings", api.daemonSettingsHandlerGET)
	router.POST("/daemon/settings", api.daemonSettingsHandlerPOST)
	router.GET("/daemon/stack", api.daemonStackHandlerGET)
	router.GET("/daemon/stop", RequirePassword(api.daemonStopHandler, requiredPassword))
	router.GET("/daemon/update", api.daemonUpdateHandlerGET)
	router.POST("/daemon/update", api.daemonUpdateHandlerPOST)
	router.GET("/daemon/version", api.daemonVersionHandler)

	// Consensus API Calls
	if api.cs != nil {
		router.GET("/consensus", api.consensusHandler)
		router.GET("/consensus/blocks", api.consensusBlocksHandler)
		router.GET("/consensus/subscribe/:id", api.consensusSubscribeHandler)
		router.POST("/consensus/validate/transactionset", api.consensusValidateTransactionsetHandler)
		router.GET("/consensus/blocks/:height", api.consensusBlocksHandlerSanasol)
	}

	// Explorer API Calls
	if api.explorer != nil {
		router.GET("/explorer", api.explorerHandler)
		router.GET("/explorer/blocks/:height", api.explorerBlocksHandler)
		router.GET("/explorer/hashes/:hash", api.explorerHashHandler)
	}

	// Gateway API Calls
	if api.gateway != nil {
		router.GET("/gateway", api.gatewayHandlerGET)
		router.POST("/gateway", api.gatewayHandlerPOST)
		router.GET("/gateway/bandwidth", api.gatewayBandwidthHandlerGET)
		router.POST("/gateway/connect/:netaddress", RequirePassword(api.gatewayConnectHandler, requiredPassword))
		router.POST("/gateway/disconnect/:netaddress", RequirePassword(api.gatewayDisconnectHandler, requiredPassword))
		router.GET("/gateway/blocklist", api.gatewayBlocklistHandlerGET)
		router.POST("/gateway/blocklist", RequirePassword(api.gatewayBlocklistHandlerPOST, requiredPassword))

		// Deprecated fields
		router.GET("/gateway/blacklist", api.gatewayBlocklistHandlerGET)
		router.POST("/gateway/blacklist", RequirePassword(api.gatewayBlocklistHandlerPOST, requiredPassword))
	}

	// Host API Calls
	if api.host != nil {
		// Calls directly pertaining to the host.
		router.GET("/host", api.hostHandlerGET)                                                   // Get the host status.
		router.POST("/host", RequirePassword(api.hostHandlerPOST, requiredPassword))              // Change the settings of the host.
		router.POST("/host/announce", RequirePassword(api.hostAnnounceHandler, requiredPassword)) // Announce the host to the network.
		router.GET("/host/contracts", api.hostContractInfoHandler)                                // Get info about contracts.
		router.GET("/host/contracts/:contractID", api.hostContractGetHandler)                     // Get info about a contract.
		router.GET("/host/estimatescore", api.hostEstimateScoreGET)
		router.GET("/host/bandwidth", api.hostBandwidthHandlerGET)

		// Calls pertaining to the storage manager that the host uses.
		router.GET("/host/storage", api.storageHandler)
		router.POST("/host/storage/folders/add", RequirePassword(api.storageFoldersAddHandler, requiredPassword))
		router.POST("/host/storage/folders/remove", RequirePassword(api.storageFoldersRemoveHandler, requiredPassword))
		router.POST("/host/storage/folders/resize", RequirePassword(api.storageFoldersResizeHandler, requiredPassword))
		router.POST("/host/storage/sectors/delete/:merkleroot", RequirePassword(api.storageSectorsDeleteHandler, requiredPassword))
	}

	// Miner API Calls
	if api.miner != nil {
		router.GET("/miner", api.minerHandler)
		router.POST("/miner/block", RequirePassword(api.minerBlockHandlerPOST, requiredPassword))
		router.GET("/miner/header", RequirePassword(api.minerHeaderHandlerGET, requiredPassword))
		router.POST("/miner/header", RequirePassword(api.minerHeaderHandlerPOST, requiredPassword))
		router.GET("/miner/start", RequirePassword(api.minerStartHandler, requiredPassword))
		router.GET("/miner/stop", RequirePassword(api.minerStopHandler, requiredPassword))
	}

	// Mining pool API Calls
	if api.pool != nil {
		router.GET("/pool", api.poolHandler)
		// router.GET("/pool/clients", api.poolGetClientsInfo)
		// router.GET("/pool/client", api.poolGetClientInfo)
		router.POST("/pool/config", RequirePassword(api.poolConfigHandlerPOST, requiredPassword)) // Change the settings of the host.
		router.GET("/pool/config", RequirePassword(api.poolConfigHandler, requiredPassword))
		// router.GET("/pool/blocks", api.poolGetBlocksInfo)
		// router.GET("/pool/block", api.poolGetBlockInfo)
	}

	// Renter API Calls
	if api.renter != nil {
		router.GET("/renter", api.renterHandlerGET)
		router.POST("/renter", RequirePassword(api.renterHandlerPOST, requiredPassword))
		router.POST("/renter/allowance/cancel", RequirePassword(api.renterAllowanceCancelHandlerPOST, requiredPassword))
		router.GET("/renter/backups", RequirePassword(api.renterBackupsHandlerGET, requiredPassword))
		router.POST("/renter/backups/create", RequirePassword(api.renterBackupsCreateHandlerPOST, requiredPassword))
		router.POST("/renter/backups/restore", RequirePassword(api.renterBackupsRestoreHandlerGET, requiredPassword))
		router.POST("/renter/contract/cancel", RequirePassword(api.renterContractCancelHandler, requiredPassword))
		router.GET("/renter/contracts", api.renterContractsHandler)
		router.GET("/renter/contractorchurnstatus", api.renterContractorChurnStatus)

		router.GET("/renter/downloadinfo/*uid", api.renterDownloadByUIDHandlerGET)
		router.GET("/renter/downloads", api.renterDownloadsHandler)
		router.POST("/renter/downloads/clear", RequirePassword(api.renterClearDownloadsHandler, requiredPassword))
		router.GET("/renter/files", api.renterFilesHandler)
		router.GET("/renter/file/*siapath", api.renterFileHandlerGET)
		router.POST("/renter/file/*siapath", RequirePassword(api.renterFileHandlerPOST, requiredPassword))
		router.GET("/renter/prices", api.renterPricesHandler)
		router.POST("/renter/recoveryscan", RequirePassword(api.renterRecoveryScanHandlerPOST, requiredPassword))
		router.GET("/renter/recoveryscan", api.renterRecoveryScanHandlerGET)
		router.GET("/renter/fuse", api.renterFuseHandlerGET)
		router.POST("/renter/fuse/mount", RequirePassword(api.renterFuseMountHandlerPOST, requiredPassword))
		router.POST("/renter/fuse/unmount", RequirePassword(api.renterFuseUnmountHandlerPOST, requiredPassword))

		router.POST("/renter/delete/*siapath", RequirePassword(api.renterDeleteHandler, requiredPassword))
		router.GET("/renter/download/*siapath", RequirePassword(api.renterDownloadHandler, requiredPassword))
		router.POST("/renter/download/cancel", RequirePassword(api.renterCancelDownloadHandler, requiredPassword))
		router.GET("/renter/downloadasync/*siapath", RequirePassword(api.renterDownloadAsyncHandler, requiredPassword))
		router.POST("/renter/rename/*siapath", RequirePassword(api.renterRenameHandler, requiredPassword))
		router.GET("/renter/stream/*siapath", api.renterStreamHandler)
		router.POST("/renter/upload/*siapath", RequirePassword(api.renterUploadHandler, requiredPassword))
		router.GET("/renter/uploadready", api.renterUploadReadyHandler)
		router.POST("/renter/uploads/pause", RequirePassword(api.renterUploadsPauseHandler, requiredPassword))
		router.POST("/renter/uploads/resume", RequirePassword(api.renterUploadsResumeHandler, requiredPassword))
		router.POST("/renter/uploadstream/*siapath", RequirePassword(api.renterUploadStreamHandler, requiredPassword))
		router.POST("/renter/validatesiapath/*siapath", RequirePassword(api.renterValidateSiaPathHandler, requiredPassword))
		router.GET("/renter/workers", api.renterWorkersHandler)

		// Pubaccess endpoints
		router.GET("/pubaccess/blacklist", api.skynetBlacklistHandlerGET)
		router.POST("/pubaccess/blacklist", RequirePassword(api.skynetBlacklistHandlerPOST, requiredPassword))
		router.POST("/pubaccess/pin/:publink", RequirePassword(api.skynetPublinkPinHandlerPOST, requiredPassword))
		router.GET("/pubaccess/portals", api.skynetPortalsHandlerGET)
		router.POST("/pubaccess/portals", RequirePassword(api.skynetPortalsHandlerPOST, requiredPassword))
		router.GET("/pubaccess/publink/*publink", api.skynetPublinkHandlerGET)
		router.HEAD("/pubaccess/publink/*publink", api.skynetPublinkHandlerGET)
		router.POST("/pubaccess/pubfile/*siapath", RequirePassword(api.skynetSkyfileHandlerPOST, requiredPassword))
		router.GET("/pubaccess/stats", api.skynetStatsHandlerGET)
		router.GET("/pubaccess/pubaccesskey", RequirePassword(api.skykeyHandlerGET, requiredPassword))
		router.POST("/pubaccess/createpubaccesskey", RequirePassword(api.skykeyCreateKeyHandlerPOST, requiredPassword))
		router.POST("/pubaccess/addpubaccesskey", RequirePassword(api.skykeyAddKeyHandlerPOST, requiredPassword))
		router.POST("/pubaccess/deletepubaccesskey", RequirePassword(api.skykeyDeleteHandlerPOST, requiredPassword))
		router.GET("/pubaccess/pubaccesskeys", RequirePassword(api.skykeysHandlerGET, requiredPassword))

		// Directory endpoints
		router.POST("/renter/dir/*siapath", RequirePassword(api.renterDirHandlerPOST, requiredPassword))
		router.GET("/renter/dir/*siapath", api.renterDirHandlerGET)

		// HostDB endpoints.
		router.GET("/hostdb", api.hostdbHandler)
		router.GET("/hostdb/active", api.hostdbActiveHandler)
		router.GET("/hostdb/all", api.hostdbAllHandler)
		router.GET("/hostdb/hosts/:pubkey", api.hostdbHostsHandler)
		router.GET("/hostdb/filtermode", api.hostdbFilterModeHandlerGET)
		router.POST("/hostdb/filtermode", RequirePassword(api.hostdbFilterModeHandlerPOST, requiredPassword))

		// Renter watchdog endpoints.
		router.GET("/renter/contractstatus", api.renterContractStatusHandler)

		// Deprecated endpoints.
		router.POST("/renter/backup", RequirePassword(api.renterBackupHandlerPOST, requiredPassword))
		router.POST("/renter/recoverbackup", RequirePassword(api.renterLoadBackupHandlerPOST, requiredPassword))
	}

	if api.stratumminer != nil {
		router.GET("/stratumminer", api.stratumminerHandler)
		router.POST("/stratumminer/start", api.stratumminerStartHandler)
		router.POST("/stratumminer/stop", api.stratumminerStopHandler)
	}

	// Transaction pool API Calls
	if api.tpool != nil {
		router.GET("/tpool/fee", api.tpoolFeeHandlerGET)
		router.GET("/tpool/raw/:id", api.tpoolRawHandlerGET)
		router.POST("/tpool/raw", api.tpoolRawHandlerPOST)
		router.GET("/tpool/confirmed/:id", api.tpoolConfirmedGET)
		router.GET("/tpool/transactions", api.tpoolTransactionsHandler)
	}

	// Wallet API Calls
	if api.wallet != nil {
		router.GET("/wallet", api.walletHandler)
		router.POST("/wallet/033x", RequirePassword(api.wallet033xHandler, requiredPassword))
		router.GET("/wallet/address", RequirePassword(api.walletAddressHandler, requiredPassword))
		router.GET("/wallet/addresses", api.walletAddressesHandler)
		router.GET("/wallet/seedaddrs", api.walletSeedAddressesHandler)
		router.GET("/wallet/backup", RequirePassword(api.walletBackupHandler, requiredPassword))
		router.POST("/wallet/init", RequirePassword(api.walletInitHandler, requiredPassword))
		router.POST("/wallet/init/seed", RequirePassword(api.walletInitSeedHandler, requiredPassword))
		router.POST("/wallet/lock", RequirePassword(api.walletLockHandler, requiredPassword))
		router.POST("/wallet/seed", RequirePassword(api.walletSeedHandler, requiredPassword))
		router.GET("/wallet/seeds", RequirePassword(api.walletSeedsHandler, requiredPassword))
		router.POST("/wallet/siacoins", RequirePassword(api.walletSiacoinsHandler, requiredPassword))
		router.POST("/wallet/siafunds", RequirePassword(api.walletSiafundsHandler, requiredPassword))
		router.POST("/wallet/siagkey", RequirePassword(api.walletSiagkeyHandler, requiredPassword))
		router.POST("/wallet/sweep/seed", RequirePassword(api.walletSweepSeedHandler, requiredPassword))
		router.GET("/wallet/transaction/:id", api.walletTransactionHandler)
		router.GET("/wallet/transactions", api.walletTransactionsHandler)
		router.GET("/wallet/transactions/:addr", api.walletTransactionsAddrHandler)
		router.GET("/wallet/verify/address/:addr", api.walletVerifyAddressHandler)
		router.POST("/wallet/unlock", RequirePassword(api.walletUnlockHandler, requiredPassword))
		router.POST("/wallet/changepassword", RequirePassword(api.walletChangePasswordHandler, requiredPassword))
		router.GET("/wallet/verifypassword", RequirePassword(api.walletVerifyPasswordHandler, requiredPassword))
		router.GET("/wallet/unlockconditions/:addr", RequirePassword(api.walletUnlockConditionsHandlerGET, requiredPassword))
		router.POST("/wallet/unlockconditions", RequirePassword(api.walletUnlockConditionsHandlerPOST, requiredPassword))
		router.GET("/wallet/unspent", RequirePassword(api.walletUnspentHandler, requiredPassword))
		router.POST("/wallet/sign", RequirePassword(api.walletSignHandler, requiredPassword))
		router.GET("/wallet/watch", RequirePassword(api.walletWatchHandlerGET, requiredPassword))
		router.POST("/wallet/watch", RequirePassword(api.walletWatchHandlerPOST, requiredPassword))
	}

	// GUI Calls
	router.GET("/favicon.ico", api.guiFaviconHandler)
	router.GET("/gui/balance", api.guiBalanceHandler)
	router.GET("/gui/blockHeight", api.guiBlockHeightHandler)
	router.GET("/gui/downloaderProgress", api.guiDownloaderProgressHandler)
	router.GET("/gui/heartbeat", api.guiHeartbeatHandler)
	router.GET("/gui/logo.png", api.guiLogoHandler)
	router.GET("/gui/scripts.js", api.guiScriptHandler)
	router.GET("/gui/styles.css", api.guiStyleHandler)
	router.GET("/gui/fonts/open-sans-v27-latin-regular.woff2", api.guiOpenSansLatinRegularWoff2Handler)
	router.GET("/gui/fonts/open-sans-v27-latin-700.woff2", api.guiOpenSansLatin700Woff2Handler)
	if api.downloader == nil && api.gateway == nil {
		router.GET("/", api.guiDownloadingHandler)
	} else if api.gui == nil && api.gateway != nil {
		router.GET("/", api.guiNotLoadedHandler)
	} else if api.gui == nil {
		router.GET("/", api.guiLoadingHandler)
	} else {
		router.GET("/", api.guiHandler)
		router.GET("/gui", api.guiHandler)
		router.GET("/gui/export", api.redirect)
		router.GET("/gui/alert/changeLock", api.redirect)
		router.GET("/gui/alert/initializeSeed", api.redirect)
		router.GET("/gui/alert/sendCoins", api.redirect)
		router.GET("/gui/alert/receiveCoins", api.redirect)
		router.GET("/gui/alert/recoverSeed", api.redirect)
		router.GET("/gui/alert/restoreFromSeed", api.redirect)
		router.GET("/gui/changeLock", api.redirect)
		router.GET("/gui/collapseMenu", api.redirect)
		router.GET("/gui/expandMenu", api.redirect)
		router.GET("/gui/initializeSeed", api.redirect)
		router.GET("/gui/lockWallet", api.redirect)
		router.GET("/gui/restoreSeed", api.redirect)
		router.GET("/gui/scanning", api.redirect)
		router.GET("/gui/sendCoins", api.redirect)
		router.GET("/gui/setTxHistoryPage", api.redirect)
		router.GET("/gui/unlockWallet", api.redirect)
		router.GET("/gui/explorer", api.redirect)
		router.POST("/gui", api.guiHandler)
		router.POST("/gui/export", api.guiTransactionHistoryCsvExport)
		router.POST("/gui/alert/changeLock", api.guiAlertChangeLockHandler)
		router.POST("/gui/alert/initializeSeed", api.guiAlertInitializeSeedHandler)
		router.POST("/gui/alert/sendCoins", api.guiAlertSendCoinsHandler)
		router.POST("/gui/alert/receiveCoins", api.guiAlertReceiveCoinsHandler)
		router.POST("/gui/alert/recoverSeed", api.guiAlertRecoverSeedHandler)
		router.POST("/gui/alert/restoreFromSeed", api.guiAlertRestoreFromSeedHandler)
		router.POST("/gui/changeLock", api.guiChangeLockHandler)
		router.POST("/gui/collapseMenu", api.guiCollapseMenuHandler)
		router.POST("/gui/expandMenu", api.guiExpandMenuHandler)
		router.POST("/gui/initializeSeed", api.guiInitializeSeedHandler)
		router.POST("/gui/lockWallet", api.guiLockWalletHandler)
		router.POST("/gui/restoreSeed", api.guiRestoreSeedHandler)
		router.POST("/gui/scanning", api.guiScanningHandler)
		router.POST("/gui/sendCoins", api.guiSendCoinsHandler)
		router.POST("/gui/setTxHistoryPage", api.guiSetTxHistoyPage)
		router.POST("/gui/unlockWallet", api.guiUnlockWalletHandler)
		router.POST("/gui/explorer", api.guiExplorerHandler)
	}

	// Apply UserAgent middleware and return the Router
	api.routerMu.Lock()
	api.router = http.TimeoutHandler(RequireUserAgent(router, requiredUserAgent), httpServerTimeout, fmt.Sprintf("HTTP call exceeded the timeout of %v", httpServerTimeout))
	api.routerMu.Unlock()
	return
}

// RequireUserAgent is middleware that requires all requests to set a
// UserAgent that contains the specified string.
func RequireUserAgent(h http.Handler, ua string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.Contains(req.UserAgent(), ua) && !isUnrestricted(req) {
			WriteError(w, Error{"Browser access disabled due to security vulnerability. Use ScPrime-UI or spc."}, http.StatusBadRequest)
			return
		}
		h.ServeHTTP(w, req)
	})
}

// RequirePassword is middleware that requires a request to authenticate with a
// password using HTTP basic auth. Usernames are ignored. Empty passwords
// indicate no authentication is required.
func RequirePassword(h httprouter.Handle, password string) httprouter.Handle {
	// An empty password is equivalent to no password.
	if password == "" {
		return h
	}
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		_, pass, ok := req.BasicAuth()
		if !ok || pass != password {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"SiaAPI\"")
			WriteError(w, Error{"API authentication failed."}, http.StatusUnauthorized)
			return
		}
		h(w, req, ps)
	}
}

// isUnrestricted checks if a request may bypass the useragent check.
func isUnrestricted(req *http.Request) bool {
	return strings.HasPrefix(req.URL.Path, "/renter/stream/") ||
		strings.HasPrefix(req.URL.Path, "/pubaccess/publink") ||
		strings.HasPrefix(req.URL.Path, "/gui") ||
		req.URL.Path == "/favicon.ico" ||
		req.URL.Path == "/"
}
