package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/downloader"
	"gitlab.com/scpcorp/ScPrime/modules/gui"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/types"

	"github.com/julienschmidt/httprouter"
	mnemonics "gitlab.com/NebulousLabs/entropy-mnemonics"
)

func (api *API) redirect(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	http.Redirect(w, req, "/", http.StatusMovedPermanently)
}

func (api *API) guiFaviconHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var favicon = api.Favicon()
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Content-Length", strconv.Itoa(len(favicon))) //len(dec)
	w.Write(favicon)
}

func (api *API) guiBalanceHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	fmtScpBal, fmtUncBal, fmtSpfBal, fmtClmBal := api.balancesHelper()
	writeArray(w, []string{fmtScpBal, fmtUncBal, fmtSpfBal, fmtClmBal})
}

func (api *API) guiBlockHeightHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	fmtHeight, fmtStatus, fmtStatCo := api.blockHeightHelper()
	writeArray(w, []string{fmtHeight, fmtStatus, fmtStatCo})
}

func (api *API) guiDownloaderProgressHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeArray(w, []string{downloader.Progress()})
}

func (api *API) guiHeartbeatHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	gui.UpdateHeartbeat()
	go api.shutdownHelper()
	writeArray(w, []string{strconv.FormatBool(!gui.Headless())})
}

func (api *API) guiLogoHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var logo = api.Logo()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(logo))) //len(dec)
	w.Write(logo)
}

func (api *API) guiScriptHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var javascript = api.Javascript()
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(javascript))) //len(dec)
	w.Write(javascript)
}

func (api *API) guiStyleHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var cssStyleSheet = api.CssStyleSheet()
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(cssStyleSheet))) //len(dec)
	w.Write(cssStyleSheet)
}

func (api *API) guiOpenSansLatinRegularWoff2Handler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var font = api.OpenSansLatinRegularWoff2()
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Content-Length", strconv.Itoa(len(font))) //len(dec)
	w.Write(font)
}

func (api *API) guiOpenSansLatin700Woff2Handler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var font = api.OpenSansLatin700Woff2()
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Content-Length", strconv.Itoa(len(font))) //len(dec)
	w.Write(font)
}

func (api *API) guiTransactionHistoryCsvExport(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	history, err := api.transctionHistoryCsvExportHelper()
	if err != nil {
		history = "failed"
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-disposition", "attachment;filename=history.csv")
	w.Header().Set("Content-Length", strconv.Itoa(len(history))) //len(dec)
	w.Write([]byte(history))
}

func (api *API) transctionHistoryCsvExportHelper() (string, error) {
	csv := `"Transaction ID","Type","Amount SCP","Amount SPF","Confirmed","DateTime"` + "\n"
	heightMin := 0
	confirmedTxns, err := api.wallet.Transactions(types.BlockHeight(heightMin), api.cs.Height())
	if err != nil {
		return "", err
	}
	unconfirmedTxns, err := api.wallet.UnconfirmedTransactions()
	if err != nil {
		return "", err
	}
	sts, err := wallet.ComputeSummarizedTransactions(append(confirmedTxns, unconfirmedTxns...), api.cs.Height())
	if err != nil {
		return "", err
	}
	for _, txn := range sts {
		// Format transaction type
		if txn.Type != "SETUP" {
			fmtSpf := txn.Spf
			if fmtSpf == "" {
				fmtSpf = "0"
			}
			csv = csv + fmt.Sprintf(`"%s","%s","%s","%s","%s","%s"`, txn.TxnId, txn.Type, txn.Scp, fmtSpf, txn.Confirmed, txn.Time) + "\n"
		}
	}
	return csv, nil
}

func (api *API) guiAlertChangeLockHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		msg := "Session ID does not exist."
		api.writeError(w, msg, "")
	}
	title := "CHANGE LOCK"
	form := api.ChangeLockForm()
	api.writeForm(w, title, form, sessionId)
}

func (api *API) guiAlertInitializeSeedHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	title := "CREATE NEW WALLET"
	form := api.IntializeSeedForm()
	api.writeForm(w, title, form, "")
}

func (api *API) guiAlertSendCoinsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		msg := "Session ID does not exist."
		api.writeError(w, msg, "")
	}
	title := "SEND"
	form := api.SendCoinsForm()
	api.writeForm(w, title, form, sessionId)
}

func (api *API) guiAlertReceiveCoinsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		msg := "Session ID does not exist."
		api.writeError(w, msg, "")
	}
	var msgPrefix = "Unable to retrieve address: "
	addresses, err := api.wallet.LastAddresses(1)
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, sessionId)
		return
	}
	if len(addresses) == 0 {
		_, err := api.wallet.NextAddress()
		if err != nil {
			msg := fmt.Sprintf("%s%v", msgPrefix, err)
			api.writeError(w, msg, sessionId)
			return
		}
	}
	addresses, err = api.wallet.LastAddresses(1)
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, sessionId)
		return
	}
	title := "RECEIVE"
	msg := strings.ToUpper(fmt.Sprintf("%s", addresses[0]))
	api.writeMsg(w, title, msg, sessionId)
}

func (api *API) guiAlertRecoverSeedHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		msg := "Session ID does not exist."
		api.writeError(w, msg, "")
	}
	cancel := req.FormValue("cancel")
	var msgPrefix = "Unable to recover seed: "
	if cancel == "true" {
		api.guiHandler(w, req, resp)
		return
	}
	unlocked, err := api.wallet.Unlocked()
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, "")
		return
	}
	if !unlocked {
		msg := msgPrefix + "Wallet is locked."
		api.writeError(w, msg, "")
		return
	}
	// Get the primary seed information.
	dictionary := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictionary == "" {
		dictionary = mnemonics.English
	}
	primarySeed, _, err := api.wallet.PrimarySeed()
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, sessionId)
		return
	}
	primarySeedStr, err := modules.SeedToString(primarySeed, dictionary)
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, sessionId)
		return
	}
	title := "RECOVER SEED"
	msg := fmt.Sprintf("%s", primarySeedStr)
	api.writeMsg(w, title, msg, sessionId)
}

func (api *API) guiAlertRestoreFromSeedHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	if !api.cs.Synced() {
		msg := "Wallet must be syncronized with the network before it can be restored from a seed."
		api.writeError(w, msg, "")
		return
	}
	title := "RESTORE FROM SEED"
	form := api.RestoreFromSeedForm()
	api.writeForm(w, title, form, "")
}

func (api *API) guiChangeLockHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		msg := "Session ID does not exist."
		api.writeError(w, msg, "")
	}
	cancel := req.FormValue("cancel")
	origPassword := req.FormValue("orig_password")
	newPassword := req.FormValue("new_password")
	confirmPassword := req.FormValue("confirm_password")
	var msgPrefix = "Unable to change lock: "
	if cancel == "true" {
		api.guiHandler(w, req, resp)
		return
	}
	if origPassword == "" {
		msg := msgPrefix + "The original password must be provided."
		api.writeError(w, msg, sessionId)
		return
	}
	validPass, err := api.isPasswordValid(origPassword)
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, sessionId)
		return
	} else if !validPass {
		msg := msgPrefix + "The original password is not valid."
		api.writeError(w, msg, sessionId)
		return
	}
	if newPassword == "" {
		msg := msgPrefix + "A new password must be provided."
		api.writeError(w, msg, sessionId)
		return
	}
	if confirmPassword == "" {
		msg := msgPrefix + "A confirmation password must be provided."
		api.writeError(w, msg, sessionId)
		return
	}
	if newPassword != confirmPassword {
		msg := msgPrefix + "New password does not match confirmation password."
		api.writeError(w, msg, sessionId)
		return
	}
	var newKey crypto.CipherKey
	newKey = crypto.NewWalletKey(crypto.HashObject(newPassword))
	primarySeed, _, _ := api.wallet.PrimarySeed()
	err = api.wallet.ChangeKeyWithSeed(primarySeed, newKey)
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, sessionId)
		return
	}
	api.guiHandler(w, req, resp)
}

func (api *API) guiInitializeSeedHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	cancel := req.FormValue("cancel")
	newPassword := req.FormValue("new_password")
	confirmPassword := req.FormValue("confirm_password")
	var msgPrefix = "Unable to initialize new wallet seed: "
	if cancel == "true" {
		api.guiHandler(w, req, resp)
		return
	}
	if newPassword == "" {
		msg := msgPrefix + "A new password must be provided."
		api.writeError(w, msg, "")
		return
	}
	if confirmPassword == "" {
		msg := msgPrefix + "A confirmation password must be provided."
		api.writeError(w, msg, "")
		return
	}
	if newPassword != confirmPassword {
		msg := msgPrefix + "New password does not match confirmation password."
		api.writeError(w, msg, "")
		return
	}
	encrypted, err := api.wallet.Encrypted()
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, "")
		return
	}
	if encrypted {
		msg := msgPrefix + "Seed was already initialized."
		api.writeError(w, msg, "")
		return
	}
	go api.initializeSeedHelper(newPassword)
	title := "<font class='status &STATUS_COLOR;'>&STATUS;</font> WALLET"
	form := api.ScanningWalletForm()
	sessionId := api.gui.AddSessionId()
	api.writeForm(w, title, form, sessionId)
}

func (api *API) guiLockWalletHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		msg := "Session ID does not exist."
		api.writeError(w, msg, "")
	}
	cancel := req.FormValue("cancel")
	var msgPrefix = "Unable to lock wallet: "
	if cancel == "true" {
		api.guiHandler(w, req, resp)
		return
	}
	unlocked, err := api.wallet.Unlocked()
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, "")
		return
	}
	if !unlocked {
		msg := msgPrefix + "Wallet was already locked."
		api.writeError(w, msg, "")
		return
	}
	api.wallet.Lock()
	api.guiHandler(w, req, resp)
}

func (api *API) guiRestoreSeedHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	cancel := req.FormValue("cancel")
	newPassword := req.FormValue("new_password")
	confirmPassword := req.FormValue("confirm_password")
	seedStr := req.FormValue("seed_str")
	var msgPrefix = "Unable to restore wallet from seed: "
	if cancel == "true" {
		api.guiHandler(w, req, resp)
		return
	}
	if newPassword == "" {
		msg := msgPrefix + "A new password must be provided."
		api.writeError(w, msg, "")
		return
	}
	if confirmPassword == "" {
		msg := msgPrefix + "A confirmation password must be provided."
		api.writeError(w, msg, "")
		return
	}
	if newPassword != confirmPassword {
		msg := msgPrefix + "New password does not match confirmation password."
		api.writeError(w, msg, "")
		return
	}
	if seedStr == "" {
		msg := msgPrefix + "A seed must be provided."
		api.writeError(w, msg, "")
		return
	}
	encrypted, err := api.wallet.Encrypted()
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, "")
		return
	}
	if encrypted {
		msg := msgPrefix + "Seed is already initialized."
		api.writeError(w, msg, "")
		return
	}
	seed, err := modules.StringToSeed(seedStr, "english")
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, "")
		return
	}
	go api.restoreSeedHelper(newPassword, seed)
	title := "<font class='status &STATUS_COLOR;'>&STATUS;</font> WALLET"
	form := api.ScanningWalletForm()
	sessionId := api.gui.AddSessionId()
	api.writeForm(w, title, form, sessionId)
}

func (api *API) guiSendCoinsHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		msg := "Session ID does not exist."
		api.writeError(w, msg, "")
	}
	cancel := req.FormValue("cancel")
	var msgPrefix = "Unable to send coins: "
	if cancel == "true" {
		api.guiHandler(w, req, resp)
		return
	}
	unlocked, err := api.wallet.Unlocked()
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, "")
		return
	}
	if !unlocked {
		msg := msgPrefix + "Wallet is locked."
		api.writeError(w, msg, "")
		return
	}
	// Verify destination address was supplied.
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		msg := msgPrefix + "Destination is not valid."
		api.writeError(w, msg, sessionId)
		return
	}
	coinType := req.FormValue("coin_type")
	if coinType == "SCP" {
		amount, err := types.NewCurrencyStr(req.FormValue("amount") + "SCP")
		if err != nil {
			msg := fmt.Sprintf("%s%v", msgPrefix, err)
			api.writeError(w, msg, sessionId)
			return
		}
		_, err = api.wallet.SendSiacoins(amount, dest)
		if err != nil {
			msg := fmt.Sprintf("%s%v", msgPrefix, err)
			api.writeError(w, msg, sessionId)
			return
		}
	} else if coinType == "SPF" {
		amount, err := types.NewCurrencyStr(req.FormValue("amount") + "SPF")
		if err != nil {
			msg := fmt.Sprintf("%s%v", msgPrefix, err)
			api.writeError(w, msg, sessionId)
			return
		}
		_, err = api.wallet.SendSiafunds(amount, dest)
		if err != nil {
			msg := fmt.Sprintf("%s%v", msgPrefix, err)
			api.writeError(w, msg, sessionId)
			return
		}
	} else {
		msg := msgPrefix + "Coin type was not supplied."
		api.writeError(w, msg, sessionId)
		return
	}
	api.guiHandler(w, req, resp)
}

func (api *API) guiUnlockWalletHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	password := req.FormValue("password")
	var msgPrefix = "Unable to unlock wallet: "
	if password == "" {
		msg := "A password must be provided."
		api.writeError(w, msgPrefix+msg, "")
		return
	}
	potentialKeys, _ := encryptionKeys(password)
	for _, key := range potentialKeys {
		unlocked, err := api.wallet.Unlocked()
		if err != nil {
			msg := fmt.Sprintf("%s%v", msgPrefix, err)
			api.writeError(w, msg, "")
			return
		}
		if !unlocked {
			api.wallet.Unlock(key)
		}
	}
	unlocked, err := api.wallet.Unlocked()
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, "")
		return
	}
	if !unlocked {
		msg := msgPrefix + "Password is not valid."
		api.writeError(w, msg, "")
		return
	}
	sessionId := api.gui.AddSessionId()
	if api.gui.Status() != "" {
		title := "<font class='status &STATUS_COLOR;'>&STATUS;</font> WALLET"
		form := api.ScanningWalletForm()
		api.writeForm(w, title, form, sessionId)
		return
	}
	api.writeWallet(w, sessionId)
}

func (api *API) guiExplorerHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		form := api.UnlockWalletForm()
		api.writeForm(w, "UNLOCK WALLET", form, "")
		return
	}
	var msgPrefix = "Unable to retrieve the transaction: "
	if req.FormValue("transaction_id") == "" {
		msg := msgPrefix + "No transaction ID was provided."
		api.writeError(w, msg, sessionId)
		return
	}
	var transactionId types.TransactionID
	jsonID := "\"" + req.FormValue("transaction_id") + "\""
	err := transactionId.UnmarshalJSON([]byte(jsonID))
	if err != nil {
		msg := msgPrefix + "Unable to parse transaction ID."
		api.writeError(w, msg, sessionId)
		return
	}
	txn, ok, err := api.wallet.Transaction(transactionId)
	if err != nil {
		msg := fmt.Sprintf("%s%v", msgPrefix, err)
		api.writeError(w, msg, sessionId)
		return
	}
	if !ok {
		msg := msgPrefix + "Transaction was not found."
		api.writeError(w, msg, sessionId)
		return
	}
	transactionDetails, _ := api.transactionExplorerHelper(txn)
	html := api.WalletHtmlTemplate()
	html = strings.Replace(html, "&TRANSACTION_PORTAL;", transactionDetails, -1)
	api.writeHtml(w, html, sessionId)
}

func (api *API) guiDownloadingHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	html := strings.Replace(api.DownloadingHtml(), "&DOWNLOADER_PROGRESS;", downloader.Progress(), -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

func (api *API) guiLoadingHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body = api.LoadingHtml()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body))) //len(dec)
	w.Write(body)
}

func (api *API) guiNotLoadedHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	writeArray(w, []string{"The GUI module is not loaded."})
}

func (api *API) guiExpandMenuHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		form := api.UnlockWalletForm()
		api.writeForm(w, "UNLOCK WALLET", form, "")
		return
	}
	api.gui.ExpandMenu(sessionId)
	api.writeHtml(w, api.gui.GetCachedPage(sessionId), sessionId)
}

func (api *API) guiCollapseMenuHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		form := api.UnlockWalletForm()
		api.writeForm(w, "UNLOCK WALLET", form, "")
		return
	}
	api.gui.CollapseMenu(sessionId)
	api.writeHtml(w, api.gui.GetCachedPage(sessionId), sessionId)
}

func (api *API) guiScanningHandler(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	height, _, _ := api.blockHeightHelper()
	if height == "0" && api.gui.Status() != "" {
		title := "<font class='status &STATUS_COLOR;'>&STATUS;</font> WALLET"
		form := api.ScanningWalletForm()
		api.writeForm(w, title, form, sessionId)
		return
	}
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		form := api.UnlockWalletForm()
		api.writeForm(w, "UNLOCK WALLET", form, "")
		return
	}
	if api.gui.Status() != "" {
		title := "<font class='status &STATUS_COLOR;'>&STATUS;</font> WALLET"
		form := api.ScanningWalletForm()
		api.writeForm(w, title, form, sessionId)
		return
	}
	api.guiHandler(w, req, resp)
}

func (api *API) guiSetTxHistoyPage(w http.ResponseWriter, req *http.Request, resp httprouter.Params) {
	sessionId := req.FormValue("session_id")
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		form := api.UnlockWalletForm()
		api.writeForm(w, "UNLOCK WALLET", form, "")
		return
	}
	page, _ := strconv.Atoi(req.FormValue("page"))
	api.gui.SetTxHistoryPage(page, sessionId)
	api.guiHandler(w, req, resp)
}

func (api *API) guiHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	sessionId := req.FormValue("session_id")
	height, _, _ := api.blockHeightHelper()
	if height == "0" && api.gui.Status() != "" {
		title := "<font class='status &STATUS_COLOR;'>&STATUS;</font> WALLET"
		form := api.ScanningWalletForm()
		api.writeForm(w, title, form, sessionId)
		return
	}
	encrypted, err := api.wallet.Encrypted()
	if err != nil {
		msg := fmt.Sprintf("Unable to determine if wallet is encrypted: %v", err)
		api.writeError(w, msg, sessionId)
		return
	}
	if !encrypted {
		title := "INITIALIZE WALLET"
		form := api.InitializeWalletForm()
		api.writeForm(w, title, form, sessionId)
		return
	}
	if sessionId == "" || !api.gui.SessionIdExists(sessionId) {
		form := api.UnlockWalletForm()
		api.writeForm(w, "UNLOCK WALLET", form, sessionId)
		return
	}
	if api.gui.Status() != "" {
		title := "<font class='status &STATUS_COLOR;'>&STATUS;</font> WALLET"
		form := api.ScanningWalletForm()
		api.writeForm(w, title, form, sessionId)
		return
	}
	unlocked, err := api.wallet.Unlocked()
	if err != nil {
		msg := fmt.Sprintf("Unable to determine if wallet is unlocked: %v", err)
		api.writeError(w, msg, sessionId)
		return
	}
	if unlocked {
		api.writeWallet(w, sessionId)
		return
	}
	title := "UNLOCK WALLET"
	form := api.UnlockWalletForm()
	api.writeForm(w, title, form, "")
}

func (api *API) writeWallet(w http.ResponseWriter, sessionId string) {
	transactionHistoryLines, pages, err := api.transactionHistoryHelper(sessionId)
	if err != nil {
		msg := fmt.Sprintf("Unable to generate transaction history: %v", err)
		api.writeError(w, msg, sessionId)
		return
	}
	html := api.WalletHtmlTemplate()
	html = strings.Replace(html, "&TRANSACTION_PORTAL;", api.TransactionsHistoryHtmlTemplate(), -1)
	html = strings.Replace(html, "&TRANSACTION_HISTORY_LINES;", transactionHistoryLines, -1)
	options := ""
	for i := 0; i < pages+1; i++ {
		selected := ""
		if i+1 == api.gui.GetTxHistoryPage(sessionId) {
			selected = "selected"
		}
		options = fmt.Sprintf("<option %s value='%d'>%d</option>", selected, i+1, i+1) + options
	}
	if pages == 0 {
		html = strings.Replace(html, "&TRANSACTION_PAGINATION;", "<div class='col-4 center no-wrap'></div>", -1)
	} else {
		html = strings.Replace(html, "&TRANSACTION_PAGINATION;", api.TransactionPaginationTemplate(), -1)
	}
	html = strings.Replace(html, "&TRANSACTION_HISTORY_PAGE;", options, -1)
	html = strings.Replace(html, "&TRANSACTION_HISTORY_PAGES;", strconv.Itoa(pages+1), -1)
	api.writeHtml(w, html, sessionId)
}

func writeArray(w http.ResponseWriter, arr []string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encjson, _ := json.Marshal(arr)
	fmt.Fprint(w, string(encjson))
}

func (api *API) writeError(w http.ResponseWriter, msg string, sessionId string) {
	html := api.AlertHtmlTemplate()
	html = strings.Replace(html, "&POPUP_TITLE;", "ERROR", -1)
	html = strings.Replace(html, "&POPUP_CONTENT;", msg, -1)
	html = strings.Replace(html, "&POPUP_CLOSE;", api.CloseAlertForm(), -1)
	fmt.Println(msg)
	api.writeHtml(w, html, sessionId)
}

func (api *API) writeMsg(w http.ResponseWriter, title string, msg string, sessionId string) {
	html := api.AlertHtmlTemplate()
	html = strings.Replace(html, "&POPUP_TITLE;", title, -1)
	html = strings.Replace(html, "&POPUP_CONTENT;", msg, -1)
	html = strings.Replace(html, "&POPUP_CLOSE;", api.CloseAlertForm(), -1)
	api.writeHtml(w, html, sessionId)
}

func (api *API) writeForm(w http.ResponseWriter, title string, form string, sessionId string) {
	html := api.AlertHtmlTemplate()
	html = strings.Replace(html, "&POPUP_TITLE;", title, -1)
	html = strings.Replace(html, "&POPUP_CONTENT;", form, -1)
	html = strings.Replace(html, "&POPUP_CLOSE;", "", -1)
	api.writeHtml(w, html, sessionId)
}

func (api *API) writeHtml(w http.ResponseWriter, html string, sessionId string) {
	api.gui.CachedPage(html, sessionId)
	fmtHeight, fmtStatus, fmtStatCo := api.blockHeightHelper()
	html = strings.Replace(html, "&STATUS_COLOR;", fmtStatCo, -1)
	html = strings.Replace(html, "&STATUS;", fmtStatus, -1)
	html = strings.Replace(html, "&BLOCK_HEIGHT;", fmtHeight, -1)
	fmtScpBal, fmtUncBal, fmtSpfBal, fmtClmBal := api.balancesHelper()
	html = strings.Replace(html, "&SCP_BALANCE;", fmtScpBal, -1)
	html = strings.Replace(html, "&UNCONFIRMED_DELTA;", fmtUncBal, -1)
	html = strings.Replace(html, "&SPF_BALANCE;", fmtSpfBal, -1)
	html = strings.Replace(html, "&SCP_CLAIM_BALANCE;", fmtClmBal, -1)
	if api.gui.MenuIsCollapsed(sessionId) {
		html = strings.Replace(html, "&MENU;", api.CollapsedMenuForm(), -1)
	} else {
		html = strings.Replace(html, "&MENU;", api.ExpandedMenuForm(), -1)
	}
	html = strings.Replace(html, "&SESSION_ID;", sessionId, -1)
	// add random data to links to act as a cache buster.
	// must be done last in case a cache buster is added in from a template.
	b := make([]byte, 16) //32 characters long
	rand.Read(b)
	cacheBuster := hex.EncodeToString(b)
	html = strings.Replace(html, "&CACHE_BUSTER;", cacheBuster, -1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

func (api *API) balancesHelper() (string, string, string, string) {
	unlocked, err := api.wallet.Unlocked()
	if err != nil {
		fmt.Printf("Unable to determine if wallet is unlocked: %v", err)
	}
	fmtScpBal := "?"
	fmtUncBal := "?"
	fmtSpfBal := "?"
	fmtClmBal := "?"
	if unlocked {
		scpBal, spfBal, scpClaimBal, err := api.wallet.ConfirmedBalance()
		if err != nil {
			fmt.Printf("Unable to obtain confirmed balance: %v", err)
		} else {
			scpBalFloat, _ := new(big.Rat).SetFrac(scpBal.Big(), types.ScPrimecoinPrecision.Big()).Float64()
			scpClaimBalFloat, _ := new(big.Rat).SetFrac(scpClaimBal.Big(), types.ScPrimecoinPrecision.Big()).Float64()
			fmtScpBal = fmt.Sprintf("%15.2f", scpBalFloat)
			fmtSpfBal = fmt.Sprintf("%s", spfBal)
			fmtClmBal = fmt.Sprintf("%15.2f", scpClaimBalFloat)
		}
		scpOut, scpIn, err := api.wallet.UnconfirmedBalance()
		if err != nil {
			fmt.Printf("Unable to obtain unconfirmed balance: %v", err)
		} else {
			scpInFloat, _ := new(big.Rat).SetFrac(scpIn.Big(), types.ScPrimecoinPrecision.Big()).Float64()
			scpOutFloat, _ := new(big.Rat).SetFrac(scpOut.Big(), types.ScPrimecoinPrecision.Big()).Float64()
			fmtUncBal = fmt.Sprintf("%15.2f", (scpInFloat - scpOutFloat))
		}
	}
	return fmtScpBal, fmtUncBal, fmtSpfBal, fmtClmBal
}

func (api *API) blockHeightHelper() (string, string, string) {
	fmtHeight := "?"
	height, err := api.wallet.Height()
	if err != nil {
		fmt.Printf("Unable to obtain block height: %v", err)
	} else {
		fmtHeight = fmt.Sprintf("%d", height)
	}
	rescanning, err := api.wallet.Rescanning()
	if err != nil {
		fmt.Printf("Unable to determine if wallet is being scanned: %v", err)
	}
	synced := api.cs.Synced()
	status := api.gui.Status()
	if status != "" {
		return fmtHeight, status, "yellow"
	} else if rescanning {
		return fmtHeight, "Rescanning", "cyan"
	} else if synced {
		return fmtHeight, "Synchronized", "blue"
	} else {
		return fmtHeight, "Synchronizing", "yellow"
	}
}

func (api *API) initializeSeedHelper(newPassword string) {
	api.gui.SetStatus("Initializing")
	var encryptionKey crypto.CipherKey = crypto.NewWalletKey(crypto.HashObject(newPassword))
	_, err := api.wallet.Encrypt(encryptionKey)
	if err != nil {
		fmt.Printf("Unable to initialize new wallet seed: %v", err)
		return
	}
	potentialKeys, _ := encryptionKeys(newPassword)
	for _, key := range potentialKeys {
		unlocked, err := api.wallet.Unlocked()
		if err != nil {
			fmt.Printf("Unable to initialize new wallet seed: %v", err)
			return
		}
		if !unlocked {
			api.wallet.Unlock(key)
		}
	}
	api.gui.SetStatus("")
}

func (api *API) isPasswordValid(password string) (bool, error) {
	keys, _ := encryptionKeys(password)
	var err error
	for _, key := range keys {
		valid, keyErr := api.wallet.IsMasterKey(key)
		if keyErr == nil {
			if valid {
				return true, nil
			}
			return false, nil
		}
		err = errors.Compose(err, keyErr)
	}
	return false, err
}

func (api *API) restoreSeedHelper(newPassword string, seed modules.Seed) {
	api.gui.SetStatus("Restoring")
	var encryptionKey crypto.CipherKey = crypto.NewWalletKey(crypto.HashObject(newPassword))
	err := api.wallet.InitFromSeed(encryptionKey, seed)
	if err != nil {
		fmt.Printf("Unable to restore wallet seed: %v", err)
		return
	}
	potentialKeys, _ := encryptionKeys(newPassword)
	for _, key := range potentialKeys {
		unlocked, err := api.wallet.Unlocked()
		if err != nil {
			fmt.Printf("Unable to initialize new wallet seed: %v", err)
			return
		}
		if !unlocked {
			api.wallet.Unlock(key)
		}
	}
	api.gui.SetStatus("")
}

func (api *API) shutdownHelper() {
	if gui.Headless() {
		return
	}
	time.Sleep(5000 * time.Millisecond)
	if time.Now().After(gui.Heartbeat().Add(5000 * time.Millisecond)) {
		fmt.Println("Shutting Down...")
		api.Shutdown()
	}
}

func (api *API) transactionExplorerHelper(txn modules.ProcessedTransaction) (string, error) {
	unixTime, _ := strconv.ParseInt(fmt.Sprintf("%v", txn.ConfirmationTimestamp), 10, 64)
	fmtTime := strings.ToUpper(time.Unix(unixTime, 0).Format("2006-01-02 15:04"))
	fmtTxnId := strings.ToUpper(fmt.Sprintf("%v", txn.TransactionID))
	fmtTxnType := strings.ToUpper(strings.Replace(fmt.Sprintf("%v", txn.TxType), "_", " ", -1))
	fmtTxnBlock := strings.ToUpper(fmt.Sprintf("%v", txn.ConfirmationHeight))
	html := api.TransactionInfoTemplate()
	html = strings.Replace(html, "&TXN_TYPE;", fmtTxnType, -1)
	html = strings.Replace(html, "&TXN_ID;", fmtTxnId, -1)
	html = strings.Replace(html, "&TXN_TIME;", fmtTime, -1)
	html = strings.Replace(html, "&TXN_BLOCK;", fmtTxnBlock, -1)
	inputs := ""
	for _, input := range txn.Inputs {
		fmtValue := strings.ToUpper(fmt.Sprintf("%v", input.Value))
		fmtAddress := strings.ToUpper(fmt.Sprintf("%v", input.RelatedAddress))
		fmtFundType := strings.ToUpper(strings.Replace(fmt.Sprintf("%v", input.FundType), "_", " ", -1))
		fmtFundType = strings.Replace(fmtFundType, "SIACOIN", "SCP", -1)
		fmtFundType = strings.Replace(fmtFundType, "SIAFUND", "SPF", -1)
		row := api.TransactionInputTemplate()
		row = strings.Replace(row, "&VALUE;", fmtValue, -1)
		row = strings.Replace(row, "&ADDRESS;", fmtAddress, -1)
		row = strings.Replace(row, "&FUND_TYPE;", fmtFundType, -1)
		inputs = inputs + row
	}
	html = strings.Replace(html, "&TXN_INPUTS;", inputs, -1)
	outputs := ""
	for _, output := range txn.Outputs {
		fmtValue := strings.ToUpper(fmt.Sprintf("%v", output.Value))
		fmtAddress := strings.ToUpper(fmt.Sprintf("%v", output.RelatedAddress))
		fmtFundType := strings.ToUpper(strings.Replace(fmt.Sprintf("%v", output.FundType), "_", " ", -1))
		fmtFundType = strings.Replace(fmtFundType, "SIACOIN", "SCP", -1)
		fmtFundType = strings.Replace(fmtFundType, "SIAFUND", "SPF", -1)
		row := api.TransactionOutputTemplate()
		row = strings.Replace(row, "&VALUE;", fmtValue, -1)
		row = strings.Replace(row, "&ADDRESS;", fmtAddress, -1)
		row = strings.Replace(row, "&FUND_TYPE;", fmtFundType, -1)
		outputs = outputs + row
	}
	html = strings.Replace(html, "&TXN_OUTPUTS;", outputs, -1)
	return html, nil
}

func (api *API) transactionHistoryHelper(sessionId string) (string, int, error) {
	html := ""
	page := api.gui.GetTxHistoryPage(sessionId)
	pageSize := 20
	pageMin := (page - 1) * pageSize
	pageMax := page * pageSize
	count := 0
	heightMin := 0
	confirmedTxns, err := api.wallet.Transactions(types.BlockHeight(heightMin), api.cs.Height())
	if err != nil {
		return "", -1, err
	}
	unconfirmedTxns, err := api.wallet.UnconfirmedTransactions()
	if err != nil {
		return "", -1, err
	}
	sts, err := wallet.ComputeSummarizedTransactions(append(confirmedTxns, unconfirmedTxns...), api.cs.Height())
	if err != nil {
		return "", -1, err
	}
	for _, txn := range sts {
		// Format transaction type
		isSetup := txn.Type == "SETUP" && txn.Scp == fmt.Sprintf("%15.2f SCP", float64(0))
		if !isSetup {
			count++
			if count >= pageMin && count < pageMax {
				fmtAmount := txn.Scp
				if txn.Spf != "" {
					fmtAmount = fmtAmount + "; " + txn.Spf
				}
				row := api.TransactionHistoryLineHtmlTemplate()
				row = strings.Replace(row, "&TRANSACTION_ID;", txn.TxnId, -1)
				row = strings.Replace(row, "&TYPE;", txn.Type, -1)
				row = strings.Replace(row, "&TIME;", txn.Time, -1)
				row = strings.Replace(row, "&AMOUNT;", fmtAmount, -1)
				row = strings.Replace(row, "&CONFIRMED;", txn.Confirmed, -1)
				html = html + row
			}
		}
	}
	return html, count / pageSize, nil
}
