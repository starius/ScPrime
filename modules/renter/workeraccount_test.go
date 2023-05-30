package renter

import (
	"fmt"
	"strings"
	"testing"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/types"
)

// newRandomHostKey creates a random key pair and uses it to create a
// SiaPublicKey, this method returns the SiaPublicKey alongisde the secret key
func newRandomHostKey() (types.SiaPublicKey, crypto.SecretKey) {
	sk, pk := crypto.GenerateKeyPair()
	return types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}, sk
}

// TestAccount verifies the functionality of the account
func TestAccount(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	rt, err := newRenterTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		closedRenter := rt.renter
		if err := rt.Close(); err != nil {
			t.Error(err)
		}

		// these test are ran on a renter after Close has been called
		t.Run("Closed", func(t *testing.T) {
			testAccountClosed(t, closedRenter)
		})
		t.Run("CriticalOnDoubleSave", func(t *testing.T) {
			testAccountCriticalOnDoubleSave(t, closedRenter)
		})
	}()

	t.Run("Constants", testAccountConstants)
	t.Run("Creation", func(t *testing.T) { testAccountCreation(t, rt) })
}

// TestWorkerAccount verifies the functionality of the account related
// functionality of the worker.
func TestWorkerAccount(t *testing.T) {
	t.Skip("EA workers disabled")
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	wt, err := newWorkerTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err := wt.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
}

// testAccountConstants makes sure that certain relationships between constants
// exist.
func testAccountConstants(t *testing.T) {
	// Sanity check that the metadata size is not larger than the account size.
	if metadataSize > accountSize {
		t.Fatal("metadata size is larger than account size")
	}
	if accountSize > 4096 {
		t.Fatal("account size must not be larger than a disk sector")
	}
	if 4096%accountSize != 0 {
		t.Fatal("account size must be a factor of 4096")
	}
}

// testAccountCreation verifies newAccount returns a valid account object
func testAccountCreation(t *testing.T, rt *renterTester) {
	r := rt.renter

	// create a random hostKey
	_, pk := crypto.GenerateKeyPair()
	hostKey := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}

	// create an account with a different hostkey to ensure the account we are
	// going to validate has an offset different from 0
	tmpKey, _ := newRandomHostKey()
	_, err := r.staticAccountManager.managedOpenAccount(tmpKey)
	if err != nil {
		t.Fatal(err)
	}

	// create a new account object
	account, err := r.staticAccountManager.managedOpenAccount(hostKey)
	if err != nil {
		t.Fatal(err)
	}

	// validate the account object
	if account.staticID.IsZeroAccount() {
		t.Fatal("Invalid account ID")
	}
	if account.staticOffset == 0 {
		t.Fatal("Invalid offset")
	}
	if !account.staticHostKey.Equals(hostKey) {
		t.Fatal("Invalid host key")
	}

	// validate the account id is built using a valid SiaPublicKey and the
	// account's secret key belongs to the public key used to construct the id
	hash := crypto.HashBytes(fastrand.Bytes(10))
	sig := crypto.SignHash(hash, account.staticSecretKey)
	err = crypto.VerifyHash(hash, account.staticID.SPK().ToPublicKey(), sig)
	if err != nil {
		t.Fatal("Invalid secret key")
	}
}

// testAccountClosed verifies accounts can not be opened after the 'closed' flag
// has been set to true by the save.
func testAccountClosed(t *testing.T, closedRenter *Renter) {
	hk, _ := newRandomHostKey()
	_, err := closedRenter.staticAccountManager.managedOpenAccount(hk)
	if !strings.Contains(err.Error(), "file already closed") {
		t.Fatal("Unexpected error when opening an account, err:", err)
	}
}

// testAccountCriticalOnDoubleSave verifies the critical when
// managedSaveAccounts is called twice.
func testAccountCriticalOnDoubleSave(t *testing.T, closedRenter *Renter) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Sprint(r)
			if !strings.Contains(err, "Trying to save accounts twice") {
				t.Fatal("Expected error not returned")
			}
		}
	}()
	err := closedRenter.staticAccountManager.managedSaveAndClose()
	if err == nil {
		t.Fatal("Expected build.Critical on double save")
	}
}

// openRandomTestAccountsOnRenter is a helper function that creates a random
// number of accounts by calling 'managedOpenAccount' on the given renter
func openRandomTestAccountsOnRenter(r *Renter) ([]*account, error) {
	accounts := make([]*account, 0)
	for i := 0; i < fastrand.Intn(10)+1; i++ {
		hostKey := types.SiaPublicKey{
			Algorithm: types.SignatureEd25519,
			Key:       fastrand.Bytes(crypto.PublicKeySize),
		}
		account, err := r.staticAccountManager.managedOpenAccount(hostKey)
		if err != nil {
			return nil, err
		}

		// give it a random balance state
		account.balance = types.NewCurrency64(fastrand.Uint64n(1e3))
		account.negativeBalance = types.NewCurrency64(fastrand.Uint64n(1e2))
		account.pendingDeposits = types.NewCurrency64(fastrand.Uint64n(1e2))
		account.pendingWithdrawals = types.NewCurrency64(fastrand.Uint64n(1e2))
		accounts = append(accounts, account)
	}
	return accounts, nil
}
