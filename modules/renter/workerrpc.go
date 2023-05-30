package renter

import (
	"fmt"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"

	// "gitlab.com/NebulousLabs/errors"
	// "gitlab.com/NebulousLabs/ratelimit"
	"gitlab.com/NebulousLabs/siamux"
	"gitlab.com/NebulousLabs/siamux/mux"
)

// defaultNewStreamTimeout is a default timeout for creating a new stream.
var defaultNewStreamTimeout = build.Select(build.Var{
	Standard: 5 * time.Minute,
	Testing:  10 * time.Second,
	Dev:      time.Minute,
}).(time.Duration)

// defaultRPCDeadline is a default timeout for executing an RPC.
var defaultRPCDeadline = build.Select(build.Var{
	Standard: 5 * time.Minute,
	Testing:  10 * time.Second,
	Dev:      time.Minute,
}).(time.Duration)

// programResponse is a helper struct that wraps the RPCExecuteProgramResponse
// alongside the data output
type programResponse struct {
	modules.RPCExecuteProgramResponse
	Output []byte
}

// managedExecuteProgram performs the ExecuteProgramRPC on the host
func (w *worker) managedExecuteProgram(p modules.Program, data []byte, fcid types.FileContractID, cost types.Currency) (responses []programResponse, limit mux.BandwidthLimit, err error) {
	return
}

// staticNewStream returns a new stream to the worker's host
func (w *worker) staticNewStream() (siamux.Stream, error) {
	return nil, fmt.Errorf("Siamux Disabled")
	// if build.VersionCmp(w.staticCache().staticHostVersion, minAsyncVersion) < 0 {
	// 	w.renter.log.Critical("calling staticNewStream on a host that doesn't support the new protocol")
	// 	return nil, errors.New("host doesn't support this")
	// }

	// // If disrupt is called we sleep for the specified 'defaultNewStreamTimeout'
	// // simulating how an unreachable host would behave in production.
	// timeout := defaultNewStreamTimeout
	// if w.renter.deps.Disrupt("InterruptNewStreamTimeout") {
	// 	time.Sleep(timeout)
	// 	return nil, errors.New("InterruptNewStreamTimeout")
	// }

	// // Create a stream with a reasonable dial up timeout.
	// stream, err := w.renter.staticMux.NewStreamTimeout(modules.HostSiaMuxSubscriberName, w.staticHostMuxAddress, timeout, modules.SiaPKToMuxPK(w.staticHostPubKey))
	// if err != nil {
	// 	return nil, err
	// }
	// // Set deadline on the stream.
	// err = stream.SetDeadline(time.Now().Add(defaultRPCDeadline))
	// if err != nil {
	// 	return nil, err
	// }
	// // Wrap the stream in a ratelimit.
	// return ratelimit.NewRLStream(stream, w.renter.rl, w.renter.tg.StopChan()), nil
}
