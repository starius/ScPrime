package wallet

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/miner"
	"gitlab.com/scpcorp/ScPrime/modules/wallet/mock_transporter_client"
	"gitlab.com/scpcorp/ScPrime/types"
	transporter "gitlab.com/scpcorp/spf-transporter"
	"gitlab.com/scpcorp/spf-transporter/common"
	"go.uber.org/mock/gomock"
)

func TestSpftransportsPremined(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ctrl := gomock.NewController(t)
	tc := mock_transporter_client.NewMockTransporterClient(ctrl)

	var wt *walletTester
	t.Cleanup(func() {
		if wt != nil {
			require.NoError(t, wt.closeWt())
		}
	})

	fundAddress := types.MustParseAddress("d6a6c5a41dc935ec6aef0a9e7f83148a3fdde61062f7204dd244740cf1591bdfc10dca990dd5")

	t.Run("setup", func(t *testing.T) {
		tc.EXPECT().
			PreminedList(gomock.Any(), gomock.Any()).
			Return(&transporter.PreminedListResponse{
				Premined: map[string]common.PreminedRecord{
					fundAddress.String(): {
						Limit: types.NewCurrency64(3000),
					},
				},
			}, error(nil))

		var err error
		wt, err = createWalletTester("TestSpftransportsPremined", modules.ProdDependencies, WithTransporterClient(tc))
		require.NoError(t, err)

		// Load the key into the wallet.
		err = wt.wallet.LoadSiagKeys(wt.walletMasterKey, []string{"../../types/siag0of1of1.siakey"})
		require.NoError(t, err)

		oldBal, err := wt.wallet.ConfirmedBalance()
		require.NoError(t, err)
		require.Equal(t, "2000", oldBal.FundBalance.String())
		// need to reset the miner as well, since it depends on the wallet
		wt.miner, err = miner.New(wt.cs, wt.tpool, wt.wallet, wt.wallet.persistDir)
		require.NoError(t, err)
	})

	t.Run("SiafundTransportAllowance empty", func(t *testing.T) {
		tc.EXPECT().
			CheckAllowance(gomock.Any(), &transporter.CheckAllowanceRequest{
				PreminedUnlockHashes: []types.UnlockHash{fundAddress},
			}).
			Return(&transporter.CheckAllowanceResponse{}, error(nil))

		allowance, err := wt.wallet.SiafundTransportAllowance(types.SpfA)
		require.NoError(t, err)
		require.Equal(t, &types.SpfTransportAllowance{
			Premined: map[string]types.SpfTransportTypeAllowance{},
		}, allowance)
	})

	t.Run("SiafundTransportAllowance premined", func(t *testing.T) {
		tc.EXPECT().
			CheckAllowance(gomock.Any(), &transporter.CheckAllowanceRequest{
				PreminedUnlockHashes: []types.UnlockHash{fundAddress},
			}).
			Return(&transporter.CheckAllowanceResponse{
				Premined: map[string]transporter.AmountWithTimeEstimate{
					fundAddress.String(): {
						Amount: types.NewCurrency64(3500),
					},
				},
			}, error(nil))

		allowance, err := wt.wallet.SiafundTransportAllowance(types.SpfA)
		require.NoError(t, err)
		require.Equal(t, &types.SpfTransportAllowance{
			Premined: map[string]types.SpfTransportTypeAllowance{
				fundAddress.String(): {
					MaxAllowed:   types.NewCurrency64(2000),
					PotentialMax: types.NewCurrency64(3500),
				},
			},
		}, allowance)
	})

	t.Run("SiafundTransportAllowance spf-b", func(t *testing.T) {
		tc.EXPECT().
			CheckAllowance(gomock.Any(), &transporter.CheckAllowanceRequest{
				PreminedUnlockHashes: []types.UnlockHash{fundAddress},
			}).
			Return(&transporter.CheckAllowanceResponse{
				Premined: map[string]transporter.AmountWithTimeEstimate{
					fundAddress.String(): {
						Amount: types.NewCurrency64(3500),
					},
				},
			}, error(nil))

		allowance, err := wt.wallet.SiafundTransportAllowance(types.SpfB)
		require.NoError(t, err)
		require.Equal(t, &types.SpfTransportAllowance{
			Premined: map[string]types.SpfTransportTypeAllowance{
				fundAddress.String(): {
					MaxAllowed:   types.ZeroCurrency,
					PotentialMax: types.NewCurrency64(3500),
				},
			},
		}, allowance)

		// TODO: check non-empty case for SPF-B.
	})

	t.Run("SiafundTransportHistory empty", func(t *testing.T) {
		history, err := wt.wallet.SiafundTransportHistory()
		require.NoError(t, err)
		require.Empty(t, history)
	})

	submitTime := types.CurrentTimestamp()
	var tx types.Transaction

	t.Run("SiafundTransportSend 10 funds", func(t *testing.T) {
		spfAmount := types.SpfAmount{
			Amount: types.NewCurrency64(10),
			Type:   types.SpfA,
		}
		solanaAddr := types.SolanaAddress{'T', 'e', 's', 't', 'S', 'o', 'l', 'a', 'n', 'a'}

		tc.EXPECT().
			CheckAllowance(gomock.Any(), &transporter.CheckAllowanceRequest{
				PreminedUnlockHashes: []types.UnlockHash{fundAddress},
			}).
			Return(&transporter.CheckAllowanceResponse{
				Premined: map[string]transporter.AmountWithTimeEstimate{
					fundAddress.String(): {
						Amount: types.NewCurrency64(3500),
					},
				},
			}, error(nil))

		tc.EXPECT().
			CheckSolanaAddress(gomock.Any(), &transporter.CheckSolanaAddressRequest{
				SolanaAddress: common.SolanaAddress(solanaAddr),
				Amount:        spfAmount.Amount,
			}).
			Return(&transporter.CheckSolanaAddressResponse{
				CurrentTime: time.Now(),
			}, error(nil))

		tc.EXPECT().
			SubmitScpTx(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *transporter.SubmitScpTxRequest) (*transporter.SubmitScpTxResponse, error) {
				tx = req.Transaction
				return &transporter.SubmitScpTxResponse{
					WaitTimeEstimate: 0,
					SpfAmountAhead:   nil,
				}, nil
			})

		dur, cur, err := wt.wallet.SiafundTransportSend(spfAmount, types.Premined, &fundAddress, solanaAddr)
		require.NoError(t, err)
		require.Equal(t, time.Duration(0), dur)
		require.Nil(t, cur)
	})

	t.Run("SiafundTransportHistory one record", func(t *testing.T) {
		history, err := wt.wallet.SiafundTransportHistory()
		require.NoError(t, err)
		require.Equal(t, []types.SpfTransport{{
			BurnID: tx.ID(),
			SpfTransportRecord: types.SpfTransportRecord{
				Status:  types.SubmittedToTransporter,
				Amount:  types.NewCurrency64(10),
				Created: submitTime,
			},
		}}, history)
	})
}

func TestSpftransportsRegular(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ctrl := gomock.NewController(t)
	tc := mock_transporter_client.NewMockTransporterClient(ctrl)

	var wt *walletTester
	t.Cleanup(func() {
		if wt != nil {
			require.NoError(t, wt.closeWt())
		}
	})

	t.Run("setup", func(t *testing.T) {
		tc.EXPECT().
			PreminedList(gomock.Any(), gomock.Any()).
			Return(&transporter.PreminedListResponse{}, error(nil))

		var err error
		wt, err = createWalletTester("TestSpftransportsRegular", modules.ProdDependencies, WithTransporterClient(tc))
		require.NoError(t, err)

		// Load the key into the wallet.
		err = wt.wallet.LoadSiagKeys(wt.walletMasterKey, []string{"../../types/siag0of1of1.siakey"})
		require.NoError(t, err)

		oldBal, err := wt.wallet.ConfirmedBalance()
		require.NoError(t, err)
		require.Equal(t, "2000", oldBal.FundBalance.String())
		// need to reset the miner as well, since it depends on the wallet
		wt.miner, err = miner.New(wt.cs, wt.tpool, wt.wallet, wt.wallet.persistDir)
		require.NoError(t, err)
	})

	t.Run("SiafundTransportAllowance", func(t *testing.T) {
		tc.EXPECT().
			CheckAllowance(gomock.Any(), &transporter.CheckAllowanceRequest{}).
			Return(&transporter.CheckAllowanceResponse{
				Regular: transporter.AmountWithTimeEstimate{
					Amount:       types.NewCurrency64(3500),
					WaitEstimate: 10 * time.Hour,
				},
			}, error(nil))

		allowance, err := wt.wallet.SiafundTransportAllowance(types.SpfA)
		require.NoError(t, err)
		require.Equal(t, &types.SpfTransportAllowance{
			Regular: types.SpfTransportTypeAllowance{
				MaxAllowed:   types.NewCurrency64(3500),
				WaitTime:     34500 * time.Second,
				PotentialMax: types.NewCurrency64(3500),
			},
			Premined: map[string]types.SpfTransportTypeAllowance{},
		}, allowance)
	})

	t.Run("SiafundTransportAllowance spf-b", func(t *testing.T) {
		tc.EXPECT().
			CheckAllowance(gomock.Any(), &transporter.CheckAllowanceRequest{}).
			Return(&transporter.CheckAllowanceResponse{
				Regular: transporter.AmountWithTimeEstimate{
					Amount:       types.NewCurrency64(3500),
					WaitEstimate: 10 * time.Hour,
				},
			}, error(nil))

		allowance, err := wt.wallet.SiafundTransportAllowance(types.SpfB)
		require.NoError(t, err)
		require.Equal(t, &types.SpfTransportAllowance{
			Regular: types.SpfTransportTypeAllowance{
				MaxAllowed:   types.NewCurrency64(3500),
				WaitTime:     32460 * time.Second,
				PotentialMax: types.NewCurrency64(3500),
			},
			Premined: map[string]types.SpfTransportTypeAllowance{},
		}, allowance)

		// TODO: check non-empty case for SPF-B.
	})

	t.Run("SiafundTransportHistory empty", func(t *testing.T) {
		history, err := wt.wallet.SiafundTransportHistory()
		require.NoError(t, err)
		require.Empty(t, history)
	})

	submitTime := types.CurrentTimestamp()
	var tx types.Transaction

	t.Run("SiafundTransportSend 10 funds", func(t *testing.T) {
		spfAmount := types.SpfAmount{
			Amount: types.NewCurrency64(10),
			Type:   types.SpfA,
		}
		solanaAddr := types.SolanaAddress{'T', 'e', 's', 't', 'S', 'o', 'l', 'a', 'n', 'a'}

		tc.EXPECT().
			CheckAllowance(gomock.Any(), &transporter.CheckAllowanceRequest{}).
			Return(&transporter.CheckAllowanceResponse{
				Regular: transporter.AmountWithTimeEstimate{
					Amount: types.NewCurrency64(3500),
				},
			}, error(nil))

		tc.EXPECT().
			CheckSolanaAddress(gomock.Any(), &transporter.CheckSolanaAddressRequest{
				SolanaAddress: common.SolanaAddress(solanaAddr),
				Amount:        spfAmount.Amount,
			}).
			Return(&transporter.CheckSolanaAddressResponse{
				CurrentTime: time.Now(),
			}, error(nil))

		spfAmountAhead := types.NewCurrency64(100)
		tc.EXPECT().
			SubmitScpTx(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *transporter.SubmitScpTxRequest) (*transporter.SubmitScpTxResponse, error) {
				tx = req.Transaction
				return &transporter.SubmitScpTxResponse{
					WaitTimeEstimate: 10 * time.Second,
					SpfAmountAhead:   &spfAmountAhead,
				}, nil
			})

		dur, cur, err := wt.wallet.SiafundTransportSend(spfAmount, types.Regular, nil, solanaAddr)
		require.NoError(t, err)
		require.Equal(t, 10*time.Second, dur)
		require.Equal(t, &spfAmountAhead, cur)
	})

	t.Run("SiafundTransportHistory one record", func(t *testing.T) {
		history, err := wt.wallet.SiafundTransportHistory()
		require.NoError(t, err)
		require.Equal(t, []types.SpfTransport{{
			BurnID: tx.ID(),
			SpfTransportRecord: types.SpfTransportRecord{
				Status:  types.SubmittedToTransporter,
				Amount:  types.NewCurrency64(10),
				Created: submitTime,
			},
		}}, history)
	})
}
