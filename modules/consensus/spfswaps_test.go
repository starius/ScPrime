package consensus

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TestSwapOffer checks if SCP-SPF swap offer works as expected
func TestSwapOffer(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	wt, err := blankConsensusSetTester(t.Name(), modules.ProdDependencies)
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Close()

	// mine a few blocks to create miner payouts
	for i := 0; i < 5; i++ {
		_, err = wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}

	// create some siacoin outputs
	uc, err := wt.wallet.NextAddress()
	addr := uc.UnlockHash()
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.wallet.SendSiacoins(types.ScPrimecoinPrecision, addr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	//Offer inputs
	uc, err = wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	offerSum := types.ScPrimecoinPrecision.Mul64(3)
	offerType := types.SpecifierSiacoinOutput
	expectSum := types.NewCurrency64(2)
	expectType := types.SpecifierSiafundOutput
	addr = uc.UnlockHash()
	offer, err := wt.wallet.CreateSwapOffer(offerSum, offerType, expectSum, expectType, addr)
	if err != nil {
		t.Fatalf("couldn't create swap offer due to error: %s", err.Error())
	}
	swapstatus, err := wt.wallet.CheckSwapOffer(offer)
	if err != nil {
		txt, _ := json.MarshalIndent(offer, "", "\t")
		t.Logf("Offer created:%+v", string(txt))
		t.Fatalf("couldn't check swap offer due to error: %s", err.Error())
	}
	t.Logf("swap offer status : %+v", swapstatus)
	//try accepting it on the same wallet
	uc2, err := wt.wallet.NextAddress()
	if err != nil {
		t.Fatal(err)
	}
	addr2 := uc2.UnlockHash()
	offer2, err := wt.wallet.AcceptSwapOffer(offer, addr2)
	if err == nil {
		t.Fatal("Expected error when accepting offer")
	}
	t.Logf("Got error as expected: %v", err)
	if !reflect.DeepEqual(offer, offer2) {
		t.Logf("swap offer after failed accept try : %+v", offer2)
		t.Fatal("The offer should have no changes")
	}

	wt.addSiafunds()
	addr3 := randAddress()
	offer2, err = wt.wallet.AcceptSwapOffer(offer, addr3)
	if err == nil {
		t.Fatal("Expected error when accepting offer")
	}
	t.Logf("Got error as expected: %v", err)
	if !reflect.DeepEqual(offer, offer2) {
		t.Fatal("The offer should not have changes")
	}

	offer2, err = wt.wallet.AcceptSwapOffer(offer, addr2)
	if err == nil {
		t.Fatal("Expected error when accepting offer using same wallet")
	}
	t.Logf("Got error as expected: %v", err)
	if !reflect.DeepEqual(offer, offer2) {
		t.Fatal("The offer should not have changes")
	}

	//Need a second wallet to create swap
	wallet2, err := wallet.New(wt.cs, wt.tpool, filepath.Join(wt.persistDir, "wallet2"))
	if err != nil {
		t.Fatalf("Error creating second wallet: %v", err)
	}
	defer wallet2.Close()
	key2 := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)
	_, err = wallet2.Encrypt(key2)
	if err != nil {
		t.Fatalf("Error initializing second wallet: %v", err)
	}
	err = wallet2.Unlock(key2)
	if err != nil {
		t.Fatalf("Error unlocking second wallet: %v", err)
	}

	swapstatus1, err := wt.wallet.CheckSwapOffer(offer)
	if err != nil {
		t.Errorf("CheckSwapOffer error: %s", err.Error())
	}
	t.Logf("swap offer status on initiator before accept: %+v", swapstatus1)

	swapstatus2, err := wallet2.CheckSwapOffer(offer)
	if err != nil {
		t.Errorf("CheckSwapOffer error: %s", err.Error())
	}
	t.Logf("swap offer status on acceptor before accept: %+v", swapstatus2)

	uc3, err := wallet2.NextAddress()
	if err != nil {
		t.Fatalf("Error getting second wallet address: %v", err)
	}
	addr4 := uc3.UnlockHash()
	_, err = wt.wallet.SendSiafunds(types.NewCurrency64(10), addr4)
	if err != nil {
		t.Fatalf("Error sending SPF to second wallet address: %v", err)
	}
	_, err = wt.miner.AddBlock()
	if err != nil {
		t.Fatal(err)
	}

	offer2, err = wallet2.AcceptSwapOffer(offer, addr4)
	if err != nil {
		t.Fatalf("Error accepting offer: %v", err)
	}

	swapstatus1, err = wt.wallet.CheckSwapOffer(offer2)
	if err != nil {
		t.Errorf("CheckSwapOffer error: %s", err.Error())
	}
	t.Logf("swap offer status on initiator before finalize : %+v", swapstatus1)

	swapstatus2, err = wallet2.CheckSwapOffer(offer2)
	if err != nil {
		t.Errorf("CheckSwapOffer error: %s", err.Error())
	}
	t.Logf("swap offer status on acceptor before finalize: %+v", swapstatus2)

	txns, err := wt.wallet.FinalizeSwapOffer(offer2)
	if err != nil {
		t.Errorf("FinalizeSwapOffer on initiator error: %s", err.Error())
	}
	for i := 0; i < 5; i++ {
		_, err = wt.miner.AddBlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(txns) > 0 {
		if modules.TransactionType(&txns[0]) != modules.TXTypeSwapSPF {
			t.Errorf("Transaction type= %v instead of %v", modules.TransactionType(&txns[0]), modules.TXTypeSwapSPF)
		}
		errValid := txns[0].StandaloneValid(wt.cs.Height())
		if errValid != nil {
			t.Errorf("transaction validity error: %s", errValid.Error())
			txt, _ := json.MarshalIndent(txns[0], "", "\t")
			t.Logf("Transactions after Finalize on initiator:%+v", string(txt))
		}
		if err != nil {
			t.Errorf("FinalizeSwapOffer on initiator error: %s", err.Error())
		}
	} else {
		t.Error("Expected to generate transaction on FinalizeSwapOffer")
	}
	wb1, err := wt.wallet.ConfirmedBalance()
	if err != nil {
		t.Errorf("Error getting initiator balance: %s", err.Error())
	} else {
		t.Logf("Wallet1 balance: %+v", wb1)
	}
	wb2, err := wallet2.ConfirmedBalance()
	if err != nil {
		t.Errorf("Error getting initiator balance: %s", err.Error())
	} else {
		t.Logf("Wallet2 balance: %+v", wb2)
	}
	swapstatus1, err = wt.wallet.CheckSwapOffer(offer2)
	if err != nil {
		t.Errorf("CheckSwapOffer error: %s", err.Error())
	}
	t.Logf("swap offer status on initiator : %+v", swapstatus1)

	swapstatus2, err = wallet2.CheckSwapOffer(offer2)
	if err != nil {
		t.Errorf("CheckSwapOffer error: %s", err.Error())
	}
	t.Logf("swap offer status on acceptor: %+v", swapstatus2)
}
