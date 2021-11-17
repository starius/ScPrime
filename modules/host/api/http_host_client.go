package api

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/starius/api2"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/zer0main/checkport"
)

// HostClienter stores and provides clients for host API.
type HostClienter struct {
	clientsMu sync.RWMutex
	clients   map[string]*http.Client
}

// NewClienter creates HostClienter.
func NewClienter() *HostClienter {
	return &HostClienter{
		clients: make(map[string]*http.Client),
	}
}

// Client returns client by public key.
func (hc *HostClienter) Client(host, port string, hostPk types.SiaPublicKey) (*Client, error) {
	hostPkStr := hostPk.String()
	hc.clientsMu.RLock()
	client, ok := hc.clients[hostPkStr]
	hc.clientsMu.RUnlock()
	if !ok {
		hc.clientsMu.Lock()
		client = modules.GetHostApiClient(hostPk)
		hc.clients[hostPkStr] = client
		hc.clientsMu.Unlock()
	}
	return HostClientFromHttpClient(host, port, client)
}

// HostClientFromHttpClient creates host API client initialized with provided http.Client.
func HostClientFromHttpClient(host, port string, client *http.Client) (*Client, error) {
	if host == "" {
		return nil, fmt.Errorf("empty address")
	}
	if port == "" {
		return nil, fmt.Errorf("empty port")
	}
	if err := checkport.CheckPort(port); err != nil {
		return nil, fmt.Errorf("check port: %w", err)
	}

	u := url.URL{
		Scheme: "https",
		Host:   net.JoinHostPort(host, port),
	}
	hostClient, err := NewClient(u.String(), api2.CustomClient(client))
	if err != nil {
		return nil, fmt.Errorf("host addr %s. new client: %w", host, err)
	}
	return hostClient, nil
}

// HostClientFromPk creates host API client using public key.
func HostClientFromPk(host, port string, hostPk types.SiaPublicKey) (*Client, error) {
	client := modules.GetHostApiClient(hostPk)
	return HostClientFromHttpClient(host, port, client)
}
