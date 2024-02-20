package types

import (
	"errors"
	"fmt"
	"time"
)

// SpfTransportType introduces enum for SPF transport types.
type SpfTransportType int

// SpfTransportType constants.
const (
	Airdrop SpfTransportType = iota
	Premined
	Regular
)

// SpfType enum (A or B).
type SpfType int

// SpfType constants.
const (
	SpfA SpfType = iota
	SpfB
)

// SpfTypeFromString creates SpfType from string.
func SpfTypeFromString(str string) (t SpfType, err error) {
	switch str {
	case "spfa":
		t = SpfA
	case "spfb":
		t = SpfB
	default:
		err = fmt.Errorf("can not create SpfType from string %s", str)
	}
	return
}

// String method converts SpfType to string.
func (st SpfType) String() string {
	if st == SpfA {
		return "spfa"
	}
	return "spfb"
}

// SpfAmount represents amount of SPF of specific type.
type SpfAmount struct {
	Amount Currency `json:"amount"`
	Type   SpfType  `json:"type"`
}

// SpfTransportTypeAllowance contains information about SPF amounts
// allowed to transport + wait estimates for these to complete.
type SpfTransportTypeAllowance struct {
	MaxAllowed   Currency      `json:"max_allowed"` // min(PotentialMax, WalletBalance)
	WaitTime     time.Duration `json:"wait_time"`
	PotentialMax Currency      `json:"potential_max"` // Max allowed on transporter side.
}

// SpfTransportAllowance contains allowance for all types.
type SpfTransportAllowance struct {
	Regular  SpfTransportTypeAllowance            `json:"regular"`
	Premined map[string]SpfTransportTypeAllowance `json:"premined"`
	Airdrop  *SpfTransportTypeAllowance           `json:"airdrop,omitempty"`
}

// ApplyTo validates SPF amount against allowance.
func (spfAllowance *SpfTransportAllowance) ApplyTo(spf SpfAmount, t SpfTransportType, preminedUh *UnlockHash) error {
	switch t {
	case Regular:
		if spf.Amount.Cmp(spfAllowance.Regular.MaxAllowed) > 0 {
			return fmt.Errorf("amount %s exceeds the limit of %s for type Regular", spf.Amount.String(), spfAllowance.Regular.MaxAllowed.String())
		}
	case Premined:
		if preminedUh == nil {
			return errors.New("nil premined UnlockHash but type is Premined")
		}
		preminedAllowance, ok := spfAllowance.Premined[preminedUh.String()]
		if !ok {
			return fmt.Errorf("premined unlock hash %s does not exist", preminedUh.String())
		}
		if spf.Amount.Cmp(preminedAllowance.MaxAllowed) > 0 {
			return fmt.Errorf("amount %s exceeds the limit of %s for type Premined; uh %s", spf.Amount.String(), preminedAllowance.MaxAllowed.String(), preminedUh.String())
		}
	case Airdrop:
		if spfAllowance.Airdrop == nil {
			return errors.New("airdrop transports are not allowed for this wallet")
		}
		if spf.Amount.Cmp(spfAllowance.Airdrop.MaxAllowed) > 0 {
			return fmt.Errorf("amount %s exceeds the limit of %s for type Airdrop", spf.Amount.String(), spfAllowance.Airdrop.MaxAllowed.String())
		}
	}
	return nil
}

// SpfTransportStatus introduces enum for SPF transport states.
type SpfTransportStatus int

// SpfTransportStatus constants.
const (
	BurnCreated SpfTransportStatus = iota
	BurnBroadcasted
	SubmittedToTransporter
	InTheQueue
	Completed
)

// String method converts SpfTransportStatus to string.
func (sts SpfTransportStatus) String() string {
	switch sts {
	case BurnCreated:
		return "burn created"
	case BurnBroadcasted:
		return "burn broadcasted"
	case SubmittedToTransporter:
		return "submitted to transporter"
	case InTheQueue:
		return "in the qeue"
	case Completed:
		return "completed"
	default:
		return "invalid"
	}
}

// SPF transport record + its ID.
type SpfTransport struct {
	BurnID TransactionID `json:"burn_id"`
	SpfTransportRecord
}

// SpfTransportRecord represents single SPF transport.
type SpfTransportRecord struct {
	Status  SpfTransportStatus `json:"status"`
	Amount  Currency           `json:"currency"`
	Created time.Time          `json:"created"`
}

// SolanaAddrLen is the length of Solana public keys.
const SolanaAddrLen = 32

// SolanaAddress represents address on Solana blockchain (public key).
type SolanaAddress [SolanaAddrLen]byte
