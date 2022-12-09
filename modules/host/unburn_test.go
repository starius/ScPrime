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
	cases := []struct {
		name         string
		sendAddress  types.UnlockHash
		spendAddress *types.UnlockHash
		wantBalance  string
	}{
		{
			name:         "unburn",
			sendAddress:  types.BurnAddressUnlockHash,
			spendAddress: &types.UnburnAddressUnlockHash,
			wantBalance:  "301000000000000000000000000000",
		},
		{
			name:         "ungift",
			sendAddress:  types.AirdropNebulousLabsUnlockHash,
			spendAddress: &types.UngiftUnlockHash,
			wantBalance:  "300001000000000000000000000000000",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ht, err := newMockHostTester(modules.ProdDependencies, t.Name())
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, ht.Close())
			})

			encryptionKey := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)

			wallet1, err := wallet.New(ht.cs, ht.tpool, filepath.Join(ht.persistDir, modules.WalletDir+"wallet1"))
			require.NoError(t, err)

			seed, err := wallet1.Encrypt(encryptionKey)
			require.NoError(t, err)
			require.NoError(t, wallet1.Unlock(encryptionKey))

			// Make some of wallet's addresses spendAddress and activate Fork2022.
			addr, err := wallet1.NextAddress()
			require.NoError(t, err)
			originalSpendAddress := *tc.spendAddress
			originalFork2022 := types.Fork2022
			*tc.spendAddress = addr.UnlockHash()
			types.Fork2022 = true
			defer func() {
				*tc.spendAddress = originalSpendAddress
				types.Fork2022 = originalFork2022
			}()

			// Recreate the wallet with the same seed.
			wallet2, err := wallet.New(ht.cs, ht.tpool, filepath.Join(ht.persistDir, modules.WalletDir+"wallet2"))
			require.NoError(t, err)
			require.NoError(t, wallet2.InitFromSeed(encryptionKey, seed))
			require.NoError(t, wallet2.Unlock(encryptionKey))

			// Wait for the block to propagate to wallet2.
			wantHeight := ht.cs.Height()
			for {
				gotHeight, err := wallet2.Height()
				require.NoError(t, err)
				if gotHeight >= wantHeight {
					break
				}
				t.Logf("Waiting, height %d < %d", gotHeight, wantHeight)
				time.Sleep(time.Second)
			}

			balance1, _, _, err := wallet2.ConfirmedBalance()
			require.NoError(t, err)
			t.Logf("Confirmed balance 1: %s", balance1)

			burntAmount := types.NewCurrency64(10000000000000000000).Mul(types.NewCurrency64(100000000))

			_, err = ht.wallet.SendSiacoins(burntAmount, tc.sendAddress)
			require.NoError(t, err)

			_, err = ht.miner.AddBlock()
			require.NoError(t, err)

			// Wait for the block to propagate to wallet2.
			wantHeight = ht.cs.Height()
			for {
				gotHeight, err := wallet2.Height()
				require.NoError(t, err)
				if gotHeight >= wantHeight {
					break
				}
				t.Logf("Waiting, height %d < %d", gotHeight, wantHeight)
				time.Sleep(time.Second)
			}

			balance2, _, _, err := wallet2.ConfirmedBalance()
			require.NoError(t, err)
			t.Logf("Confirmed balance 2: %s", balance2)

			require.Equal(t, tc.wantBalance, balance2.String())

			diff := balance2.Sub(balance1)
			require.Equal(t, burntAmount.String(), diff.String())
		})
	}
}
