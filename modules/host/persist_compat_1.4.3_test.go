package host

import (
	"bytes"
	"path/filepath"

	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"

	"gitlab.com/NebulousLabs/errors"
)

// loadExistingHostWithNewDeps will create all of the dependencies for a host,
// then load the host on top of the given directory.
func loadExistingHostWithNewDeps(modulesDir, siaMuxDir, hostDir string) (modules.Host, error) {
	// Create the siamux
	mux, err := modules.NewSiaMux(siaMuxDir, modulesDir, "localhost:0")
	if err != nil {
		return nil, err
	}

	// Create the host dependencies.
	g, err := gateway.New("localhost:0", false, filepath.Join(modulesDir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, errChan := consensus.New(g, false, filepath.Join(modulesDir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(modulesDir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(modulesDir, modules.WalletDir))
	if err != nil {
		return nil, err
	}

	// Create the host.
	h, err := NewCustomHost(modules.ProdDependencies, cs, g, tp, w, mux, "localhost:0", hostDir)
	if err != nil {
		return nil, err
	}

	pubKey := mux.PublicKey()
	if !bytes.Equal(h.publicKey.Key, pubKey[:]) {
		return nil, errors.New("host and siamux pubkeys don't match")
	}
	privKey := mux.PrivateKey()
	if !bytes.Equal(h.secretKey[:], privKey[:]) {
		return nil, errors.New("host and siamux privkeys don't match")
	}
	return h, nil
}
