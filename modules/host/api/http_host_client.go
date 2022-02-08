package api

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/hashicorp/golang-lru"
	"github.com/starius/api2"
	"github.com/starius/api2/closingclient"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/zer0main/checkport"
)

// HostClienterConfig represents HostClienter settings.
type HostClienterConfig struct {
	lruSize int
}

// NewDefaultHostClienterConfig creates default HostClienterConfig.
func NewDefaultHostClienterConfig() *HostClienterConfig {
	return &HostClienterConfig{}
}

// Option is option callback for NewHostClienter.
type Option func(*HostClienterConfig)

// WithLRU is an option for LRU.
func WithLRU(lruSize int) Option {
	return func(config *HostClienterConfig) {
		config.lruSize = lruSize
	}
}

func newLruCache(size int) (*lru.Cache, error) {
	return lru.NewWithEvict(size, func(key interface{}, value interface{}) {
		cli, ok := value.(*Client)
		if !ok {
			panic("failed to convert value to client type")
		}
		if err := cli.Close(); err != nil {
			log.Printf("Failed to close client on eviction: %v.", err)
		}
	})
}

// HostClienter stores and provides clients for host API.
type HostClienter struct {
	// If without LRU.
	clientsMu sync.RWMutex
	clients   map[string]*http.Client

	// If with LRU.
	cache *lru.Cache
}

// NewClienter creates HostClienter.
func NewClienter(opts ...Option) (*HostClienter, error) {
	config := NewDefaultHostClienterConfig()
	for _, opt := range opts {
		opt(config)
	}
	if config.lruSize == 0 {
		return &HostClienter{
			clients: make(map[string]*http.Client),
		}, nil
	}
	lruCache, err := newLruCache(config.lruSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}
	return &HostClienter{
		cache: lruCache,
	}, nil
}

type lruCacheKey struct {
	host, port string
	hostPk     types.SiaPublicKey
}

func (hc *HostClienter) clientFromLRU(host, port string, hostPk types.SiaPublicKey) (*Client, error) {
	key := lruCacheKey{host: host, port: port, hostPk: hostPk}
	if val, ok := hc.cache.Get(key); ok {
		cli, ok := val.(*Client)
		if !ok {
			panic("failed to convert value to client type")
		}
		return cli, nil
	}

	closingClient, err := closingclient.New(modules.GetHostApiClient(hostPk))
	if err != nil {
		return nil, fmt.Errorf("failed to create closing wrapper for client: %w", err)
	}
	cli, err := HostClientFromHttpClient(host, port, closingClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create api2 client: %w", err)
	}
	hc.cache.ContainsOrAdd(key, cli)
	return cli, nil
}

func (hc *HostClienter) client(host, port string, hostPk types.SiaPublicKey) (*Client, error) {
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

// Client returns client by public key.
func (hc *HostClienter) Client(host, port string, hostPk types.SiaPublicKey) (*Client, error) {
	if hc.cache != nil {
		return hc.clientFromLRU(host, port, hostPk)
	}
	return hc.client(host, port, hostPk)
}

// HostClientFromHttpClient creates host API client initialized with provided http.Client.
func HostClientFromHttpClient(host, port string, client api2.HttpClient) (*Client, error) {
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
