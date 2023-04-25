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
		name           string
		enabledHeight  *types.BlockHeight
		sendAddress    types.UnlockHash
		spendAddress   *types.UnlockHash
		wantBalance    string
		disabledHeight *types.BlockHeight
	}{
		{
			name:           "unburn",
			enabledHeight:  &types.UnburnStartBlockHeight,
			sendAddress:    types.BurnAddressUnlockHash,
			spendAddress:   &types.UnburnAddressUnlockHash,
			wantBalance:    "301000000000000000000000000000",
			disabledHeight: &types.UnburnStopBlockHeight,
		},
		{
			name:          "ungift",
			enabledHeight: &types.UngiftStartBlockHeight,
			sendAddress:   types.AirdropNebulousLabsUnlockHash,
			spendAddress:  &types.UngiftUnlockHash,
			wantBalance:   "300001000000000000000000000000000",
		},
		{
			name:          "ungift2",
			enabledHeight: &types.Ungift2StartBlockHeight,
			sendAddress:   types.AirdropNebulousLabsUnlockHash,
			spendAddress:  &types.Ungift2UnlockHash,
			wantBalance:   "300001000000000000000000000000000",
		},
	}

	burntAmount := types.NewCurrency64(10000000000000000000).Mul(types.NewCurrency64(100000000))
	thresholdAmount := types.NewCurrency64(10000000000000000000).Mul(types.NewCurrency64(100000))

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var (
				err           error
				ht            *hostTester
				encryptionKey crypto.CipherKey
				seed          modules.Seed
				addr          types.UnlockConditions
				wallet2       *wallet.Wallet
				wallet3       *wallet.Wallet
				balance2      modules.ConfirmedBalance
			)

			t.Run("initialize wallets", func(t *testing.T) {
				ht, err = newMockHostTester(modules.ProdDependencies, t.Name())
				require.NoError(t, err)

				encryptionKey = crypto.GenerateSiaKey(crypto.TypeDefaultWallet)

				wallet1, err := wallet.New(ht.cs, ht.tpool, filepath.Join(ht.persistDir, modules.WalletDir+"wallet1"))
				require.NoError(t, err)

				seed, err = wallet1.Encrypt(encryptionKey)
				require.NoError(t, err)
				require.NoError(t, wallet1.Unlock(encryptionKey))

				addr, err = wallet1.NextAddress()
				require.NoError(t, err)
			})
			t.Cleanup(func() {
				require.NoError(t, ht.Close())
			})

			sync := func(w *wallet.Wallet) {
				// Wait for the block to propagate to wallet2.
				wantHeight := ht.cs.Height()
				for {
					gotHeight, err := w.Height()
					require.NoError(t, err)
					if gotHeight >= wantHeight {
						break
					}
					t.Logf("Waiting, height %d < %d", gotHeight, wantHeight)
					time.Sleep(time.Second)
				}
			}

			// Make some of wallet's addresses spendAddress and activate Fork2022.
			originalSpendAddress := *tc.spendAddress
			originalFork2022 := types.Fork2022
			*tc.spendAddress = addr.UnlockHash()
			types.Fork2022 = true
			t.Cleanup(func() {
				*tc.spendAddress = originalSpendAddress
				types.Fork2022 = originalFork2022
			})

			if tc.enabledHeight != nil {
				// Set enabledHeight to current height to enable spending.
				originalEnabledHeight := *tc.enabledHeight
				*tc.enabledHeight = ht.cs.Height()
				t.Cleanup(func() {
					*tc.enabledHeight = originalEnabledHeight
				})
			}

			t.Run("recreate the wallet with the same seed", func(t *testing.T) {
				wallet2, err = wallet.New(ht.cs, ht.tpool, filepath.Join(ht.persistDir, modules.WalletDir+"wallet2"))
				require.NoError(t, err)
				require.NoError(t, wallet2.InitFromSeed(encryptionKey, seed))
				require.NoError(t, wallet2.Unlock(encryptionKey))
			})

			t.Run("send coins to special address", func(t *testing.T) {
				// Wait for the block to propagate to wallet2.
				sync(wallet2)

				balance1, err := wallet2.ConfirmedBalance()
				require.NoError(t, err)
				t.Logf("Confirmed balance 1: %s", balance1)

				_, err = ht.wallet.SendSiacoins(burntAmount, tc.sendAddress)
				require.NoError(t, err)

				_, err = ht.miner.AddBlock()
				require.NoError(t, err)

				// Wait for the block to propagate to wallet2.
				sync(wallet2)

				balance2, err = wallet2.ConfirmedBalance()
				require.NoError(t, err)
				t.Logf("Confirmed balance 2: %s", balance2)

				require.Equal(t, tc.wantBalance, balance2.CoinBalance.String())

				diff := balance2.CoinBalance.Sub(balance1.CoinBalance)
				require.Equal(t, burntAmount.String(), diff.String())
			})

			t.Run("send coins from special address", func(t *testing.T) {
				wallet3, err = wallet.New(ht.cs, ht.tpool, filepath.Join(ht.persistDir, modules.WalletDir+"wallet3"))
				require.NoError(t, err)
				encryptionKey2 := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)
				_, err = wallet3.Encrypt(encryptionKey2)
				require.NoError(t, err)
				require.NoError(t, wallet3.Unlock(encryptionKey2))

				addr, err = wallet3.NextAddress()
				require.NoError(t, err)

				amount := balance2.CoinBalance.Div(types.NewCurrency64(10))

				_, err = wallet2.SendSiacoins(amount, addr.UnlockHash())
				require.NoError(t, err)

				_, err = ht.miner.AddBlock()
				require.NoError(t, err)

				// Wait for the block to propagate to wallet2.
				sync(wallet3)

				balance3, err := wallet3.ConfirmedBalance()
				require.NoError(t, err)
				t.Logf("Confirmed balance 3: %s", balance3)

				require.Equal(t, amount.String(), balance3.CoinBalance.String())
			})

			t.Run("make sure it is impossible to send coins to replaced address", func(t *testing.T) {
				_, err = ht.wallet.SendSiacoins(thresholdAmount, *tc.spendAddress)
				require.ErrorContains(t, err, types.ErrBadOutput.Error())
			})

			if tc.disabledHeight != nil {
				t.Run("make sure it stops working after disabledHeight", func(t *testing.T) {
					// First, assemble all the coins in wallet2 on the sendAddress.
					// They can now be on a change address after the sending subtest.
					balance2a, err := wallet2.ConfirmedBalance()
					require.NoError(t, err)
					amount := balance2a.CoinBalance.Sub(thresholdAmount)
					_, err = wallet2.SendSiacoins(amount, tc.sendAddress)
					require.NoError(t, err)
					_, err = ht.miner.AddBlock()
					require.NoError(t, err)

					// Disable the effect at the next block height.
					// The coins should be locked on sendAddress.
					originalDisabledHeight := *tc.disabledHeight
					*tc.disabledHeight = ht.cs.Height() + 1
					t.Cleanup(func() {
						*tc.disabledHeight = originalDisabledHeight
					})

					_, err = ht.miner.AddBlock()
					require.NoError(t, err)

					sync(wallet2)

					// Make sure that coins can not be spent.
					amount = balance2.CoinBalance.Div(types.NewCurrency64(10))
					addr, err = wallet3.NextAddress()
					require.NoError(t, err)

					_, err = wallet2.SendSiacoins(amount, addr.UnlockHash())
					require.ErrorContains(t, err, types.ErrBadInput.Error())
				})
			}
		})
	}
}
