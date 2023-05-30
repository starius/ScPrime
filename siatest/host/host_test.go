package host

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/host"
	"gitlab.com/scpcorp/ScPrime/modules/host/contractmanager"
	"gitlab.com/scpcorp/ScPrime/node"
	"gitlab.com/scpcorp/ScPrime/node/api"
	"gitlab.com/scpcorp/ScPrime/node/api/client"
	"gitlab.com/scpcorp/ScPrime/siatest"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TestHostGetPubKey confirms that the pubkey is returned through the API
func TestHostGetPubKey(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Host
	testDir := hostTestDir(t.Name())

	// Create a new server
	hostParams := node.Host(testDir)
	testNode, err := siatest.NewCleanNode(hostParams)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testNode.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Call HostGet, confirm public key is not a blank key
	hg, err := testNode.HostGet()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(hg.PublicKey.Key, []byte{}) {
		t.Fatal("Host has empty pubkey key", hg.PublicKey.Key)
	}

	// Get host pubkey from the server and compare to the pubkey return through
	// the HostGet endpoint
	pk, err := testNode.HostPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if !pk.Equals(hg.PublicKey) {
		t.Log("HostGet PubKey:", hg.PublicKey)
		t.Log("Server PubKey:", pk)
		t.Fatal("Public Keys don't match")
	}
}

// TestHostAlertDiskTrouble verifies the host properly registers the disk
// trouble alert, and returns it through the alerts endpoint
func TestHostAlertDiskTrouble(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	groupParams := siatest.GroupParams{
		Miners: 1,
	}

	alert := modules.Alert{
		Cause:    "",
		Module:   "contractmanager",
		Msg:      contractmanager.AlertMSGHostDiskTrouble,
		Severity: modules.SeverityCritical,
	}

	testDir := hostTestDir(t.Name())
	tg, err := siatest.NewGroupFromTemplate(testDir, groupParams)
	if err != nil {
		t.Fatal("Failed to create group:", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Add a node which won't be able to form a contract due to disk trouble
	depDiskTrouble := dependencies.NewDependencyHostDiskTrouble()
	hostParams := node.Host(filepath.Join(testDir, "/host"))
	hostParams.StorageManagerDeps = depDiskTrouble
	nodes, err := tg.AddNodes(hostParams)
	if err != nil {
		t.Fatal(err)
	}
	h := nodes[0]

	// Add a storage folder and resize it - should trigger disk trouble
	sf := hostTestDir("/some/folder")
	err = h.HostStorageFoldersAddPost(sf, 1<<24)
	if err != nil {
		t.Fatal(err)
	}

	depDiskTrouble.Fail()
	_ = h.HostStorageFoldersResizePost(sf, 1<<23)

	// Test that host registered the alert.
	err = h.IsAlertRegistered(alert)
	if err != nil {
		t.Fatal(err)
	}

	// Test that host reload unregisters the alert
	err = tg.RestartNode(h)
	if err != nil {
		t.Fatal(err)
	}
	err = h.IsAlertUnregistered(alert)
	if err != nil {
		t.Fatal(err)
	}
}

// TestHostAlertInsufficientCollateral verifies the host properly registers the
// insufficient collateral alert, and returns it through the alerts endpoint
func TestHostAlertInsufficientCollateral(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	//t.Parallel()

	// Create a test group
	groupParams := siatest.GroupParams{
		Hosts:   2,
		Renters: 1,
		Miners:  1,
	}
	testDir := hostTestDir(t.Name())
	tg, err := siatest.NewGroupFromTemplate(testDir, groupParams)
	if err != nil {
		t.Fatal("Failed to create group:", err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Confirm contracts got created
	r := tg.Renters()[0]
	rc, err := r.RenterContractsGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.ActiveContracts) == 0 {
		t.Fatal("No contracts created")
	}

	// Nullify the host's collateral budget
	h := tg.Hosts()[0]
	hS, _ := h.HostGet()
	err = h.HostModifySettingPost(client.HostParamCollateralBudget, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Mine blocks to force contract renewal
	if err = siatest.RenewContractsByRenewWindow(r, tg); err != nil {
		t.Fatal(err)
	}

	// Test that host registered alert.
	alert := modules.Alert{
		Cause:    "",
		Module:   "host",
		Msg:      host.AlertMSGHostInsufficientCollateral,
		Severity: modules.SeverityWarning,
	}

	if err = h.IsAlertRegistered(alert); err != nil {
		t.Fatal(err)
	}

	// Reinstate the host's collateral budget
	err = h.HostModifySettingPost(client.HostParamCollateralBudget, hS.InternalSettings.CollateralBudget)
	if err != nil {
		t.Fatal(err)
	}

	// Test that host unregistered alert.
	if err = h.IsAlertUnregistered(alert); err != nil {
		t.Fatal(err)
	}
}

// TestHostBandwidth confirms that the host module is monitoring bandwidth
func TestHostBandwidth(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	gp := siatest.GroupParams{
		Hosts:   2,
		Renters: 0,
		Miners:  1,
	}
	tg, err := siatest.NewGroupFromTemplate(hostTestDir(t.Name()), gp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	hostNode := tg.Hosts()[0]

	hbw, err := hostNode.HostBandwidthGet()
	if err != nil {
		t.Fatal(err)
	}

	if hbw.Upload != 0 || hbw.Download != 0 {
		t.Fatal("Expected host to have no upload or download bandwidth")
	}

	if _, err := tg.AddNodes(node.RenterTemplate); err != nil {
		t.Fatal(err)
	}

	hbw, err = hostNode.HostBandwidthGet()
	if err != nil {
		t.Fatal(err)
	}

	if hbw.Upload == 0 || hbw.Download == 0 {
		t.Fatal("Expected host to use bandwidth from rpc with new renter node")
	}

	lastUpload := hbw.Upload
	lastDownload := hbw.Download
	renterNode := tg.Renters()[0]

	_, rf, err := renterNode.UploadNewFileBlocking(100, 1, 1, false)
	if err != nil {
		t.Fatal(err)
	}

	hbw, err = hostNode.HostBandwidthGet()
	if err != nil {
		t.Fatal(err)
	}

	if hbw.Upload <= lastUpload || hbw.Download <= lastDownload {
		t.Fatal("Expected host to use more bandwidth from uploaded file")
	}

	lastUpload = hbw.Upload
	lastDownload = hbw.Download

	if _, _, err := renterNode.DownloadToDisk(rf, false); err != nil {
		t.Fatal(err)
	}

	hbw, err = hostNode.HostBandwidthGet()
	if err != nil {
		t.Fatal(err)
	}

	if hbw.Upload <= lastUpload || hbw.Download <= lastDownload {
		t.Fatal("Expected host to use more bandwidth from downloaded file")
	}
}

// TestHostContract confirms that the host contract endpoint returns the expected values
func TestHostContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	//t.Parallel()

	gp := siatest.GroupParams{
		Hosts:   2,
		Renters: 1,
		Miners:  1,
	}
	tg, err := siatest.NewGroupFromTemplate(hostTestDir(t.Name()), gp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	hostNode := tg.Hosts()[0]
	renterNode := tg.Renters()[0]

	_, err = hostNode.HostContractGet(types.FileContractID{})
	if err == nil {
		t.Fatal("expected unknown obligation id to return error")
	}

	formed, err := hostNode.HostContractInfoGet()
	if err != nil {
		t.Fatal(err)
	}

	if len(formed.Contracts) == 0 {
		t.Fatal("expected renter to form contract")
	}

	contractID := formed.Contracts[0].ObligationId
	hcg, err := hostNode.HostContractGet(contractID)
	if err != nil {
		t.Fatal(err)
	}

	if hcg.Contract.ObligationId != contractID {
		t.Fatalf("returned contract id %s should match requested id %s", hcg.Contract.ObligationId, contractID)
	}

	if hcg.Contract.DataSize != 0 {
		t.Fatal("contract should have 0 datasize")
	}

	prevValidPayout := hcg.Contract.ValidProofOutputs[1].Value
	prevMissPayout := hcg.Contract.MissedProofOutputs[1].Value
	_, _, err = renterNode.UploadNewFileBlocking(int(modules.SectorSize), 1, 1, true)
	if err != nil {
		t.Fatal(err)
	}

	hcg, err = hostNode.HostContractGet(contractID)
	if err != nil {
		t.Fatal(err)
	}

	if hcg.Contract.DataSize != modules.SectorSize {
		t.Fatal("contract should have 1 sector uploaded")
	}

	// to avoid an NDF we do not compare the RevisionNumber to an exact number
	// because that is not deterministic due to the new RHP3 protocol, which
	// uses the contract to fund EAs do balance checks and so forth
	if hcg.Contract.RevisionNumber == 1 {
		t.Fatal("contract should have received more revisions from the upload", hcg.Contract.RevisionNumber)
	}

	if !hcg.Contract.PotentialAccountFunding.IsZero() {
		t.Fatal("contract shouldn't have account funding as EA is removed")
	}

	if hcg.Contract.PotentialUploadRevenue.IsZero() {
		t.Fatal("contract should have upload revenue")
	}

	if hcg.Contract.PotentialStorageRevenue.IsZero() {
		t.Fatal("contract should have storage revenue")
	}

	if hcg.Contract.ValidProofOutputs[1].Value.Cmp(prevValidPayout) != 1 {
		t.Fatal("valid payout should be greater than old valid payout")
	}

	newMissPayout := hcg.Contract.MissedProofOutputs[1].Value
	if cmp := newMissPayout.Cmp(prevMissPayout); cmp != -1 {
		t.Errorf("missed proof payout for provider %v should be less than previous missed proof payout %v after a getting data", newMissPayout, prevMissPayout)
	}
}

// TestHostContracts confirms that the host contracts endpoint returns the expected values
func TestHostContracts(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	gp := siatest.GroupParams{
		Hosts:   2,
		Renters: 0,
		Miners:  1,
	}
	tg, err := siatest.NewGroupFromTemplate(hostTestDir(t.Name()), gp)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	hostNode := tg.Hosts()[0]
	hc, err := hostNode.HostContractInfoGet()
	if err != nil {
		t.Fatal(err)
	}

	if len(hc.Contracts) != 0 {
		t.Fatal("expected host to have no contracts")
	}

	if _, err := tg.AddNodes(node.RenterTemplate); err != nil {
		t.Fatal(err)
	}

	renterNode := tg.Renters()[0]
	hc, err = hostNode.HostContractInfoGet()
	if err != nil {
		t.Fatal(err)
	}

	if len(hc.Contracts) == 0 {
		t.Fatal("expected host to have new contract")
	}

	if hc.Contracts[0].DataSize != 0 {
		t.Fatal("contract should have 0 datasize")
	}

	if hc.Contracts[0].RevisionNumber != 1 {
		t.Fatal("contract should have 1 revision")
	}

	prevValidPayout := hc.Contracts[0].ValidProofOutputs[1].Value
	prevMissPayout := hc.Contracts[0].MissedProofOutputs[1].Value
	_, _, err = renterNode.UploadNewFileBlocking(4096, 1, 1, true)
	if err != nil {
		t.Fatal(err)
	}

	hc, err = hostNode.HostContractInfoGet()
	if err != nil {
		t.Fatal(err)
	}
	if hc.Contracts[0].DataSize != 4096 {
		t.Fatal("contract should have 1 sector uploaded")
	}

	// to avoid an NDF we do not compare the RevisionNumber to an exact number
	// because that is not deterministic due to the new RHP3 protocol, which
	// uses the contract to fund EAs do balance checks and so forth
	if hc.Contracts[0].RevisionNumber == 1 {
		t.Fatal("contract should have received more revisions from the upload", hc.Contracts[0].RevisionNumber)
	}

	if !hc.Contracts[0].PotentialAccountFunding.IsZero() {
		t.Fatal("contract shouldn't have account funding as EA is removed")
	}

	if hc.Contracts[0].PotentialUploadRevenue.IsZero() {
		t.Fatal("contract should have upload revenue")
	}

	if hc.Contracts[0].PotentialStorageRevenue.IsZero() {
		t.Fatal("contract should have storage revenue")
	}

	if hc.Contracts[0].ValidProofOutputs[1].Value.Cmp(prevValidPayout) != 1 {
		t.Fatal("valid payout should be greater than old valid payout")
	}

	newMissPayout := hc.Contracts[0].MissedProofOutputs[1].Value
	if cmp := newMissPayout.Cmp(prevMissPayout); cmp != -1 {
		t.Errorf("missed proof payout for provider %v should be less than previous missed proof payout %v after a getting data", newMissPayout, prevMissPayout)
	}
}

// TestHostValidPrices confirms that the user can't set invalid prices through
// the API
func TestHostValidPrices(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create Host
	testDir := hostTestDir(t.Name())
	hostParams := node.Host(testDir)
	host, err := siatest.NewCleanNode(hostParams)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := host.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Get the Host
	hg, err := host.HostGet()
	if err != nil {
		t.Fatal(err)
	}

	// Verify that setting an invalid RPC price will return an error
	rpcPrice := hg.InternalSettings.MaxBaseRPCPrice().Mul64(modules.MaxBaseRPCPriceVsBandwidth)
	err = host.HostModifySettingPost(client.HostParamMinBaseRPCPrice, rpcPrice)
	if err == nil || !strings.Contains(err.Error(), api.ErrInvalidRPCDownloadRatio.Error()) {
		t.Fatalf("Expected Error %v but got %v", api.ErrInvalidRPCDownloadRatio, err)
	}

	// Verify that setting an invalid Sector price will return an error
	sectorPrice := hg.InternalSettings.MaxSectorAccessPrice().Mul64(modules.MaxSectorAccessPriceVsBandwidth)
	err = host.HostModifySettingPost(client.HostParamMinSectorAccessPrice, sectorPrice)
	if err == nil || !strings.Contains(err.Error(), api.ErrInvalidSectorAccessDownloadRatio.Error()) {
		t.Fatalf("Expected Error %v but got %v", api.ErrInvalidSectorAccessDownloadRatio, err)
	}

	// Verify that setting an invalid download price will return an error. Error
	// should be the RPC error since that is the first check
	downloadPrice := hg.InternalSettings.MinDownloadBandwidthPrice.Div64(modules.MaxBaseRPCPriceVsBandwidth)
	err = host.HostModifySettingPost(client.HostParamMinDownloadBandwidthPrice, downloadPrice)
	if err == nil || !strings.Contains(err.Error(), api.ErrInvalidRPCDownloadRatio.Error()) {
		t.Fatalf("Expected Error %v but got %v", api.ErrInvalidRPCDownloadRatio, err)
	}
}

// TestStorageProofEmptyContract tests that both empty contracts as well as
// not-empty contracts will result in storage proofs.
func TestStorageProofEmptyContract(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a testgroup.
	groupParams := siatest.GroupParams{
		Hosts:  2,
		Miners: 1,
	}
	groupDir := hostTestDir(t.Name())

	tg, err := siatest.NewGroupFromTemplate(groupDir, groupParams)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tg.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// Prevent contract renewals to make sure the revision number stays at 1.
	rt := node.RenterTemplate
	rt.HostAPIAddr = ":0"
	rt.ContractorDeps = &dependencies.DependencyDisableRenewal{}
	_, err = tg.AddNodeN(rt, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Fetch the renters.
	renters := tg.Renters()
	//	renterUpload := renters[0]
	renterDownload := renters[1]
	// Get the storage obligations from the hosts.
	hosts := tg.Hosts()
	host1, host2 := hosts[0], hosts[1]
	cig1, err := host1.HostContractInfoGet()
	if err != nil {
		t.Fatal(err)
	}
	cig2, err := host2.HostContractInfoGet()
	if err != nil {
		t.Fatal(err)
	}

	// There should be 2 contracts per host.
	contracts := append(cig1.Contracts, cig2.Contracts...)
	if len(contracts) != len(hosts)*len(renters) {
		t.Fatalf("expected %v contracts but got %v", len(hosts)*len(renters), len(contracts))
	}

	// Mine until the proof deadline that is furthest in the future.
	var proofDeadline types.BlockHeight
	for _, so := range contracts {
		if so.ProofDeadLine > proofDeadline {
			proofDeadline = so.ProofDeadLine
		}
	}
	bh, err := renterDownload.BlockHeight()
	if err != nil {
		t.Fatal(err)
	}
	for ; bh <= proofDeadline; bh++ {
		err = tg.Miners()[0].MineBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check that the right number of storage obligations were provided.
	retries := 0
	err = build.Retry(1000, 100*time.Millisecond, func() error {
		if retries%10 == 0 {
			err = tg.Miners()[0].MineBlock()
			if err != nil {
				t.Error(err)
				return nil
			}
		}
		retries++

		cig1, err = host1.HostContractInfoGet()
		if err != nil {
			return err
		}
		cig2, err = host2.HostContractInfoGet()
		if err != nil {
			return err
		}
		proofs := 0
		emptyContracts := 0
		for _, contract := range append(cig1.Contracts, cig2.Contracts...) {
			if contract.ProofConfirmed {
				proofs++
				if contract.DataSize == 0 {
					emptyContracts++
				}
			}
		}

		expectedProofs := len(contracts)
		expectedEmptyContracts := 2
		if proofs < expectedProofs {
			return fmt.Errorf("expected at least %v submitted proofs but got %v", expectedProofs, proofs)
		}
		if emptyContracts < expectedEmptyContracts {
			return fmt.Errorf("expected at least %v submitted empty proofs but got %v", expectedEmptyContracts, emptyContracts)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
