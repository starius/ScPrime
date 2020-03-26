package modules

import (
	"net"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
)

const (
	// GatewayDir is the name of the directory used to store the gateway's
	// persistent data.
	GatewayDir = "gateway"
)

var (
	// BootstrapPeers is a list of peers that can be used to find other peers -
	// when a client first connects to the network, the only options for
	// finding peers are either manual entry of peers or to use a hardcoded
	// bootstrap point. While the bootstrap point could be a central service,
	// it can also be a list of peers that are known to be stable. We have
	// chosen to hardcode known-stable peers.
	BootstrapPeers = build.Select(build.Var{
		Standard: []NetAddress{
			"50.116.9.206:4281",
			"45.79.100.217:4281",
			"52.39.226.32:4281",
			"68.183.100.181:4281",
			"59.167.191.60:4281",
			"81.149.127.41:4281",
			"107.2.170.129:4281",
			"195.130.205.91:4281",
		},
		Dev:     []NetAddress(nil),
		Testing: []NetAddress(nil),
	}).([]NetAddress)
)

type (
	// Peer contains all the info necessary to Broadcast to a peer.
	Peer struct {
		Inbound    bool       `json:"inbound"`
		Local      bool       `json:"local"`
		NetAddress NetAddress `json:"netaddress"`
		Version    string     `json:"version"`
	}

	// A PeerConn is the connection type used when communicating with peers during
	// an RPC. It is identical to a net.Conn with the additional RPCAddr method.
	// This method acts as an identifier for peers and is the address that the
	// peer can be dialed on. It is also the address that should be used when
	// calling an RPC on the peer.
	PeerConn interface {
		net.Conn
		RPCAddr() NetAddress
	}

	// RPCFunc is the type signature of functions that handle RPCs. It is used for
	// both the caller and the callee. RPCFuncs may perform locking. RPCFuncs may
	// close the connection early, and it is recommended that they do so to avoid
	// keeping the connection open after all necessary I/O has been performed.
	RPCFunc func(PeerConn) error

	// A Gateway facilitates the interactions between the local node and remote
	// nodes (peers). It relays incoming blocks and transactions to local modules,
	// and broadcasts outgoing blocks and transactions to peers. In a broad sense,
	// it is responsible for ensuring that the local consensus set is consistent
	// with the "network" consensus set.
	Gateway interface {
		Alerter

		// BandwidthCounters returns the Gateway's upload and download bandwidth
		BandwidthCounters() (uint64, uint64, time.Time, error)

		// Connect establishes a persistent connection to a peer.
		Connect(NetAddress) error

		// ConnectManual is a Connect wrapper for a user-initiated Connect
		ConnectManual(NetAddress) error

		// Disconnect terminates a connection to a peer.
		Disconnect(NetAddress) error

		// DiscoverAddress discovers and returns the current public IP address
		// of the gateway. Contrary to Address, DiscoverAddress is blocking and
		// might take multiple minutes to return. A channel to cancel the
		// discovery can be supplied optionally.
		DiscoverAddress(cancel <-chan struct{}) (net.IP, error)

		// ForwardPort adds a port mapping to the router. It will block until
		// the mapping is established or until it is interrupted by a shutdown.
		ForwardPort(port string) error

		// DisconnectManual is a Disconnect wrapper for a user-initiated
		// disconnect
		DisconnectManual(NetAddress) error

		// AddToBlacklist adds addresses to the blacklist of the gateway
		AddToBlacklist(addresses []string) error

		// Blacklist returns the current blacklist of the Gateway
		Blacklist() ([]string, error)

		// RemoveFromBlacklist removes addresses from the blacklist of the
		// gateway
		RemoveFromBlacklist(addresses []string) error

		// SetBlacklist sets the blacklist of the gateway
		SetBlacklist(addresses []string) error

		// Address returns the Gateway's address.
		Address() NetAddress

		// Peers returns the addresses that the Gateway is currently connected
		// to.
		Peers() []Peer

		// RegisterRPC registers a function to handle incoming connections that
		// supply the given RPC ID.
		RegisterRPC(string, RPCFunc)

		// RateLimits returns the currently set bandwidth limits of the gateway.
		RateLimits() (int64, int64)

		// SetRateLimits changes the rate limits for the peer-connections of the
		// gateway.
		SetRateLimits(downloadSpeed, uploadSpeed int64) error

		// UnregisterRPC unregisters an RPC and removes all references to the
		// RPCFunc supplied in the corresponding RegisterRPC call. References to
		// RPCFuncs registered with RegisterConnectCall are not removed and
		// should be removed with UnregisterConnectCall. If the RPC does not
		// exist no action is taken.
		UnregisterRPC(string)

		// RegisterConnectCall registers an RPC name and function to be called
		// upon connecting to a peer.
		RegisterConnectCall(string, RPCFunc)

		// UnregisterConnectCall unregisters an RPC and removes all references to the
		// RPCFunc supplied in the corresponding RegisterConnectCall call. References
		// to RPCFuncs registered with RegisterRPC are not removed and should be
		// removed with UnregisterRPC. If the RPC does not exist no action is taken.
		UnregisterConnectCall(string)

		// RPC calls an RPC on the given address. RPC cannot be called on an
		// address that the Gateway is not connected to.
		RPC(NetAddress, string, RPCFunc) error

		// Broadcast transmits obj, prefaced by the RPC name, to all of the
		// given peers in parallel.
		Broadcast(name string, obj interface{}, peers []Peer)

		// Online returns true if the gateway is connected to remote hosts
		Online() bool

		// Close safely stops the Gateway's listener process.
		Close() error
	}
)
