package api

import (
	"bytes"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/starius/api2"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules/host/tokenstorage"
	"gitlab.com/scpcorp/ScPrime/types"
)

// TokenStorage represent communication between api and token storage.
type TokenStorage interface {
	TokenRecord(id types.TokenID) (tokenstorage.TokenRecord, error)
	RecordDownload(id types.TokenID, downloadBytes, sectorAccesses int64, time time.Time) (tokenstorage.TokenRecord, error)
	AddSectors(id types.TokenID, sectorsIDs []crypto.Hash, time time.Time) (tokenstorage.TokenRecord, error)
	ListSectorIDs(id types.TokenID, pageID string, limit int) (sectorIDs []crypto.Hash, nextPageID string, err error)
	RemoveSpecificSectors(id types.TokenID, sectorsIDs []crypto.Hash, time time.Time) error
	AttachSectors(tokensSectors map[types.TokenID][]crypto.Hash, time time.Time) error
	EnoughStorageResource(id types.TokenID, sectorsNum int64, now time.Time) (bool, error)
}

// Host represent host interface.
type Host interface {
	BlockHeight() types.BlockHeight
	AddSector(sectorRoot crypto.Hash, sectorData []byte) error
	RemoveSectorBatch(sectorRoots []crypto.Hash) error
	ReadSector(sectorRoot crypto.Hash) ([]byte, error)
	MoveTokenSectorsToStorageObligation(fcID types.FileContractID, newRev types.FileContractRevision, sectorsWithTokens []types.SectorWithToken, renterSig []byte) ([]byte, error)
}

// API represent host API.
type API struct {
	host       Host
	hostSK     crypto.SecretKey
	ts         TokenStorage
	httpServer *http.Server
}

// NewAPI return new host API.
func NewAPI(ts TokenStorage, hostSK crypto.SecretKey, host Host) *API {
	return &API{
		hostSK: hostSK,
		host:   host,
		ts:     ts,
	}
}

// Start run API.
func (a *API) Start(ln net.Listener) (err error) {
	routes := GetRoutes(a)
	mux := http.NewServeMux()
	api2.BindRoutes(mux, routes)
	cert := a.certSetup()

	a.httpServer = &http.Server{
		Handler: mux,
		// Mitigate Potential Slowloris Attack
		ReadHeaderTimeout: time.Minute / 2,
		TLSConfig: &tls.Config{ //nolint:gosec
			Certificates: []tls.Certificate{*cert},
		},
	}

	go func() {
		if err := a.httpServer.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	return nil
}

func (a *API) certSetup() *tls.Certificate {
	sk := ed25519.PrivateKey(a.hostSK[:])
	pk := sk.Public()

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber := fastrand.BigIntn(serialNumberLimit)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{""},
		},
		NotBefore:   time.Now().Add(-time.Hour * 24 * 360 * 10),
		NotAfter:    time.Now().Add(time.Hour * 24 * 360 * 10),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))

	derBytes, err := x509.CreateCertificate(fastrand.Reader, &template, &template, pk, sk)
	if err != nil {
		panic(err)
	}
	var certBuffer, keyBuffer bytes.Buffer
	if err := pem.Encode(&certBuffer, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		panic(err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(sk)
	if err != nil {
		panic(err)
	}
	if err := pem.Encode(&keyBuffer, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		panic(err)
	}
	cert, err := tls.X509KeyPair(certBuffer.Bytes(), keyBuffer.Bytes())
	if err != nil {
		panic(err)
	}
	return &cert
}

// Close stop API server.
func (a *API) Close() error {
	return a.httpServer.Close()
}
