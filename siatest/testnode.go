package siatest

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/node"
	"gitlab.com/scpcorp/ScPrime/node/api/client"
	"gitlab.com/scpcorp/ScPrime/node/api/server"
	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/types"
)

var (
	// testNodeAddressCounter is a global variable that tracks the counter for
	// the test node addresses for all tests. It starts at 127.1.0.0 and
	// iterates through the entire range of 127.X.X.X above 127.1.0.0
	testNodeAddressCounter = newNodeAddressCounter()
)

type (
	// TestNode is a helper struct for testing that contains a server and a
	// client as embedded fields.
	TestNode struct {
		*server.Server
		client.Client
		params      node.NodeParams
		primarySeed string

		downloadDir *LocalDir
		filesDir    *LocalDir
	}

	// addressCounter is a help struct for assigning new addresses to test nodes
	addressCounter struct {
		address net.IP
		mu      sync.Mutex
	}
)

// newNodeAddressCounter creates a new address counter and returns it. The
// counter will cover the entire range 127.X.X.X above 127.1.0.0
//
// The counter is initialized with an IP address of 127.1.0.0 so that testers
// can manually add nodes in the address range of 127.0.X.X without causing a
// conflict
func newNodeAddressCounter() *addressCounter {
	counter := &addressCounter{
		address: net.IPv4(127, 1, 0, 0),
	}
	return counter
}

// managedNextNodeAddress returns the next node address from the addressCounter
func (ac *addressCounter) managedNextNodeAddress() (string, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	// IPs are byte slices with a length of 16, the IPv4 address range is stored
	// in the last 4 indexes, 12-15
	for i := len(ac.address) - 1; i >= 12; i-- {
		if i == 12 {
			return "", errors.New("ran out of IP addresses")
		}
		ac.address[i]++
		if ac.address[i] > 0 {
			break
		}
	}

	// If mac return 127.0.0.1
	if runtime.GOOS == "darwin" {
		return "127.0.0.1", nil
	}

	return ac.address.String(), nil
}

// NewNode creates a new funded TestNode
func NewNode(nodeParams node.NodeParams) (*TestNode, error) {
	// We can't create a funded node without a miner
	if !nodeParams.CreateMiner && nodeParams.Miner == nil {
		return nil, errors.New("Can't create funded node without miner")
	}
	// Create clean node
	tn, err := NewCleanNodeAsync(nodeParams)
	if err != nil {
		return nil, fmt.Errorf("error creating new clean node, %w", err)
	}
	// Fund the node
	for i := types.BlockHeight(0); i <= types.MaturityDelay+types.TaxHardforkHeight; i++ {
		if err := tn.MineBlock(); err != nil {
			return nil, err
		}
	}
	// Return TestNode
	return tn, nil
}

// NewCleanNode creates a new TestNode that's not yet funded
func NewCleanNode(nodeParams node.NodeParams) (*TestNode, error) {
	return newCleanNode(nodeParams, false)
}

// NewCleanNodeAsync creates a new TestNode that's not yet funded
func NewCleanNodeAsync(nodeParams node.NodeParams) (*TestNode, error) {
	return newCleanNode(nodeParams, true)
}

// newCleanNode creates a new TestNode that's not yet funded
func newCleanNode(nodeParams node.NodeParams, asyncSync bool) (*TestNode, error) {
	userAgent := "ScPrime-Agent"
	password := "password"

	nodeParams.HostAPIAddr = "127.0.0.1:0"

	// Check if an RPC address is set
	if nodeParams.RPCAddress == "" {
		addr, err := testNodeAddressCounter.managedNextNodeAddress()
		if err != nil {
			return nil, fmt.Errorf("error getting next node address: %w", err)
		}
		nodeParams.RPCAddress = addr + ":0"
	}

	nodeParams.CheckTokenExpirationFrequency = 5 * time.Second

	// Create server
	var s *server.Server
	var err error
	if asyncSync {
		var errChan <-chan error
		s, errChan = server.NewAsync("127.0.0.1:0", userAgent, password, nodeParams, time.Now())
		e := modules.PeekErr(errChan)
		if e != nil {
			err = fmt.Errorf("error creating NewAsync server: %w", e)
		}
	} else {
		s, err = server.New("127.0.0.1:0", userAgent, password, nodeParams, time.Now())
		if err != nil {
			err = fmt.Errorf("error creating new server: %w", err)
		}
	}
	if err != nil {
		return nil, err
	}

	// Create client
	opts := client.Options{
		Address:   s.APIAddress(),
		Password:  password,
		UserAgent: userAgent,
	}
	c := client.New(opts)

	// Create TestNode
	tn := &TestNode{
		Server:      s,
		Client:      *c,
		params:      nodeParams,
		primarySeed: "",
	}
	if err = tn.initRootDirs(); err != nil {
		return nil, fmt.Errorf("failed to create root directories: %w", err)
	}

	// If there is no wallet we are done.
	if !nodeParams.CreateWallet && nodeParams.Wallet == nil {
		return tn, nil
	}

	// If the SkipWalletInit flag is set then we are done
	if nodeParams.SkipWalletInit {
		return tn, nil
	}

	// Init wallet
	if nodeParams.PrimarySeed != "" {
		err := tn.WalletInitSeedPost(nodeParams.PrimarySeed, "", false)
		if err != nil {
			return nil, err
		}
		tn.primarySeed = nodeParams.PrimarySeed
	} else {
		wip, err := tn.WalletInitPost("", false)
		if err != nil {
			return nil, err
		}
		tn.primarySeed = wip.PrimarySeed
	}

	// Unlock wallet
	if err := tn.WalletUnlockPost(tn.primarySeed); err != nil {
		return nil, err
	}

	// Return TestNode
	return tn, nil
}

// IsAlertRegistered returns an error if the given alert is not found
func (tn *TestNode) IsAlertRegistered(a modules.Alert) error {
	return build.Retry(10, 100*time.Millisecond, func() error {
		dag, err := tn.DaemonAlertsGet()
		if err != nil {
			return err
		}
		for _, alert := range dag.Alerts {
			if alert.Equals(a) {
				return nil
			}
		}
		return errors.New("alert is not registered")
	})
}

// IsAlertUnregistered returns an error if the given alert is still found
func (tn *TestNode) IsAlertUnregistered(a modules.Alert) error {
	return build.Retry(10, 100*time.Millisecond, func() error {
		dag, err := tn.DaemonAlertsGet()
		if err != nil {
			return err
		}

		for _, alert := range dag.Alerts {
			if alert.Equals(a) {
				return errors.New("alert is registered")
			}
		}
		return nil
	})
}

// PrintDebugInfo prints out helpful debug information when debug tests and ndfs, the
// boolean arguments dictate what is printed
func (tn *TestNode) PrintDebugInfo(t *testing.T, contractInfo, hostInfo, renterInfo bool) {
	if contractInfo {
		rc, err := tn.RenterAllContractsGet()
		if err != nil {
			t.Log(err)
		}
		t.Log("Active Contracts")
		for _, c := range rc.ActiveContracts {
			t.Log("    ID", c.ID)
			t.Log("    HostPublicKey", c.HostPublicKey)
			t.Log("    GoodForUpload", c.GoodForUpload)
			t.Log("    GoodForRenew", c.GoodForRenew)
			t.Log("    EndHeight", c.EndHeight)
			t.Log("    Size", c.Size)
		}
		t.Log()
		t.Log("Passive Contracts")
		for _, c := range rc.PassiveContracts {
			t.Log("    ID", c.ID)
			t.Log("    HostPublicKey", c.HostPublicKey)
			t.Log("    GoodForUpload", c.GoodForUpload)
			t.Log("    GoodForRenew", c.GoodForRenew)
			t.Log("    EndHeight", c.EndHeight)
			t.Log("    Size", c.Size)
		}
		t.Log()
		t.Log("Refreshed Contracts")
		for _, c := range rc.RefreshedContracts {
			t.Log("    ID", c.ID)
			t.Log("    HostPublicKey", c.HostPublicKey)
			t.Log("    GoodForUpload", c.GoodForUpload)
			t.Log("    GoodForRenew", c.GoodForRenew)
			t.Log("    EndHeight", c.EndHeight)
			t.Log("    Size", c.Size)
		}
		t.Log()
		t.Log("Disabled Contracts")
		for _, c := range rc.DisabledContracts {
			t.Log("    ID", c.ID)
			t.Log("    HostPublicKey", c.HostPublicKey)
			t.Log("    GoodForUpload", c.GoodForUpload)
			t.Log("    GoodForRenew", c.GoodForRenew)
			t.Log("    EndHeight", c.EndHeight)
			t.Log("    Size", c.Size)
		}
		t.Log()
		t.Log("Expired Contracts")
		for _, c := range rc.ExpiredContracts {
			t.Log("    ID", c.ID)
			t.Log("    HostPublicKey", c.HostPublicKey)
			t.Log("    GoodForUpload", c.GoodForUpload)
			t.Log("    GoodForRenew", c.GoodForRenew)
			t.Log("    EndHeight", c.EndHeight)
			t.Log("    Size", c.Size)
		}
		t.Log()
		t.Log("Expired Refreshed Contracts")
		for _, c := range rc.ExpiredRefreshedContracts {
			t.Log("    ID", c.ID)
			t.Log("    HostPublicKey", c.HostPublicKey)
			t.Log("    GoodForUpload", c.GoodForUpload)
			t.Log("    GoodForRenew", c.GoodForRenew)
			t.Log("    EndHeight", c.EndHeight)
			t.Log("    Size", c.Size)
		}
		t.Log()
	}

	if hostInfo {
		hdbag, err := tn.HostDbAllGet()
		if err != nil {
			t.Log(err)
		}
		t.Log("Active Hosts from HostDB")
		for _, host := range hdbag.Hosts {
			hostInfo, err := tn.HostDbHostsGet(host.PublicKey)
			if err != nil {
				t.Log(err)
			}
			t.Log("    Host:", host.NetAddress)
			t.Log("        score", hostInfo.ScoreBreakdown.Score)
			t.Log("        breakdown", hostInfo.ScoreBreakdown)
			t.Log("        pk", host.PublicKey)
			t.Log("        Accepting Contracts", host.HostExternalSettings.AcceptingContracts)
			t.Log("        Filtered", host.Filtered)
			t.Log("        LastIPNetChange", host.LastIPNetChange.String())
			t.Log("        Subnets")
			for _, subnet := range host.IPNets {
				t.Log("            ", subnet)
			}
			t.Log()
			t.Log("        TotalStorage", host.TotalStorage)
			t.Log("        RemainingStorage", host.RemainingStorage)
			t.Log("        StoragePrice", host.StoragePrice)
			t.Log("        UploadPrice", host.UploadBandwidthPrice)
			t.Log()
		}
		t.Log()
	}

	if renterInfo {
		t.Log("Renter Info")
		rg, err := tn.RenterGet()
		if err != nil {
			t.Log(err)
		}
		t.Log("CP:", rg.CurrentPeriod)
		cg, err := tn.ConsensusGet()
		if err != nil {
			t.Log(err)
		}
		t.Log("BH:", cg.Height)
		settings := rg.Settings
		t.Log("Allowance Funds:", settings.Allowance.Funds.HumanString())
		fm := rg.FinancialMetrics
		t.Log("Unspent Funds:", fm.Unspent.HumanString())
		t.Log()
	}
}

// RestartNode restarts a TestNode
func (tn *TestNode) RestartNode() error {
	err := tn.StopNode()
	if err != nil {
		return fmt.Errorf("error stopping node: %w", err)
	}
	err = tn.StartNode()
	if err != nil {
		return fmt.Errorf("error starting node: %w", err)
	}
	return nil
}

// SiaPath returns the siapath of a local file or directory to be used for
// uploading
func (tn *TestNode) SiaPath(path string) modules.SiaPath {
	s := strings.TrimPrefix(path, tn.filesDir.path+string(filepath.Separator))
	sp, err := modules.NewSiaPath(s)
	if err != nil {
		build.Critical("This shouldn't happen", err)
	}
	return sp
}

// StartNode starts a TestNode from an active group
func (tn *TestNode) StartNode() error {
	// Create server
	s, err := server.New(":0", tn.UserAgent, tn.Password, tn.params, time.Now())
	if err != nil {
		return fmt.Errorf("error starting testnode: %w", err)
	}
	tn.Server = s
	tn.Client.Address = s.APIAddress()
	if !tn.params.CreateWallet && tn.params.Wallet == nil {
		return nil
	}
	return tn.WalletUnlockPost(tn.primarySeed)
}

// StartNodeCleanDeps restarts a node from an active group without its
// previously assigned dependencies.
func (tn *TestNode) StartNodeCleanDeps() error {
	tn.params.ConsensusSetDeps = nil
	tn.params.ContractorDeps = nil
	tn.params.ContractSetDeps = nil
	tn.params.GatewayDeps = nil
	tn.params.HostDeps = nil
	tn.params.HostDBDeps = nil
	tn.params.RenterDeps = nil
	tn.params.TPoolDeps = nil
	tn.params.WalletDeps = nil
	return tn.StartNode()
}

// StopNode stops a TestNode
func (tn *TestNode) StopNode() error {
	if err := tn.Close(); err != nil {
		return fmt.Errorf("failed to stop node: %w", err)
	}
	return nil
}

// initRootDirs creates the download and upload directories for the TestNode
func (tn *TestNode) initRootDirs() error {
	tn.downloadDir = &LocalDir{
		path: filepath.Join(tn.RenterDir(), "downloads"),
	}
	if err := os.MkdirAll(tn.downloadDir.path, persist.DefaultDiskPermissionsTest); err != nil {
		return err
	}
	tn.filesDir = &LocalDir{
		path: filepath.Join(tn.RenterDir(), "uploads"),
	}
	if err := os.MkdirAll(tn.filesDir.path, persist.DefaultDiskPermissionsTest); err != nil {
		return err
	}
	return nil
}
