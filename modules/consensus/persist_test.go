package consensus

import (
	"path/filepath"
	"testing"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
)

// TestSaveLoad populates a blockchain, saves it, loads it, and checks
// the consensus set hash before and after
func TestSaveLoad(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	cst, err := createConsensusSetTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer cst.Close()
	cst.testBlockSuite()
	oldHash := cst.cs.dbConsensusChecksum()
	cst.cs.Close()

	// Reassigning this will lose subscribers and such, but we
	// just want to call load and get a hash
	g, err := gateway.New("127.0.0.1:0", false, build.TempDir(modules.ConsensusDir, t.Name(), modules.GatewayDir))
	if err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(build.SiaTestingDir, modules.ConsensusDir, t.Name(), modules.ConsensusDir)
	var errChan <-chan error
	cst.cs, errChan = New(g, false, d)
	if err := <-errChan; err != nil {
		t.Fatal(err)
	}
	newHash := cst.cs.dbConsensusChecksum()
	if oldHash != newHash {
		t.Fatal("consensus set hash changed after load")
	}
}
