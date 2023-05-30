package contractor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/ratelimit"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/renter/hostdb"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/types"
)

// Create a closeFn type that allows helpers which need to be closed to return
// methods that close the helpers.
type closeFn func() error

// tryClose is shorthand to run a t.Error() if a closeFn fails.
func tryClose(cf closeFn, t *testing.T) {
	err := cf()
	if err != nil {
		t.Error(err)
	}
}

// newModules initializes the modules needed to test creating a new contractor
func newModules(testdir string) (modules.ConsensusSet, modules.Wallet, modules.TransactionPool, modules.HostDB, closeFn, error) {
	g, err := gateway.New("127.0.0.1:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	cs, errChan := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, nil, nil, nil, nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	hdb, errChanHDB := hostdb.New(g, cs, tp, testdir)
	if err := <-errChanHDB; err != nil {
		return nil, nil, nil, nil, nil, err
	}
	cf := func() error {
		return errors.Compose(hdb.Close(), w.Close(), tp.Close(), cs.Close(), g.Close())
	}
	return cs, w, tp, hdb, cf, nil
}

// TestNew tests the New function.
func TestNew(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Create the modules.
	dir := build.TempDir("contractor", t.Name())
	cs, w, tpool, hdb, closeFn, err := newModules(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tryClose(closeFn, t)

	// Sane values.
	rl := ratelimit.NewRateLimit(0, 0, 0)
	_, errChan := New(cs, w, tpool, hdb, rl, dir)
	if err := <-errChan; err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Nil consensus set.
	_, errChan = New(nil, w, tpool, hdb, rl, dir)
	if err := <-errChan; err != errNilCS {
		t.Fatalf("expected %v, got %v", errNilCS, err)
	}

	// Nil wallet.
	_, errChan = New(cs, nil, tpool, hdb, rl, dir)
	if err := <-errChan; err != errNilWallet {
		t.Fatalf("expected %v, got %v", errNilWallet, err)
	}

	// Nil transaction pool.
	_, errChan = New(cs, w, nil, hdb, rl, dir)
	if err := <-errChan; err != errNilTpool {
		t.Fatalf("expected %v, got %v", errNilTpool, err)
	}
	// Nil hostdb.
	_, errChan = New(cs, w, tpool, nil, rl, dir)
	if err := <-errChan; err != errNilHDB {
		t.Fatalf("expected %v, got %v", errNilHDB, err)
	}

	// Bad persistDir.
	_, errChan = New(cs, w, tpool, hdb, rl, "")
	if err := <-errChan; !os.IsNotExist(err) {
		t.Fatalf("expected invalid directory, got %v", err)
	}
}

// TestAllowance tests the Allowance method.
func TestAllowance(t *testing.T) {
	c := &Contractor{
		allowance: modules.Allowance{
			Funds:  types.NewCurrency64(1),
			Period: 2,
			Hosts:  3,
		},
	}
	a := c.Allowance()
	if a.Funds.Cmp(c.allowance.Funds) != 0 ||
		a.Period != c.allowance.Period ||
		a.Hosts != c.allowance.Hosts {
		t.Fatal("Allowance did not return correct allowance:", a, c.allowance)
	}
}

// TestIntegrationSetAllowance tests the SetAllowance method.
func TestIntegrationSetAllowance(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create testing trio
	h, c, m, cf, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tryClose(cf, t)

	// this test requires two hosts: create another one
	h, hostCF, err := newTestingHost(build.TempDir("hostdata", ""), c.cs.(modules.ConsensusSet), c.tpool.(modules.TransactionPool))
	if err != nil {
		t.Fatal(err)
	}
	defer tryClose(hostCF, t)

	// announce the extra host
	err = h.Announce()
	if err != nil {
		t.Fatal(err)
	}

	// mine a block, processing the announcement
	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// wait for hostdb to scan
	hosts, err := c.hdb.RandomHosts(1, nil, nil)
	if err != nil {
		t.Fatal("failed to get hosts", err)
	}
	for i := 0; i < 100 && len(hosts) == 0; i++ {
		time.Sleep(time.Millisecond * 50)
	}

	// cancel allowance
	var a modules.Allowance
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}

	// bad args
	a.Hosts = 1
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroFunds {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroFunds, err)
	}
	a.Funds = types.ScPrimecoinPrecision
	a.Hosts = 0
	err = c.SetAllowance(a)
	if err != ErrAllowanceNoHosts {
		t.Errorf("expected %q, got %q", ErrAllowanceNoHosts, err)
	}
	a.Hosts = 1
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroPeriod {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroPeriod, err)
	}
	a.Period = 20
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroWindow {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroWindow, err)
	}
	// There should not be any errors related to RenewWindow size
	a.RenewWindow = 30
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroExpectedStorage {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroExpectedStorage, err)
	}
	a.RenewWindow = 20
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroExpectedStorage {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroExpectedStorage, err)
	}
	a.RenewWindow = 10
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroExpectedStorage {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroExpectedStorage, err)
	}
	a.ExpectedStorage = modules.DefaultAllowance.ExpectedStorage
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroExpectedUpload {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroExpectedUpload, err)
	}
	a.ExpectedUpload = modules.DefaultAllowance.ExpectedUpload
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroExpectedDownload {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroExpectedDownload, err)
	}
	a.ExpectedDownload = modules.DefaultAllowance.ExpectedDownload
	err = c.SetAllowance(a)
	if err != ErrAllowanceZeroExpectedRedundancy {
		t.Errorf("expected %q, got %q", ErrAllowanceZeroExpectedRedundancy, err)
	}
	a.ExpectedRedundancy = modules.DefaultAllowance.ExpectedRedundancy
	a.MaxPeriodChurn = modules.DefaultAllowance.MaxPeriodChurn

	// reasonable values; should succeed
	a.Funds = types.ScPrimecoinPrecision.Mul64(100)
	a.ExpectedStorage = 8192
	a.ExpectedUpload = 2048
	a.ExpectedDownload = 2048
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 250*time.Millisecond, func() error {
		if len(c.Contracts()) < 1 {
			message := fmt.Sprintf("allowance \n%+v\nforming seems to have failed\n hosts:\n%+v\n", a, hosts)
			return errors.New(message)
		}
		_, err = m.AddBlock()
		return err
	})
	if err != nil {
		t.Error(err)
	}

	// set same allowance; should no-op
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	clen := c.staticContracts.Len()
	if clen != 1 {
		t.Fatal("expected 1 contract, got", clen)
	}

	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// set allowance with Hosts = 2; should only form one new contract
	a.Hosts = 2
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 2 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// set allowance with Funds*2; should trigger renewal of both contracts
	a.Funds = a.Funds.Mul64(2)
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 2 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	// delete one of the contracts and set allowance with Funds*2; should
	// trigger 1 renewal and 1 new contract
	c.mu.Lock()
	ids := c.staticContracts.IDs()
	contract, _ := c.staticContracts.Acquire(ids[0])
	c.staticContracts.Delete(contract)
	c.mu.Unlock()
	a.Funds = a.Funds.Mul64(2)
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 2 {
			return errors.New("allowance forming seems to have failed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestHostMaxDuration tests that a host will not be used if their max duration
// is not sufficient when renewing contracts
func TestHostMaxDuration(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create testing trio
	h, c, m, cf, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tryClose(cf, t)

	// Set host's MaxDuration to 5 to test if host will be skipped when contract
	// is formed
	settings := h.InternalSettings()
	settings.MaxDuration = types.BlockHeight(5)
	if err := h.SetInternalSettings(settings); err != nil {
		t.Fatal(err)
	}
	// Let host settings permeate
	err = build.Retry(1000, 100*time.Millisecond, func() error {
		host, _, err := c.hdb.Host(h.PublicKey())
		if err != nil {
			return err
		}
		if settings.MaxDuration != host.MaxDuration {
			return fmt.Errorf("host max duration not set, expected %v, got %v", settings.MaxDuration, host.MaxDuration)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create allowance
	a := modules.Allowance{
		Funds:              types.ScPrimecoinPrecision.Mul64(100),
		Hosts:              1,
		Period:             30,
		RenewWindow:        20,
		ExpectedStorage:    modules.DefaultAllowance.ExpectedStorage,
		ExpectedUpload:     modules.DefaultAllowance.ExpectedUpload,
		ExpectedDownload:   modules.DefaultAllowance.ExpectedDownload,
		ExpectedRedundancy: modules.DefaultAllowance.ExpectedRedundancy,
		MaxPeriodChurn:     modules.DefaultAllowance.MaxPeriodChurn,
	}
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for and confirm no Contract creation
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) == 0 {
			return errors.New("no contract created")
		}
		return nil
	})
	if err == nil {
		t.Fatal("Contract should not have been created")
	}

	// Set host's MaxDuration to 50 to test if host will now form contract
	settings = h.InternalSettings()
	settings.MaxDuration = types.BlockHeight(50)
	if err := h.SetInternalSettings(settings); err != nil {
		t.Fatal(err)
	}
	// Let host settings permeate
	err = build.Retry(50, 100*time.Millisecond, func() error {
		host, _, err := c.hdb.Host(h.PublicKey())
		if err != nil {
			return err
		}
		if settings.MaxDuration != host.MaxDuration {
			return fmt.Errorf("host max duration not set, expected %v, got %v", settings.MaxDuration, host.MaxDuration)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	// Wait for Contract creation
	err = build.Retry(600, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 1 {
			return errors.New("no contract created")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	// Set host's MaxDuration to 5 to test if host will be skipped when contract
	// is renewed
	settings = h.InternalSettings()
	settings.MaxDuration = types.BlockHeight(5)
	if err := h.SetInternalSettings(settings); err != nil {
		t.Fatal(err)
	}
	// Let host settings permeate
	err = build.Retry(50, 100*time.Millisecond, func() error {
		host, _, err := c.hdb.Host(h.PublicKey())
		if err != nil {
			return err
		}
		if settings.MaxDuration != host.MaxDuration {
			return fmt.Errorf("host max duration not set, expected %v, got %v", settings.MaxDuration, host.MaxDuration)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Mine blocks to renew contract
	for i := types.BlockHeight(0); i <= c.allowance.Period-c.allowance.RenewWindow; i++ {
		_, err = m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Confirm Contract is not renewed
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.OldContracts()) == 0 {
			return errors.New("no contract renewed")
		}
		return nil
	})
	if err == nil {
		t.Fatal("Contract should not have been renewed")
	}
}

/* Remove EA
// TestPayment verifies the PaymentProvider interface on the contractor. It does
// this by trying to pay the host using a filecontract and verifying if payment
// can be made successfully.
func TestPayment(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// newStream is a helper to get a ready-to-use stream that is connected to a
	// host.
	newStream := func(mux *siamux.SiaMux, h modules.Host) (siamux.Stream, error) {
		hes := h.ExternalSettings()
		muxAddress := fmt.Sprintf("%s:%s", hes.NetAddress.Host(), hes.SiaMuxPort)
		muxPK := modules.SiaPKToMuxPK(h.PublicKey())
		return mux.NewStream(modules.HostSiaMuxSubscriberName, muxAddress, muxPK)
	}

	// create a siamux
	testdir := build.TempDir("contractor", t.Name())
	siaMuxDir := filepath.Join(testdir, modules.SiaMuxDir)
	mux, err := modules.NewSiaMux(siaMuxDir, testdir, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	// create a testing trio with our mux injected
	h, c, _, cf, err := newCustomTestingTrio(t.Name(), mux, modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer tryClose(cf, t)
	hpk := h.PublicKey()

	// set an allowance and wait for contracts
	err = c.SetAllowance(modules.DefaultAllowance)
	if err != nil {
		t.Fatal(err)
	}
	err = build.Retry(50, 100*time.Millisecond, func() error {
		if len(c.Contracts()) == 0 {
			return errors.New("no contract created")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// create a refund account
	aid, _ := modules.NewAccountID()

	// Fetch the contracts, there's a race condition between contract creation
	// and the contractor knowing the contract exists, so do this in a retry.
	var contract modules.RenterContract
	err = build.Retry(200, 100*time.Millisecond, func() error {
		var ok bool
		contract, ok = c.ContractByPublicKey(hpk)
		if !ok {
			return errors.New("contract not found")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// backup the amount renter funds
	initial := contract.RenterFunds

	// write the rpc id
	stream, err := newStream(mux, h)
	if err != nil {
		t.Fatal(err)
	}
	err = modules.RPCWrite(stream, modules.RPCUpdatePriceTable)
	if err != nil {
		t.Fatal(err)
	}

	// read the updated response
	var update modules.RPCUpdatePriceTableResponse
	err = modules.RPCRead(stream, &update)
	if err != nil {
		t.Fatal(err)
	}

	// unmarshal the JSON into a price table
	var pt modules.RPCPriceTable
	err = json.Unmarshal(update.PriceTableJSON, &pt)
	if err != nil {
		t.Fatal(err)
	}

	// provide payment
	err = c.ProvidePayment(stream, contract.HostPublicKey, modules.RPCUpdatePriceTable, pt.UpdatePriceTableCost, aid, c.blockHeight)
	if err != nil {
		t.Fatal(err)
	}

	// await the track response
	var tracked modules.RPCTrackedPriceTableResponse
	err = modules.RPCRead(stream, &tracked)
	if err != nil {
		t.Fatal(err)
	}

	// verify the contract was updated
	contract, _ = c.ContractByPublicKey(hpk)
	remaining := contract.RenterFunds
	expected := initial.Sub(pt.UpdatePriceTableCost)
	if !remaining.Equals(expected) {
		t.Fatalf("Expected renter contract to reflect the payment, the renter funds should be %v but were %v", expected.HumanString(), remaining.HumanString())
	}

	// prepare a buffer so we can optimize our writes
	buffer := bytes.NewBuffer(nil)

	// write the rpc id
	stream, err = newStream(mux, h)
	if err != nil {
		t.Fatal(err)
	}
	err = modules.RPCWrite(buffer, modules.RPCFundAccount)
	if err != nil {
		t.Fatal(err)
	}

	// write the price table uid
	err = modules.RPCWrite(buffer, pt.UID)
	if err != nil {
		t.Fatal(err)
	}

	// send fund account request (re-use the refund account)
	err = modules.RPCWrite(buffer, modules.FundAccountRequest{Account: aid})
	if err != nil {
		t.Fatal(err)
	}

	// write contents of the buffer to the stream
	_, err = stream.Write(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	// provide payment
	funding := remaining.Div64(2)
	if funding.Cmp(h.InternalSettings().MaxEphemeralAccountBalance) > 0 {
		funding = h.InternalSettings().MaxEphemeralAccountBalance
	}

	err = c.ProvidePayment(stream, hpk, modules.RPCFundAccount, funding.Add(pt.FundAccountCost), modules.ZeroAccountID, c.blockHeight)
	if err != nil {
		t.Fatal(err)
	}

	// receive response
	var resp modules.FundAccountResponse
	err = modules.RPCRead(stream, &resp)
	if err != nil {
		t.Fatal(err)
	}

	// verify the receipt
	receipt := resp.Receipt
	err = crypto.VerifyHash(crypto.HashAll(receipt), hpk.ToPublicKey(), resp.Signature)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Amount.Equals(funding) {
		t.Fatalf("Unexpected funded amount in the receipt, expected %v but received %v", funding.HumanString(), receipt.Amount.HumanString())
	}
	if receipt.Account != aid {
		t.Fatalf("Unexpected account id in the receipt, expected %v but received %v", aid, receipt.Account)
	}
	if !receipt.Host.Equals(hpk) {
		t.Fatalf("Unexpected host pubkey in the receipt, expected %v but received %v", hpk, receipt.Host)
	}
}

*/ //Remove EA

// TestLinkedContracts tests that the contractors maps are updated correctly
// when renewing contracts
func TestLinkedContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create testing trio
	h, c, m, cf, err := newTestingTrio(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer tryClose(cf, t)

	// Create allowance
	a := modules.Allowance{
		Funds:              types.ScPrimecoinPrecision.Mul64(100),
		Hosts:              1,
		Period:             20,
		RenewWindow:        10,
		ExpectedStorage:    modules.DefaultAllowance.ExpectedStorage,
		ExpectedUpload:     modules.DefaultAllowance.ExpectedUpload,
		ExpectedDownload:   modules.DefaultAllowance.ExpectedDownload,
		ExpectedRedundancy: modules.DefaultAllowance.ExpectedRedundancy,
		MaxPeriodChurn:     modules.DefaultAllowance.MaxPeriodChurn,
	}
	err = c.SetAllowance(a)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for Contract creation
	numRetries := 0
	err = build.Retry(200, 100*time.Millisecond, func() error {
		if numRetries%10 == 0 {
			if _, err := m.AddBlock(); err != nil {
				return err
			}
		}
		numRetries++
		if len(c.Contracts()) != 1 {
			return errors.New("no contract created")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	// Confirm that maps are empty
	if len(c.renewedFrom) != 0 {
		t.Fatal("renewedFrom map should be empty")
	}
	if len(c.renewedTo) != 0 {
		t.Fatal("renewedTo map should be empty")
	}

	// Set host's uploadbandwidthprice to zero to test divide by zero check when
	// contracts are renewed
	settings := h.InternalSettings()
	settings.MinUploadBandwidthPrice = types.ZeroCurrency
	if err := h.SetInternalSettings(settings); err != nil {
		t.Fatal(err)
	}

	// Mine blocks to renew contract
	for i := types.BlockHeight(0); i < c.allowance.Period-c.allowance.RenewWindow; i++ {
		_, err = m.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Confirm Contracts got renewed
	err = build.Retry(200, 100*time.Millisecond, func() error {
		if len(c.Contracts()) != 1 {
			return errors.New("no contract")
		}
		if len(c.OldContracts()) != 1 {
			return errors.New("no old contract")
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}

	// Confirm maps are updated as expected
	if len(c.renewedFrom) != 1 {
		t.Fatalf("renewedFrom map should have 1 entry but has %v", len(c.renewedFrom))
	}
	if len(c.renewedTo) != 1 {
		t.Fatalf("renewedTo map should have 1 entry but has %v", len(c.renewedTo))
	}
	if c.renewedFrom[c.Contracts()[0].ID] != c.OldContracts()[0].ID {
		t.Fatalf(`Map assignment incorrect,
		expected:
		map[%v:%v]
		got:
		%v`, c.Contracts()[0].ID, c.OldContracts()[0].ID, c.renewedFrom)
	}
	if c.renewedTo[c.OldContracts()[0].ID] != c.Contracts()[0].ID {
		t.Fatalf(`Map assignment incorrect,
		expected:
		map[%v:%v]
		got:
		%v`, c.OldContracts()[0].ID, c.Contracts()[0].ID, c.renewedTo)
	}
}

/* Remove EA
// TestPaymentMissingStorageObligation tests the case where a host can't find a
// storage obligation with which to pay.
func TestPaymentMissingStorageObligation(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a siamux
	testdir := build.TempDir("contractor", t.Name())
	siaMuxDir := filepath.Join(testdir, modules.SiaMuxDir)
	mux, err := modules.NewSiaMux(siaMuxDir, testdir, "127.0.0.1:0", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	// create a testing trio with our mux injected
	deps := &dependencies.DependencyStorageObligationNotFound{}
	h, c, _, cf, err := newCustomTestingTrio(t.Name(), mux, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer cf()
	hpk := h.PublicKey()

	// set an allowance and wait for contracts
	err = c.SetAllowance(modules.DefaultAllowance)
	if err != nil {
		t.Fatal(err)
	}

	// create a refund account
	aid, _ := modules.NewAccountID()

	// Fetch the contracts, there's a race condition between contract creation
	// and the contractor knowing the contract exists, so do this in a retry.
	var contract modules.RenterContract
	err = build.Retry(200, 100*time.Millisecond, func() error {
		var ok bool
		contract, ok = c.ContractByPublicKey(hpk)
		if !ok {
			return errors.New("contract not found")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// get a stream
	stream, err := newStream(mux, h)
	if err != nil {
		t.Fatal(err)
	}
	// write the rpc id
	err = modules.RPCWrite(stream, modules.RPCUpdatePriceTable)
	if err != nil {
		t.Fatal(err)
	}

	// read the updated response
	var update modules.RPCUpdatePriceTableResponse
	err = modules.RPCRead(stream, &update)
	if err != nil {
		t.Fatal(err)
	}

	// unmarshal the JSON into a price table
	var pt modules.RPCPriceTable
	err = json.Unmarshal(update.PriceTableJSON, &pt)
	if err != nil {
		t.Fatal(err)
	}

	// provide payment
	err = c.ProvidePayment(stream, contract.HostPublicKey, modules.RPCUpdatePriceTable, pt.UpdatePriceTableCost, aid, c.blockHeight)
	if err == nil || !strings.Contains(err.Error(), "storage obligation not found") {
		t.Fatal("expected storage obligation not found but got", err)
	}

	// verify the contract was updated
	contract, _ = c.ContractByPublicKey(hpk)
	if contract.Utility.GoodForRenew {
		t.Fatal("GFR should be false")
	}
	if contract.Utility.GoodForUpload {
		t.Fatal("GFU should be false")
	}
	if !contract.Utility.BadContract {
		t.Fatal("Contract should be bad")
	}
	if contract.Utility.Locked {
		t.Fatal("Contract should not be locked")
	}
}
*/ // Remove EA
