package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"

	"github.com/julienschmidt/httprouter"
	mnemonics "gitlab.com/NebulousLabs/entropy-mnemonics"
	"gitlab.com/NebulousLabs/errors"
)

type (
	// WalletGET contains general information about the wallet.
	WalletGET struct {
		Encrypted  bool              `json:"encrypted"`
		Height     types.BlockHeight `json:"height"`
		Rescanning bool              `json:"rescanning"`
		Unlocked   bool              `json:"unlocked"`

		ConfirmedSiacoinBalance     types.Currency `json:"confirmedsiacoinbalance"`
		UnconfirmedOutgoingSiacoins types.Currency `json:"unconfirmedoutgoingsiacoins"`
		UnconfirmedIncomingSiacoins types.Currency `json:"unconfirmedincomingsiacoins"`

		SiacoinClaimBalance    types.Currency `json:"siacoinclaimbalance"`
		SiafundBalance         types.Currency `json:"siafundbalance"`
		SiafundBBalance        types.Currency `json:"siafundbbalance"`
		SiacoinBClaimBalance   types.Currency `json:"siacoinbclaimbalance"`
		SiacoinBUnclaimBalance types.Currency `json:"siacoinbunclaimbalance"`

		DustThreshold types.Currency `json:"dustthreshold"`
	}

	// WalletAddressGET contains an address returned by a GET call to
	// /wallet/address.
	WalletAddressGET struct {
		Address          types.UnlockHash       `json:"address"`
		UnlockConditions types.UnlockConditions `json:"unlock_conditions"`
	}

	// WalletAddressesGET contains the list of wallet addresses returned by a
	// GET call to /wallet/addresses.
	WalletAddressesGET struct {
		Addresses []types.UnlockHash `json:"addresses"`
	}

	// WalletInitPOST contains the primary seed that gets generated during a
	// POST call to /wallet/init.
	WalletInitPOST struct {
		PrimarySeed string `json:"primaryseed"`
	}

	// WalletSiacoinsPOST contains the transaction sent in the POST call to
	// /wallet/siacoins.
	WalletSiacoinsPOST struct {
		Transactions   []types.Transaction   `json:"transactions"`
		TransactionIDs []types.TransactionID `json:"transactionids"`
	}

	// WalletSiafundsPOST contains the transaction sent in the POST call to
	// /wallet/siafunds.
	WalletSiafundsPOST struct {
		Transactions   []types.Transaction   `json:"transactions"`
		TransactionIDs []types.TransactionID `json:"transactionids"`
	}

	// WalletBatchTransactionPOST contains the transaction sent in the POST
	// call to /wallet/batchtransaction
	WalletBatchTransactionPOST struct {
		Transactions   []types.Transaction   `json:"transactions"`
		TransactionIDs []types.TransactionID `json:"transactionids"`
	}

	// WalletSignPOSTParams contains the unsigned transaction and a set of
	// inputs to sign.
	WalletSignPOSTParams struct {
		Transaction types.Transaction `json:"transaction"`
		ToSign      []crypto.Hash     `json:"tosign"`
	}

	// WalletSignPOSTResp contains the signed transaction.
	WalletSignPOSTResp struct {
		Transaction types.Transaction `json:"transaction"`
	}

	// WalletSeedsGET contains the seeds used by the wallet.
	WalletSeedsGET struct {
		PrimarySeed        string   `json:"primaryseed"`
		AddressesRemaining int      `json:"addressesremaining"`
		AllSeeds           []string `json:"allseeds"`
	}

	// WalletSweepPOST contains the coins and funds returned by a call to
	// /wallet/sweep.
	WalletSweepPOST struct {
		Coins types.Currency `json:"coins"`
		Funds types.Currency `json:"funds"`
	}

	// WalletTransactionGETid contains the transaction returned by a call to
	// /wallet/transaction/:id
	WalletTransactionGETid struct {
		Transaction modules.ProcessedTransaction `json:"transaction"`
	}

	// WalletTransactionsGET contains the specified set of confirmed and
	// unconfirmed transactions.
	WalletTransactionsGET struct {
		ConfirmedTransactions   []modules.ProcessedTransaction `json:"confirmedtransactions"`
		UnconfirmedTransactions []modules.ProcessedTransaction `json:"unconfirmedtransactions"`
	}

	// WalletTransactionsGETaddr contains the set of wallet transactions
	// relevant to the input address provided in the call to
	// /wallet/transaction/:addr
	WalletTransactionsGETaddr struct {
		ConfirmedTransactions   []modules.ProcessedTransaction `json:"confirmedtransactions"`
		UnconfirmedTransactions []modules.ProcessedTransaction `json:"unconfirmedtransactions"`
	}

	// WalletUnlockConditionsGET contains a set of unlock conditions.
	WalletUnlockConditionsGET struct {
		UnlockConditions types.UnlockConditions `json:"unlockconditions"`
	}

	// WalletUnlockConditionsPOSTParams contains a set of unlock conditions.
	WalletUnlockConditionsPOSTParams struct {
		UnlockConditions types.UnlockConditions `json:"unlockconditions"`
	}

	// WalletUnspentGET contains the unspent outputs tracked by the wallet.
	// The MaturityHeight field of each output indicates the height of the
	// block that the output appeared in.
	WalletUnspentGET struct {
		Outputs []modules.UnspentOutput `json:"outputs"`
	}

	// WalletVerifyAddressGET contains a bool indicating if the address passed to
	// /wallet/verify/address/:addr is a valid address.
	WalletVerifyAddressGET struct {
		Valid bool `json:"valid"`
	}

	// WalletVerifyPasswordGET contains a bool indicating if the password passed
	// to /wallet/verifypassword is the password being used to encrypt the
	// wallet.
	WalletVerifyPasswordGET struct {
		Valid bool `json:"valid"`
	}

	// WalletWatchPOST contains the set of addresses to add or remove from the
	// watch set.
	WalletWatchPOST struct {
		Addresses []types.UnlockHash `json:"addresses"`
		Remove    bool               `json:"remove"`
		Unused    bool               `json:"unused"`
	}

	// WalletWatchGET contains the set of addresses that the wallet is
	// currently watching.
	WalletWatchGET struct {
		Addresses []types.UnlockHash `json:"addresses"`
	}
)

// encryptionKeys enumerates the possible encryption keys that can be derived
// from an input string.
func encryptionKeys(seedStr string) (validKeys []crypto.CipherKey, seeds []modules.Seed) {
	dicts := []mnemonics.DictionaryID{"english", "german", "japanese"}
	for _, dict := range dicts {
		seed, err := modules.StringToSeed(seedStr, dict)
		if err != nil {
			continue
		}
		validKeys = append(validKeys, crypto.NewWalletKey(crypto.HashObject(seed)))
		seeds = append(seeds, seed)
	}
	validKeys = append(validKeys, crypto.NewWalletKey(crypto.HashObject(seedStr)))
	return
}

// walletHander handles API calls to /wallet.
func (api *API) walletHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	balance, err := api.wallet.ConfirmedBalance()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
		return
	}
	siacoinsOut, siacoinsIn, err := api.wallet.UnconfirmedBalance()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
		return
	}
	dustThreshold, err := api.wallet.DustThreshold()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
		return
	}
	encrypted, err := api.wallet.Encrypted()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
		return
	}
	unlocked, err := api.wallet.Unlocked()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
		return
	}
	rescanning, err := api.wallet.Rescanning()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
		return
	}
	height, err := api.wallet.Height()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletGET{
		Encrypted:  encrypted,
		Unlocked:   unlocked,
		Rescanning: rescanning,
		Height:     height,

		ConfirmedSiacoinBalance:     balance.CoinBalance,
		UnconfirmedOutgoingSiacoins: siacoinsOut,
		UnconfirmedIncomingSiacoins: siacoinsIn,

		SiafundBalance:         balance.FundBalance,
		SiacoinClaimBalance:    balance.ClaimBalance,
		SiafundBBalance:        balance.FundbBalance,
		SiacoinBClaimBalance:   balance.ClaimbBalance,
		SiacoinBUnclaimBalance: balance.UnclaimbBalance,

		DustThreshold: dustThreshold,
	})
}

// wallet033xHandler handles API calls to /wallet/033x.
func (api *API) wallet033xHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	source := req.FormValue("source")
	// Check that source is an absolute paths.
	if !filepath.IsAbs(source) {
		WriteError(w, Error{"error when calling /wallet/033x: source must be an absolute path"}, http.StatusBadRequest)
		return
	}
	potentialKeys, _ := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := api.wallet.Load033xWallet(key, source)
		if err == nil {
			WriteSuccess(w)
			return
		}
		if err != modules.ErrBadEncryptionKey {
			WriteError(w, Error{"error when calling /wallet/033x: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	WriteError(w, Error{modules.ErrBadEncryptionKey.Error()}, http.StatusBadRequest)
}

// walletAddressHandler handles API calls to /wallet/address.
func (api *API) walletAddressHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	unlockConditions, err := api.wallet.NextAddress()
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/addresses: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletAddressGET{
		Address:          unlockConditions.UnlockHash(),
		UnlockConditions: unlockConditions,
	})
}

// walletSeedAddressesHandler handles the requests to /wallet/seedaddrs.
func (api *API) walletSeedAddressesHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Parse the count argument. If it isn't specified we return as many
	// addresses as possible.
	count := uint64(math.MaxUint64)
	c := req.FormValue("count")
	if c != "" {
		_, err := fmt.Sscan(c, &count)
		if err != nil {
			WriteError(w, Error{"Failed to parse count: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	// Get the last count addresses.
	addresses, err := api.wallet.LastAddresses(count)
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet/addresses: %v", err)}, http.StatusBadRequest)
		return
	}
	// Send the response.
	WriteJSON(w, WalletAddressesGET{
		Addresses: addresses,
	})
}

// walletAddressHandler handles API calls to /wallet/addresses.
func (api *API) walletAddressesHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	addresses, err := api.wallet.AllAddresses()
	if err != nil {
		WriteError(w, Error{fmt.Sprintf("Error when calling /wallet/addresses: %v", err)}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletAddressesGET{
		Addresses: addresses,
	})
}

// walletBackupHandler handles API calls to /wallet/backup.
func (api *API) walletBackupHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	destination := req.FormValue("destination")
	// Check that the destination is absolute.
	if !filepath.IsAbs(destination) {
		WriteError(w, Error{"error when calling /wallet/backup: destination must be an absolute path"}, http.StatusBadRequest)
		return
	}
	err := api.wallet.CreateBackup(destination)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/backup: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// walletInitHandler handles API calls to /wallet/init.
func (api *API) walletInitHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var encryptionKey crypto.CipherKey
	if req.FormValue("encryptionpassword") != "" {
		encryptionKey = crypto.NewWalletKey(crypto.HashObject(req.FormValue("encryptionpassword")))
	}

	if req.FormValue("force") == "true" {
		err := api.wallet.Reset()
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/init: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	seed, err := api.wallet.Encrypt(encryptionKey)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/init: " + err.Error()}, http.StatusBadRequest)
		return
	}

	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictID == "" {
		dictID = "english"
	}
	seedStr, err := modules.SeedToString(seed, dictID)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/init: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletInitPOST{
		PrimarySeed: seedStr,
	})
}

// walletInitSeedHandler handles API calls to /wallet/init/seed.
func (api *API) walletInitSeedHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var encryptionKey crypto.CipherKey
	if req.FormValue("encryptionpassword") != "" {
		encryptionKey = crypto.NewWalletKey(crypto.HashObject(req.FormValue("encryptionpassword")))
	}
	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictID == "" {
		dictID = "english"
	}
	seed, err := modules.StringToSeed(req.FormValue("seed"), dictID)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/init/seed: " + err.Error()}, http.StatusBadRequest)
		return
	}

	if req.FormValue("force") == "true" {
		err = api.wallet.Reset()
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/init/seed: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}

	err = api.wallet.InitFromSeed(encryptionKey, seed)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/init/seed: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// walletSeedHandler handles API calls to /wallet/seed.
func (api *API) walletSeedHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Get the seed using the dictionary + phrase
	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictID == "" {
		dictID = "english"
	}
	seed, err := modules.StringToSeed(req.FormValue("seed"), dictID)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/seed: " + err.Error()}, http.StatusBadRequest)
		return
	}

	potentialKeys, _ := encryptionKeys(req.FormValue("encryptionpassword"))
	for _, key := range potentialKeys {
		err := api.wallet.LoadSeed(key, seed)
		if err == nil {
			WriteSuccess(w)
			return
		}
		if err != modules.ErrBadEncryptionKey {
			WriteError(w, Error{"error when calling /wallet/seed: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	WriteError(w, Error{"error when calling /wallet/seed: " + modules.ErrBadEncryptionKey.Error()}, http.StatusBadRequest)
}

// walletSiagkeyHandler handles API calls to /wallet/siagkey.
func (api *API) walletSiagkeyHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Fetch the list of keyfiles from the post body.
	keyfiles := strings.Split(req.FormValue("keyfiles"), ",")
	potentialKeys, _ := encryptionKeys(req.FormValue("encryptionpassword"))

	for _, keypath := range keyfiles {
		// Check that all key paths are absolute paths.
		if !filepath.IsAbs(keypath) {
			WriteError(w, Error{"error when calling /wallet/siagkey: keyfiles contains a non-absolute path"}, http.StatusBadRequest)
			return
		}
	}

	for _, key := range potentialKeys {
		err := api.wallet.LoadSiagKeys(key, keyfiles)
		if err == nil {
			WriteSuccess(w)
			return
		}
		if err != modules.ErrBadEncryptionKey {
			WriteError(w, Error{"error when calling /wallet/siagkey: " + err.Error()}, http.StatusBadRequest)
			return
		}
	}
	WriteError(w, Error{"error when calling /wallet/siagkey: " + modules.ErrBadEncryptionKey.Error()}, http.StatusBadRequest)
}

// walletLockHandler handles API calls to /wallet/lock.
func (api *API) walletLockHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	err := api.wallet.Lock()
	if err != nil {
		WriteError(w, Error{err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// walletSeedsHandler handles API calls to /wallet/seeds.
func (api *API) walletSeedsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	dictionary := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictionary == "" {
		dictionary = mnemonics.English
	}

	// Get the primary seed information.
	primarySeed, addrsRemaining, err := api.wallet.PrimarySeed()
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
		return
	}
	primarySeedStr, err := modules.SeedToString(primarySeed, dictionary)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
		return
	}

	// Get the list of seeds known to the wallet.
	allSeeds, err := api.wallet.AllSeeds()
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
		return
	}
	var allSeedsStrs []string
	for _, seed := range allSeeds {
		str, err := modules.SeedToString(seed, dictionary)
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/seeds: " + err.Error()}, http.StatusBadRequest)
			return
		}
		allSeedsStrs = append(allSeedsStrs, str)
	}
	WriteJSON(w, WalletSeedsGET{
		PrimarySeed:        primarySeedStr,
		AddressesRemaining: int(addrsRemaining),
		AllSeeds:           allSeedsStrs,
	})
}

// walletBatchTransaction handles API calls to /wallet/batchtransaction
func (api *API) walletBatchTransaction(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var coinOutputs []types.SiacoinOutput
	var fundOutputs []types.SiafundOutput
	var fundbOutputs []types.SiafundOutput
	if req.FormValue("coinOutputs") != "" {
		err := json.Unmarshal([]byte(req.FormValue("coinOutputs")), &coinOutputs)
		if err != nil {
			WriteError(w, Error{"could not decode outputs: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	}
	if req.FormValue("fundOutputs") != "" {
		err := json.Unmarshal([]byte(req.FormValue("fundOutputs")), &fundOutputs)
		if err != nil {
			WriteError(w, Error{"could not decode outputs: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	}
	if req.FormValue("fundbOutputs") != "" {
		err := json.Unmarshal([]byte(req.FormValue("fundbOutputs")), &fundbOutputs)
		if err != nil {
			WriteError(w, Error{"could not decode outputs: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	}
	// Mock the transaction to verify that the wallet is able to fund both transactions.
	txnBuilder, err := api.wallet.BuildUnsignedBatchTransaction(coinOutputs, fundOutputs, fundbOutputs)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/batchtransaction: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	txnBuilder.Drop()
	// Send the transactions
	var coinTxns []types.Transaction
	if len(coinOutputs) != 0 {
		coinTxns, err = api.wallet.SendBatchTransaction(coinOutputs, nil, nil)
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/batchtransaction: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	}
	var fundTxns []types.Transaction
	if len(fundOutputs) != 0 {
		fundTxns, err = api.wallet.SendBatchTransaction(nil, fundOutputs, nil)
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/batchtransaction: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	}
	var fundbTxns []types.Transaction
	if len(fundbOutputs) != 0 {
		fundbTxns, err = api.wallet.SendBatchTransaction(nil, nil, fundbOutputs)
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/batchtransaction: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	}
	// Collect and return the results
	var txns []types.Transaction
	var txids []types.TransactionID
	for _, coinTxn := range coinTxns {
		txns = append(txns, coinTxn)
		txids = append(txids, coinTxn.ID())
	}
	for _, fundTxn := range fundTxns {
		txns = append(txns, fundTxn)
		txids = append(txids, fundTxn.ID())
	}
	for _, fundbTxn := range fundbTxns {
		txns = append(txns, fundbTxn)
		txids = append(txids, fundbTxn.ID())
	}
	WriteJSON(w, WalletBatchTransactionPOST{
		Transactions:   txns,
		TransactionIDs: txids,
	})
}

// walletSiacoinsHandler handles API calls to /wallet/siacoins.
func (api *API) walletSiacoinsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var txns []types.Transaction
	if req.FormValue("outputs") != "" {
		// multiple amounts + destinations
		if req.FormValue("amount") != "" || req.FormValue("destination") != "" || req.FormValue("feeIncluded") != "" {
			WriteError(w, Error{"cannot supply both 'outputs' and single amount+destination pair and/or feeIncluded parameter"}, http.StatusInternalServerError)
			return
		}

		var outputs []types.SiacoinOutput
		err := json.Unmarshal([]byte(req.FormValue("outputs")), &outputs)
		if err != nil {
			WriteError(w, Error{"could not decode outputs: " + err.Error()}, http.StatusInternalServerError)
			return
		}
		txns, err = api.wallet.SendSiacoinsMulti(outputs)
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/siacoins: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	} else {
		// single amount + destination
		amount, ok := scanAmount(req.FormValue("amount"))
		if !ok {
			WriteError(w, Error{"could not read amount from POST call to /wallet/siacoins"}, http.StatusBadRequest)
			return
		}
		dest, err := scanAddress(req.FormValue("destination"))
		if err != nil {
			WriteError(w, Error{"could not read address from POST call to /wallet/siacoins"}, http.StatusBadRequest)
			return
		}
		feeIncluded, err := scanBool(req.FormValue("feeIncluded"))
		if err != nil {
			WriteError(w, Error{"could not read feeIncluded from POST call to /wallet/siacoins"}, http.StatusBadRequest)
			return
		}

		if feeIncluded {
			txns, err = api.wallet.SendSiacoinsFeeIncluded(amount, dest)
		} else {
			txns, err = api.wallet.SendSiacoins(amount, dest)
		}
		if err != nil {
			WriteError(w, Error{"error when calling /wallet/siacoins: " + err.Error()}, http.StatusInternalServerError)
			return
		}
	}

	var txids []types.TransactionID
	for _, txn := range txns {
		txids = append(txids, txn.ID())
	}
	WriteJSON(w, WalletSiacoinsPOST{
		Transactions:   txns,
		TransactionIDs: txids,
	})
}

// walletSiafundsHandler handles API calls to /wallet/siafunds.
func (api *API) walletSiafundsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		WriteError(w, Error{"could not read 'amount' from POST call to /wallet/siafunds"}, http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/siafunds: " + err.Error()}, http.StatusBadRequest)
		return
	}

	txns, err := api.wallet.SendSiafunds(amount, dest)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/siafunds: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	var txids []types.TransactionID
	for _, txn := range txns {
		txids = append(txids, txn.ID())
	}
	WriteJSON(w, WalletSiafundsPOST{
		Transactions:   txns,
		TransactionIDs: txids,
	})
}

// walletSiafundbsHandler handles API calls to /wallet/siafundbs.
func (api *API) walletSiafundbsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	amount, ok := scanAmount(req.FormValue("amount"))
	if !ok {
		WriteError(w, Error{"could not read 'amount' from POST call to /wallet/siafundbs"}, http.StatusBadRequest)
		return
	}
	dest, err := scanAddress(req.FormValue("destination"))
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/siafundbs: " + err.Error()}, http.StatusBadRequest)
		return
	}

	txns, err := api.wallet.SendSiafundbs(amount, dest)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/siafundbs: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	var txids []types.TransactionID
	for _, txn := range txns {
		txids = append(txids, txn.ID())
	}
	WriteJSON(w, WalletSiafundsPOST{
		Transactions:   txns,
		TransactionIDs: txids,
	})
}

// walletSweepSeedHandler handles API calls to /wallet/sweep/seed.
func (api *API) walletSweepSeedHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	// Get the seed using the dictionary + phrase
	dictID := mnemonics.DictionaryID(req.FormValue("dictionary"))
	if dictID == "" {
		dictID = "english"
	}
	seed, err := modules.StringToSeed(req.FormValue("seed"), dictID)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/sweep/seed: " + err.Error()}, http.StatusBadRequest)
		return
	}

	coins, funds, err := api.wallet.SweepSeed(seed)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/sweep/seed: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletSweepPOST{
		Coins: coins,
		Funds: funds,
	})
}

// walletTransactionHandler handles API calls to /wallet/transaction/:id.
func (api *API) walletTransactionHandler(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	// Parse the id from the url.
	var id types.TransactionID
	jsonID := "\"" + ps.ByName("id") + "\""
	err := id.UnmarshalJSON([]byte(jsonID))
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/transaction/id: " + err.Error()}, http.StatusBadRequest)
		return
	}

	txn, ok, err := api.wallet.Transaction(id)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/transaction/id: " + err.Error()}, http.StatusBadRequest)
		return
	}
	if !ok {
		WriteError(w, Error{"error when calling /wallet/transaction/id  :  transaction not found"}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletTransactionGETid{
		Transaction: txn,
	})
}

// walletTransactionsHandler handles API calls to /wallet/transactions.
func (api *API) walletTransactionsHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	startheightStr, endheightStr, depthStr := req.FormValue("startheight"), req.FormValue("endheight"), req.FormValue("depth")
	var start, end, depth uint64
	var err error
	if depthStr == "" {
		if startheightStr == "" || endheightStr == "" {
			WriteError(w, Error{"startheight and endheight must be provided to a /wallet/transactions call if depth is unspecified."}, http.StatusBadRequest)
			return
		}
		// Get the start and end blocks.
		start, err = strconv.ParseUint(startheightStr, 10, 64)
		if err != nil {
			WriteError(w, Error{"parsing integer value for parameter `startheight` failed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		// Check if endheightStr is set to -1. If it is, we use MaxUint64 as the
		// end. Otherwise we parse the argument as an unsigned integer.
		if endheightStr == "-1" {
			end = math.MaxUint64
		} else {
			end, err = strconv.ParseUint(endheightStr, 10, 64)
		}
		if err != nil {
			WriteError(w, Error{"parsing integer value for parameter `endheight` failed: " + err.Error()}, http.StatusBadRequest)
			return
		}
	} else {
		if startheightStr != "" || endheightStr != "" {
			WriteError(w, Error{"startheight and endheight must not be provided to a /wallet/transactions call if depth is specified."}, http.StatusBadRequest)
			return
		}
		// Get the start and end blocks by looking backwards from our current height.
		depth, err = strconv.ParseUint(depthStr, 10, 64)
		if err != nil {
			WriteError(w, Error{"parsing integer value for parameter `depth` failed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		height, err := api.wallet.Height()
		if err != nil {
			WriteError(w, Error{fmt.Sprintf("Error when calling /wallet: %v", err)}, http.StatusBadRequest)
			return
		}
		end = uint64(height)
		start = end - depth - 1
		if start < 0 {
			start = 0
		}
	}
	confirmedTxns, err := api.wallet.Transactions(types.BlockHeight(start), types.BlockHeight(end))
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/transactions: " + err.Error()}, http.StatusBadRequest)
		return
	}
	unconfirmedTxns, err := api.wallet.UnconfirmedTransactions()
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/transactions: " + err.Error()}, http.StatusBadRequest)
		return
	}

	WriteJSON(w, WalletTransactionsGET{
		ConfirmedTransactions:   confirmedTxns,
		UnconfirmedTransactions: unconfirmedTxns,
	})
}

// walletTransactionsAddrHandler handles API calls to
// /wallet/transactions/:addr.
func (api *API) walletTransactionsAddrHandler(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	// Parse the address being input.
	jsonAddr := "\"" + ps.ByName("addr") + "\""
	var addr types.UnlockHash
	err := addr.UnmarshalJSON([]byte(jsonAddr))
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/transactions: " + err.Error()}, http.StatusBadRequest)
		return
	}

	confirmedATs, err := api.wallet.AddressTransactions(addr)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/transactions: " + err.Error()}, http.StatusBadRequest)
		return
	}
	unconfirmedATs, err := api.wallet.AddressUnconfirmedTransactions(addr)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/transactions: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletTransactionsGETaddr{
		ConfirmedTransactions:   confirmedATs,
		UnconfirmedTransactions: unconfirmedATs,
	})
}

// walletUnlockHandler handles API calls to /wallet/unlock.
func (api *API) walletUnlockHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	potentialKeys, _ := encryptionKeys(req.FormValue("encryptionpassword"))
	var err error
	for _, key := range potentialKeys {
		errChan := api.wallet.UnlockAsync(key)
		var unlockErr error
		select {
		case unlockErr = <-errChan:
		default:
		}
		if unlockErr == nil {
			WriteSuccess(w)
			return
		}
		err = errors.Compose(err, unlockErr)
	}
	WriteError(w, Error{"error when calling /wallet/unlock: " + err.Error()}, http.StatusBadRequest)
}

// walletChangePasswordHandler handles API calls to /wallet/changepassword
func (api *API) walletChangePasswordHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var newKey crypto.CipherKey
	newPassword := req.FormValue("newpassword")
	if newPassword == "" {
		WriteError(w, Error{"a password must be provided to newpassword"}, http.StatusBadRequest)
		return
	}
	newKey = crypto.NewWalletKey(crypto.HashObject(newPassword))

	originalKeys, seeds := encryptionKeys(req.FormValue("encryptionpassword"))
	var err error
	for _, key := range originalKeys {
		keyErr := api.wallet.ChangeKey(key, newKey)
		if keyErr == nil {
			WriteSuccess(w)
			return
		}
		err = errors.Compose(err, keyErr)
	}
	for _, seed := range seeds {
		seedErr := api.wallet.ChangeKeyWithSeed(seed, newKey)
		if seedErr == nil {
			WriteSuccess(w)
			return
		}
		err = errors.Compose(err, seedErr)
	}
	WriteError(w, Error{"error when calling /wallet/changepassword: " + err.Error()}, http.StatusBadRequest)
	return
}

// walletVerifyPasswordHandler handles API calls to /wallet/verifypassword
func (api *API) walletVerifyPasswordHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	originalKeys, _ := encryptionKeys(req.FormValue("password"))
	var err error
	for _, key := range originalKeys {
		valid, keyErr := api.wallet.IsMasterKey(key)
		if keyErr == nil {
			WriteJSON(w, WalletVerifyPasswordGET{
				Valid: valid,
			})
			return
		}
		err = errors.Compose(err, keyErr)
	}
	WriteError(w, Error{"error when calling /wallet/verifypassword: " + err.Error()}, http.StatusBadRequest)
}

// walletVerifyAddressHandler handles API calls to /wallet/verify/address/:addr.
func (api *API) walletVerifyAddressHandler(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	addrString := ps.ByName("addr")

	err := new(types.UnlockHash).LoadString(addrString)
	WriteJSON(w, WalletVerifyAddressGET{Valid: err == nil})
}

// walletUnlockConditionsHandlerGET handles GET calls to /wallet/unlockconditions.
func (api *API) walletUnlockConditionsHandlerGET(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	var addr types.UnlockHash
	err := addr.LoadString(ps.ByName("addr"))
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/unlockconditions: " + err.Error()}, http.StatusBadRequest)
		return
	}
	uc, err := api.wallet.UnlockConditions(addr)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/unlockconditions: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletUnlockConditionsGET{
		UnlockConditions: uc,
	})
}

// walletUnlockConditionsHandlerPOST handles POST calls to /wallet/unlockconditions.
func (api *API) walletUnlockConditionsHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var params WalletUnlockConditionsPOSTParams
	err := json.NewDecoder(req.Body).Decode(&params)
	if err != nil {
		WriteError(w, Error{"invalid parameters: " + err.Error()}, http.StatusBadRequest)
		return
	}
	err = api.wallet.AddUnlockConditions(params.UnlockConditions)
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/unlockconditions: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}

// walletUnspentHandler handles API calls to /wallet/unspent.
func (api *API) walletUnspentHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	outputs, err := api.wallet.UnspentOutputs()
	if err != nil {
		WriteError(w, Error{"error when calling /wallet/unspent: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	WriteJSON(w, WalletUnspentGET{
		Outputs: outputs,
	})
}

// walletSignHandler handles API calls to /wallet/sign.
func (api *API) walletSignHandler(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var params WalletSignPOSTParams
	err := json.NewDecoder(req.Body).Decode(&params)
	if err != nil {
		WriteError(w, Error{"invalid parameters: " + err.Error()}, http.StatusBadRequest)
		return
	}
	err = api.wallet.SignTransaction(&params.Transaction, params.ToSign)
	if err != nil {
		WriteError(w, Error{"failed to sign transaction: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletSignPOSTResp{
		Transaction: params.Transaction,
	})
}

// walletWatchHandlerGET handles GET calls to /wallet/watch.
func (api *API) walletWatchHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	addrs, err := api.wallet.WatchAddresses()
	if err != nil {
		WriteError(w, Error{"failed to get watch addresses: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteJSON(w, WalletWatchGET{
		Addresses: addrs,
	})
}

// walletWatchHandlerPOST handles POST calls to /wallet/watch.
func (api *API) walletWatchHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var wwpp WalletWatchPOST
	err := json.NewDecoder(req.Body).Decode(&wwpp)
	if err != nil {
		WriteError(w, Error{"invalid parameters: " + err.Error()}, http.StatusBadRequest)
		return
	}
	if wwpp.Remove {
		err = api.wallet.RemoveWatchAddresses(wwpp.Addresses, wwpp.Unused)
	} else {
		err = api.wallet.AddWatchAddresses(wwpp.Addresses, wwpp.Unused)
	}
	if err != nil {
		WriteError(w, Error{"failed to update watch set: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}
