package host

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/types"
)

func TestUnburn(t *testing.T) {
	ht, err := newMockHostTester(modules.ProdDependencies, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ht.Close(); err != nil {
			t.Fatal(err)
		}
	})

	encryptionKey := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)

	unburnwallet, err := wallet.New(ht.cs, ht.tpool, filepath.Join(ht.persistDir, modules.WalletDir+"unburnwallet"))
	require.NoError(t, err)

	unburnSeed, err := unburnwallet.Encrypt(encryptionKey)
	require.NoError(t, err)
	require.NoError(t, unburnwallet.Unlock(encryptionKey))

	// Make some of wallet's addresses UnburnAddressUnlockHash and activate Fork2022.
	addr, err := unburnwallet.NextAddress()
	require.NoError(t, err)
	originalUnburnAddressUnlockHash := types.UnburnAddressUnlockHash
	originalFork2022 := types.Fork2022
	types.UnburnAddressUnlockHash = addr.UnlockHash()
	types.Fork2022 = true
	defer func() {
		types.UnburnAddressUnlockHash = originalUnburnAddressUnlockHash
		types.Fork2022 = originalFork2022
	}()

	// Recreate the wallet with the same seed.
	unburnwallet2, err := wallet.New(ht.cs, ht.tpool, filepath.Join(ht.persistDir, modules.WalletDir+"unburnwallet2"))
	require.NoError(t, err)
	require.NoError(t, unburnwallet2.InitFromSeed(encryptionKey, unburnSeed))
	require.NoError(t, unburnwallet2.Unlock(encryptionKey))

	// Wait for the block to propagate to unburnwallet2.
	wantHeight := ht.cs.Height()
	for {
		gotHeight, err := unburnwallet2.Height()
		require.NoError(t, err)
		if gotHeight >= wantHeight {
			break
		}
		t.Logf("Waiting, height %d < %d", gotHeight, wantHeight)
		time.Sleep(time.Second)
	}

	balance1, _, _, err := unburnwallet2.ConfirmedBalance()
	require.NoError(t, err)
	t.Logf("Confirmed balance 1: %s", balance1)

	burntAmount := types.NewCurrency64(10000000000000000000).Mul(types.NewCurrency64(100000000))

	_, err = ht.wallet.SendSiacoins(burntAmount, types.BurnAddressUnlockHash)
	require.NoError(t, err)

	_, err = ht.miner.AddBlock()
	require.NoError(t, err)

	// Wait for the block to propagate to unburnwallet2.
	wantHeight = ht.cs.Height()
	for {
		gotHeight, err := unburnwallet2.Height()
		require.NoError(t, err)
		if gotHeight >= wantHeight {
			break
		}
		t.Logf("Waiting, height %d < %d", gotHeight, wantHeight)
		time.Sleep(time.Second)
	}

	balance2, _, _, err := unburnwallet2.ConfirmedBalance()
	require.NoError(t, err)
	t.Logf("Confirmed balance 2: %s", balance2)

	diff := balance2.Sub(balance1)
	require.Equal(t, burntAmount.String(), diff.String())
}
