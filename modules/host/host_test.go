package host

import (
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	siasync "gitlab.com/scpcorp/ScPrime/sync"
	"gitlab.com/scpcorp/ScPrime/types"
)

// blankHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankHostTester(name string) (*hostTester, error) {
	return blankMockHostTester(modules.ProdDependencies, name)
}

// newHostTester creates a host tester with an initialized wallet and money in
// that wallet.
func newHostTester(name string) (*hostTester, error) {
	return newMockHostTester(modules.ProdDependencies, name)
}

// Close safely closes the hostTester. It panics if err != nil because there
// isn't a good way to errcheck when deferring a close.
func (ht *hostTester) Close() (err error) {
	if ht.host != nil {
		err = ht.host.Close()
	}
	err = errors.Compose(
		err,
		ht.miner.Close(),
		ht.wallet.Close(),
		ht.tpool.Close(),
		ht.cs.Close(),
		ht.gateway.Close(),
	)

	if err != nil {
		panic(err)
	}
	return nil
}

// renterHostPair is a helper struct that contains a secret key, symbolizing the
// renter, a host and the id of the file contract they share.
type renterHostPair struct {
	staticFCID     types.FileContractID
	staticRenterSK crypto.SecretKey
	staticRenterPK types.SiaPublicKey
	staticHT       *hostTester
}

// newRenterHostPair creates a new host tester and returns a renter host pair,
// this pair is a helper struct that contains both the host and renter,
// represented by its secret key. This helper will create a storage
// obligation emulating a file contract between them.
func newRenterHostPair(name string) (*renterHostPair, error) {
	return newCustomRenterHostPair(name, modules.ProdDependencies)
}

// newCustomRenterHostPair creates a new host tester and returns a renter host
// pair, this pair is a helper struct that contains both the host and renter,
// represented by its secret key. This helper will create a storage obligation
// emulating a file contract between them. It is custom as it allows passing a
// set of dependencies.
func newCustomRenterHostPair(name string, deps modules.Dependencies) (*renterHostPair, error) {
	// setup host
	ht, err := newMockHostTester(deps, name)
	if err != nil {
		return nil, err
	}
	return newRenterHostPairCustomHostTester(ht)
}

// newRenterHostPairCustomHostTester returns a renter host pair, this pair is a
// helper struct that contains both the host and renter, represented by its
// secret key. This helper will create a storage obligation emulating a file
// contract between them. This method requires the caller to pass a hostTester
// opposed to creating one, which allows setting up multiple renters which each
// have a contract with the one host.
func newRenterHostPairCustomHostTester(ht *hostTester) (*renterHostPair, error) {
	// create a renter key pair
	sk, pk := crypto.GenerateKeyPair()
	renterPK := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}

	// setup storage obligation (emulating a renter creating a contract)
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		return nil, errors.AddContext(err, "unable to make the new tester storage obligation")
	}
	so, err = ht.addNoOpRevision(so, renterPK)
	if err != nil {
		return nil, errors.AddContext(err, "unable to add noop revision")
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so, false)
	if err != nil {
		return nil, errors.AddContext(err, "unable to add the storage obligation")
	}
	ht.host.managedUnlockStorageObligation(so.id())

	pair := &renterHostPair{
		staticRenterSK: sk,
		staticRenterPK: renterPK,
		staticFCID:     so.id(),
		staticHT:       ht,
	}

	return pair, nil
}

// Close closes the underlying host tester.
func (p *renterHostPair) Close() error {
	return p.staticHT.Close()
}

// managedSign returns the renter's signature of the given revision
func (p *renterHostPair) managedSign(rev types.FileContractRevision) crypto.Signature {
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(rev.ParentID),
			CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
			PublicKeyIndex: 0,
		}},
	}
	hash := signedTxn.SigHash(0, p.staticHT.host.BlockHeight())
	return crypto.SignHash(hash, p.staticRenterSK)
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
	ht.host, err = NewCustomHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, "127.0.0.1:0", filepath.Join(ht.persistDir, modules.HostDir), nil, 5*time.Second)
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
	_, err = New(nil, ht.gateway, ht.tpool, ht.wallet, "127.0.0.1:0", hostDir, nil, 5*time.Second)
	if err != errNilCS {
		t.Fatal("could not trigger errNilCS")
	}

	_, err = New(ht.cs, nil, ht.tpool, ht.wallet, "127.0.0.1:0", hostDir, nil, 5*time.Second)
	if err != errNilGateway {
		t.Fatal("Could not trigger errNilGateay")
	}

	_, err = New(ht.cs, ht.gateway, nil, ht.wallet, "127.0.0.1:0", hostDir, nil, 5*time.Second)
	if err != errNilTpool {
		t.Fatal("could not trigger errNilTpool")
	}

	_, err = New(ht.cs, ht.gateway, ht.tpool, nil, "127.0.0.1:0", hostDir, nil, 5*time.Second)
	if err != errNilWallet {
		t.Fatal("Could not trigger errNilWallet")
	}
}

// TestRenterHostPair tests the newRenterHostPair constructor
func TestRenterHostPair(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	rhp, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	err = rhp.Close()
	if err != nil {
		t.Fatal(err)
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
	if settings.MaxDuration != modules.DefaultMaxDuration {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxDownloadBatchSize != uint64(modules.DefaultMaxDownloadBatchSize) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxReviseBatchSize != uint64(modules.DefaultMaxReviseBatchSize) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.NetAddress != "" {
		t.Error("settings retrieval did not return default value")
	}
	if settings.WindowSize != modules.DefaultWindowSize {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.Collateral.Equals(modules.DefaultCollateral) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.CollateralBudget.Equals(defaultCollateralBudget) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MaxCollateral.Equals(modules.DefaultMaxCollateral) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinContractPrice.Equals(modules.DefaultContractPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinDownloadBandwidthPrice.Equals(modules.DefaultDownloadBandwidthPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinStoragePrice.Equals(modules.DefaultStoragePrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinUploadBandwidthPrice.Equals(modules.DefaultUploadBandwidthPrice) {
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
	rebootHost, err := New(ht.cs, ht.gateway, ht.tpool, ht.wallet, "127.0.0.1:0", filepath.Join(ht.persistDir, modules.HostDir), nil, 5*time.Second)
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
