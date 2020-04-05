package host

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/miner"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	siasync "gitlab.com/scpcorp/ScPrime/sync"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/scpcorp/siamux"
)

// A hostTester is the helper object for host testing, including helper modules
// and methods for controlling synchronization.
type hostTester struct {
	mux *siamux.SiaMux

	cs        modules.ConsensusSet
	gateway   modules.Gateway
	miner     modules.TestMiner
	tpool     modules.TransactionPool
	wallet    modules.Wallet
	walletKey crypto.CipherKey

	host *Host

	persistDir string
}

/*
// initRenting prepares the host tester for uploads and downloads by announcing
// the host to the network and performing other preparational tasks.
// initRenting takes a while because the renter needs to process the host
// announcement, requiring asynchronous network communication between the
// renter and host.
func (ht *hostTester) initRenting() error {
	if ht.renting {
		return nil
	}

	// Because the renting test takes a long time, it will fail if
	// testing.Short.
	if testing.Short() {
		return errors.New("cannot call initRenting in short tests")
	}

	// Announce the host.
	err := ht.host.Announce()
	if err != nil {
		return err
	}

	// Mine a block to get the announcement into the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		return err
	}

	// Wait for the renter to see the host announcement.
	for i := 0; i < 50; i++ {
		time.Sleep(time.Millisecond * 100)
		if len(ht.renter.ActiveHosts()) != 0 {
			break
		}
	}
	if len(ht.renter.ActiveHosts()) == 0 {
		return errors.New("could not start renting in the host tester")
	}
	ht.renting = true
	return nil
}
*/

// initWallet creates a wallet key, initializes the host wallet, unlocks it,
// and then stores the key in the host tester.
func (ht *hostTester) initWallet() error {
	// Create the keys for the wallet and unlock it.
	key := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)
	ht.walletKey = key
	_, err := ht.wallet.Encrypt(key)
	if err != nil {
		return err
	}
	err = ht.wallet.Unlock(key)
	if err != nil {
		return err
	}
	return nil
}

// blankHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankHostTester(name string) (*hostTester, error) {
	return blankMockHostTester(modules.ProdDependencies, name)
}

// blankMockHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankMockHostTester(d modules.Dependencies, name string) (*hostTester, error) {
	testdir := build.TempDir(modules.HostDir, name)

	// Create the siamux.
	siaMuxDir := filepath.Join(testdir, modules.SiaMuxDir)
	mux, err := modules.NewSiaMux(siaMuxDir, testdir, "localhost:0")
	if err != nil {
		return nil, err
	}

	// Create the modules.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, errChan := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	h, err := NewCustomHost(d, cs, g, tp, w, mux, "localhost:0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}
	/*
		r, err := renter.New(cs, w, tp, filepath.Join(testdir, modules.RenterDir))
		if err != nil {
			return nil, err
		}
	*/

	// Assemble all objects into a hostTester
	ht := &hostTester{
		mux: mux,

		cs:      cs,
		gateway: g,
		miner:   m,
		// renter:  r,
		tpool:  tp,
		wallet: w,

		host: h,

		persistDir: testdir,
	}

	return ht, nil
}

// newHostTester creates a host tester with an initialized wallet and money in
// that wallet.
func newHostTester(name string) (*hostTester, error) {
	return newMockHostTester(modules.ProdDependencies, name)
}

// newMockHostTester creates a host tester with an initialized wallet and money
// in that wallet, using the dependencies provided.
func newMockHostTester(d modules.Dependencies, name string) (*hostTester, error) {
	// Create a blank host tester.
	ht, err := blankMockHostTester(d, name)
	if err != nil {
		return nil, err
	}

	// Initialize the wallet and mine blocks until the wallet has money.
	err = ht.initWallet()
	if err != nil {
		return nil, err
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}

	// Create two storage folder for the host, one the minimum size and one
	// twice the minimum size.
	storageFolderOne := filepath.Join(ht.persistDir, "hostTesterStorageFolderOne")
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		return nil, err
	}
	err = ht.host.AddStorageFolder(storageFolderOne, modules.SectorSize*64)
	if err != nil {
		return nil, err
	}
	storageFolderTwo := filepath.Join(ht.persistDir, "hostTesterStorageFolderTwo")
	err = os.Mkdir(storageFolderTwo, 0700)
	if err != nil {
		return nil, err
	}
	err = ht.host.AddStorageFolder(storageFolderTwo, modules.SectorSize*64*2)
	if err != nil {
		return nil, err
	}
	return ht, nil
}

// Close safely closes the hostTester. It panics if err != nil because there
// isn't a good way to errcheck when deferring a close.
func (ht *hostTester) Close() error {
	errs := []error{
		ht.host.Close(),
		ht.miner.Close(),
		ht.tpool.Close(),
		ht.cs.Close(),
		ht.gateway.Close(),
	}
	if err := build.JoinErrors(errs, "; "); err != nil {
		panic(err)
	}
	return nil
}

// renterHostPair is a helper struct that contains a secret key, symbolizing the
// renter, a host and the id of the file contract they share.
type renterHostPair struct {
	host   *Host
	renter crypto.SecretKey
	fcid   types.FileContractID
}

// newRenterHostPair creates a new host tester and returns a renter host pair,
// this pair is a helper struct that contains both the host and renter,
// represented by its secret key. This helper will create a storage
// obligation emulating a file contract between them.
func newRenterHostPair(name string) (*hostTester, *renterHostPair, error) {
	// setup host
	ht, err := newHostTester(name)
	if err != nil {
		return nil, nil, err
	}

	// create a renter key pair
	sk, pk := crypto.GenerateKeyPair()
	renterPK := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}

	// setup storage obligationn (emulating a renter creating a contract)
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		return nil, nil, err
	}
	so, err = ht.addNoOpRevision(so, renterPK)
	if err != nil {
		return nil, nil, err
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so, false)
	if err != nil {
		return nil, nil, err
	}
	ht.host.managedUnlockStorageObligation(so.id())

	pair := &renterHostPair{
		host:   ht.host,
		renter: sk,
		fcid:   so.id(),
	}
	return ht, pair, nil
}

// paymentRevision returns a new revision that transfer the given amount to the
// host. Returns the payment revision together with a signature signed by the
// pair's renter.
func (p *renterHostPair) paymentRevision(amount types.Currency) (types.FileContractRevision, crypto.Signature, error) {
	updated, err := p.host.managedGetStorageObligation(p.fcid)
	if err != nil {
		return types.FileContractRevision{}, crypto.Signature{}, err
	}

	recent, err := updated.recentRevision()
	if err != nil {
		return types.FileContractRevision{}, crypto.Signature{}, err
	}

	rev, err := recent.PaymentRevision(amount)
	if err != nil {
		return types.FileContractRevision{}, crypto.Signature{}, err
	}

	sig := revisionSignature(rev, p.host.BlockHeight(), p.renter)
	return rev, sig, nil
}

// TestHostInitialization checks that the host initializes to sensible default
// values.
func TestHostInitialization(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a blank host tester
	ht, err := blankHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// verify its initial block height is zero
	if ht.host.blockHeight != 0 {
		t.Fatal("host initialized to the wrong block height")
	}

	// verify its RPC price table was properly initialised
	ht.host.staticPriceTables.mu.RLock()
	defer ht.host.staticPriceTables.mu.RUnlock()
	if reflect.DeepEqual(ht.host.staticPriceTables.current, modules.RPCPriceTable{}) {
		t.Fatal("RPC price table wasn't initialized")
	}
	if ht.host.staticPriceTables.current.Expiry == 0 {
		t.Fatal("RPC price table was not properly initialised")
	}
}

// TestHostMultiClose checks that the host returns an error if Close is called
// multiple times on the host.
func TestHostMultiClose(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestHostMultiClose")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.Close()
	if err != siasync.ErrStopped {
		t.Fatal(err)
	}
	err = ht.host.Close()
	if err != siasync.ErrStopped {
		t.Fatal(err)
	}
	// Set ht.host to something non-nil - nil was returned because startup was
	// incomplete. If ht.host is nil at the end of the function, the ht.Close()
	// operation will fail.
	ht.host, err = NewCustomHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
}

// TestNilValues tries initializing the host with nil values.
func TestNilValues(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestStartupRescan")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	hostDir := filepath.Join(ht.persistDir, modules.HostDir)
	_, err = New(nil, ht.gateway, ht.tpool, ht.wallet, ht.mux, "localhost:0", hostDir)
	if err != errNilCS {
		t.Fatal("could not trigger errNilCS")
	}
	_, err = New(ht.cs, nil, ht.tpool, ht.wallet, ht.mux, "localhost:0", hostDir)
	if err != errNilGateway {
		t.Fatal("Could not trigger errNilGateay")
	}
	_, err = New(ht.cs, ht.gateway, nil, ht.wallet, ht.mux, "localhost:0", hostDir)
	if err != errNilTpool {
		t.Fatal("could not trigger errNilTpool")
	}
	_, err = New(ht.cs, ht.gateway, ht.tpool, nil, ht.mux, "localhost:0", hostDir)
	if err != errNilWallet {
		t.Fatal("Could not trigger errNilWallet")
	}
}

// TestSetAndGetInternalSettings checks that the functions for interacting with
// the host's internal settings object are working as expected.
func TestSetAndGetInternalSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ht, err := newHostTester("TestSetAndGetInternalSettings")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Check the default settings get returned at first call.
	settings := ht.host.InternalSettings()
	if settings.AcceptingContracts != false {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxDuration != defaultMaxDuration {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxDownloadBatchSize != uint64(defaultMaxDownloadBatchSize) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxReviseBatchSize != uint64(defaultMaxReviseBatchSize) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.NetAddress != "" {
		t.Error("settings retrieval did not return default value")
	}
	if settings.WindowSize != defaultWindowSize {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.Collateral.Equals(defaultCollateral) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.CollateralBudget.Equals(defaultCollateralBudget) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MaxCollateral.Equals(defaultMaxCollateral) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinContractPrice.Equals(defaultContractPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinDownloadBandwidthPrice.Equals(defaultDownloadBandwidthPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinStoragePrice.Equals(defaultStoragePrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinUploadBandwidthPrice.Equals(defaultUploadBandwidthPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.EphemeralAccountExpiry != (defaultEphemeralAccountExpiry) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MaxEphemeralAccountBalance.Equals(defaultMaxEphemeralAccountBalance) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MaxEphemeralAccountRisk.Equals(defaultMaxEphemeralAccountRisk) {
		t.Error("settings retrieval did not return default value")
	}

	// Check that calling SetInternalSettings with valid settings updates the settings.
	settings.AcceptingContracts = true
	settings.NetAddress = "foo.com:123"
	err = ht.host.SetInternalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	settings = ht.host.InternalSettings()
	if settings.AcceptingContracts != true {
		t.Fatal("SetInternalSettings failed to update settings")
	}
	if settings.NetAddress != "foo.com:123" {
		t.Fatal("SetInternalSettings failed to update settings")
	}

	// Check that calling SetInternalSettings with invalid settings does not update the settings.
	settings.NetAddress = "invalid"
	err = ht.host.SetInternalSettings(settings)
	if err == nil {
		t.Fatal("expected SetInternalSettings to error with invalid settings")
	}
	settings = ht.host.InternalSettings()
	if settings.NetAddress != "foo.com:123" {
		t.Fatal("SetInternalSettings should not modify the settings if the new settings are invalid")
	}

	// Reload the host and verify that the altered settings persisted.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	rebootHost, err := New(ht.cs, ht.gateway, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	rebootSettings := rebootHost.InternalSettings()
	if rebootSettings.AcceptingContracts != settings.AcceptingContracts {
		t.Error("settings retrieval did not return updated value")
	}
	if rebootSettings.NetAddress != settings.NetAddress {
		t.Error("settings retrieval did not return updated value")
	}

	// Set ht.host to 'rebootHost' so that the 'ht.Close()' method will close
	// everything cleanly.
	ht.host = rebootHost
}

/*
// TestSetAndGetSettings checks that the functions for interacting with the
// hosts settings object are working as expected.
func TestSetAndGetSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestSetAndGetSettings")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Check the default settings get returned at first call.
	settings := ht.host.Settings()
	if settings.MaxDuration != defaultMaxDuration {
		t.Error("settings retrieval did not return default value")
	}
	if settings.WindowSize != defaultWindowSize {
		t.Error("settings retrieval did not return default value")
	}
	if settings.Price.Cmp(defaultPrice) != 0 {
		t.Error("settings retrieval did not return default value")
	}
	if settings.Collateral.Cmp(defaultCollateral) != 0 {
		t.Error("settings retrieval did not return default value")
	}

	// Submit updated settings and check that the changes stuck.
	settings.TotalStorage += 15
	settings.MaxDuration += 16
	settings.WindowSize += 17
	settings.Price = settings.Price.Add(types.NewCurrency64(18))
	settings.Collateral = settings.Collateral.Add(types.NewCurrency64(19))
	err = ht.host.SetSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	newSettings := ht.host.Settings()
	if settings.MaxDuration != newSettings.MaxDuration {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.WindowSize != newSettings.WindowSize {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Price.Cmp(newSettings.Price) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Collateral.Cmp(newSettings.Collateral) != 0 {
		t.Error("settings retrieval did not return updated value")
	}

	// Reload the host and verify that the altered settings persisted.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	rebootSettings := rebootHost.Settings()
	if settings.TotalStorage != rebootSettings.TotalStorage {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.MaxDuration != rebootSettings.MaxDuration {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.WindowSize != rebootSettings.WindowSize {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Price.Cmp(rebootSettings.Price) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Collateral.Cmp(rebootSettings.Collateral) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
}

// TestPersistentSettings checks that settings persist between instances of the
// host.
func TestPersistentSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestSetPersistentSettings")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Submit updated settings.
	settings := ht.host.Settings()
	settings.TotalStorage += 25
	settings.MaxDuration += 36
	settings.WindowSize += 47
	settings.Price = settings.Price.Add(types.NewCurrency64(38))
	settings.Collateral = settings.Collateral.Add(types.NewCurrency64(99))
	err = ht.host.SetSettings(settings)
	if err != nil {
		t.Fatal(err)
	}

	// Reboot the host and verify that the new settings stuck.
	err = ht.host.Close() // host saves upon closing
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(ht.cs, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	newSettings := h.Settings()
	if settings.TotalStorage != newSettings.TotalStorage {
		t.Error("settings retrieval did not return updated value:", settings.TotalStorage, "vs", newSettings.TotalStorage)
	}
	if settings.MaxDuration != newSettings.MaxDuration {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.WindowSize != newSettings.WindowSize {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Price.Cmp(newSettings.Price) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Collateral.Cmp(newSettings.Collateral) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
}
*/
