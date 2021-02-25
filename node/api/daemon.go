package api

//TODO: Enable upgrading
import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/crypto/openpgp"
	//"archive/zip"
	//"bytes"
	//"crypto/sha256"
	"encoding/json"
	"fmt"

	//"io"
	//"io/ioutil"
	"math/big"
	"net/http"

	//"path"
	//"path/filepath"
	//"runtime"
	//"strings"

	"github.com/inconshreveable/go-update"
	"github.com/julienschmidt/httprouter"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
)

const (
	// The developer key is used to sign updates and other important ScPrime-
	// related information.
	developerKey = `-----BEGIN PUBLIC KEY-----
	MIIEIjANBgkqhkiG9w0BAQEFAAOCBA8AMIIECgKCBAEA05U9xFO9EgKEUQ5LzLkD
	//iODWzBY+bKWEMial0hxHnFU/FD0DOqpjvbZV8NgF0rFZu6MznN+UO8+a8EqGiq
	L3XkUmnJu9K2sWOJOv9aYYbFQwxay1gI+jIFsldjTVf7yOz+9IcM48zyX0A952Fx
	y5ugctqr7Sr+2uuLAqXXyWwA8FQ5ZPWOzCySJCbcaKSP4fP+caRKCUeAci9CON6g
	UxIGgO3H5KjBa1nt38XE4Qt7hy7peIRigmfm3FJkSnqWijkYUXrPIwILfqaFMknZ
	40Yc5GEqxOdzSYoptWXfVdnoDvu58qJI5ikAwr/pIjVCSn3sdMudlDiRoiUfHCYn
	T7OEq6wWSL/lmpOKSXavIxUwfs4mpCA/a9E0P1FICLr52DNrzVVveEXmkzUEtqlF
	26FssMzTt6mkmAF8T7HRBtVyuTt+UD2I7oEJ29CPV0U7YE5KVhmQG7ae3i6k2csJ
	mEdDNbb0rOdZrsygyfJNkG/IkD09PTn9KteTDc9ULnEhq2LXtot0y1/zas1z8BsC
	rJbAdGU0o5/SpWPfOrw2O/aa0Ja413cMyGAXUZRiseTncLod9dKn9Y6GVIb3LSj7
	D3FfV2ADG2vdpuBc18JsPDC+oLWjIjpEZKlyr3tqyQsE3lJjWteU2W+Tju4PWM9Q
	GT/e9SY9DcFKTY8AIxrtzyuCGwuW5a3yQ9FP2yEDpF7j2POIU7LIEtnyn9oXP/6d
	3y4X+DJhkDm4cKfV+OsVyCYwK3+IQvr1g1F2N/xsKByZbb4Jgwge5Libtrnc0GH8
	WxdIlbNBHHuU58yI0GjPnFLqRMe0duPzJXWtq18RJFjUHg8muhb14lBuXOqaF0cs
	gn+8izPgbSMhEHPQBhT9Ux+byU3qDjKqim3f5xtZNH6R2eT58N8y5CwXYYodpMVL
	nhcX29QsqqpO1mZQwx6snSJ6QDvmcSrhEgFhWLAbUY6R+I0UWGY23dlI/cNEWOyu
	zRSS/DDK0gV7+pPFT2+HiP6YkZ4qlHqmWIWXBFeXSAEzVNe2AG48BD3sXQGlh7Jd
	x73jHo/17DZUivnQ8MyvOfDY1ThfwSNlCJzfYvQUxpa43JAZzCGxEDw7pJvrHqLD
	cV75Zk12DzRBoC4neDRWd4qfXApx066Ew56xxeCQI5Z5suZ1juaejpr9BDshHyE4
	Abt4U3mhMBtcdD4Oz0mo+iuXLePbdo3uwCGFE7GOb+XdxvegYzTE0w/24mXNluz8
	BiFzhF0Dgq2SmUkVpqYehZNmfm61/xtyOM5uHOtphetSdA5lPaplSiLzi6piaaKa
	huUdkWLWC9t9M+VuEZhk0ra/x7b1HDy4bLqBaXzJtKvZyRlOBWBFmIVmctygG6t9
	ZwIDAQAB
	-----END PUBLIC KEY-----`
)

type (
	// DaemonAlertsGet contains information about currently registered alerts
	// across all loaded modules.
	DaemonAlertsGet struct {
		Alerts         []modules.Alert `json:"alerts"`
		CriticalAlerts []modules.Alert `json:"criticalalerts"`
		ErrorAlerts    []modules.Alert `json:"erroralerts"`
		WarningAlerts  []modules.Alert `json:"warningalerts"`
	}

	// DaemonVersionGet contains information about the running daemon's version.
	DaemonVersionGet struct {
		Version     string
		GitRevision string
		BuildTime   string
	}

	// DaemonUpdateGet contains information about a potential available update for
	// the daemon.
	DaemonUpdateGet struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
	}

	// UpdateInfo indicates whether an update is available, and to what
	// version.
	UpdateInfo struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
	}
	// gitlabRelease represents some of the JSON returned by the GitLab
	// release API endpoint. Only the fields relevant to updating are
	// included.
	gitlabRelease struct {
		Name string `json:"name"`
	}

	// SiaConstants is a struct listing all of the constants in use.
	SiaConstants struct {
		BlockFrequency         types.BlockHeight `json:"blockfrequency"`
		BlockSizeLimit         uint64            `json:"blocksizelimit"`
		ExtremeFutureThreshold types.Timestamp   `json:"extremefuturethreshold"`
		FutureThreshold        types.Timestamp   `json:"futurethreshold"`
		GenesisTimestamp       types.Timestamp   `json:"genesistimestamp"`
		MaturityDelay          types.BlockHeight `json:"maturitydelay"`
		MedianTimestampWindow  uint64            `json:"mediantimestampwindow"`
		SiafundCount           types.Currency    `json:"siafundcount"`
		SiafundPortion         *big.Rat          `json:"siafundportion"`
		TargetWindow           types.BlockHeight `json:"targetwindow"`

		InitialCoinbase uint64 `json:"initialcoinbase"`
		MinimumCoinbase uint64 `json:"minimumcoinbase"`

		RootTarget types.Target `json:"roottarget"`
		RootDepth  types.Target `json:"rootdepth"`

		DefaultAllowance modules.Allowance `json:"defaultallowance"`

		// DEPRECATED: same values as MaxTargetAdjustmentUp and
		// MaxTargetAdjustmentDown.
		MaxAdjustmentUp   *big.Rat `json:"maxadjustmentup"`
		MaxAdjustmentDown *big.Rat `json:"maxadjustmentdown"`

		MaxTargetAdjustmentUp   *big.Rat `json:"maxtargetadjustmentup"`
		MaxTargetAdjustmentDown *big.Rat `json:"maxtargetadjustmentdown"`

		// SiacoinPrecision is the number of base units in a siacoin. This constant is used
		// for mining rewards calculation and supported for compatibility with
		// existing 3rd party intergations.
		// DEPRECATED: Since February 2020 one scprimecoin equals 10^27 Hastings
		// Use the types.ScPrimecoinPrecision constant.
		SiacoinPrecision types.Currency `json:"siacoinprecision"`
		// ScPrimecoinPrecision is the number of base units in a scprimecoin that is used
		// by clients (1 SCP = 10^27 H).
		ScPrimecoinPrecision types.Currency `json:"scprimecoinprecision"`
	}

	// DaemonStackGet contains information about the daemon's stack.
	DaemonStackGet struct {
		Stack []byte `json:"stack"`
	}

	// DaemonSettingsGet contains information about global daemon settings.
	DaemonSettingsGet struct {
		MaxDownloadSpeed int64         `json:"maxdownloadspeed"`
		MaxUploadSpeed   int64         `json:"maxuploadspeed"`
		Modules          configModules `json:"modules"`
	}

	// DaemonVersion holds the version information for spd
	DaemonVersion struct {
		Version     string `json:"version"`
		GitRevision string `json:"gitrevision"`
		BuildTime   string `json:"buildtime"`
	}
)

// fetchLatestRelease returns metadata about the most recent GitLab release.
func fetchLatestRelease() (gitlabRelease, error) {
	resp, err := http.Get("https://gitlab.com/api/v4/projects/17421950/repository/tags?order_by=name")
	if err != nil {
		return gitlabRelease{}, err
	}
	defer resp.Body.Close()
	var releases []gitlabRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return gitlabRelease{}, err
	} else if len(releases) == 0 {
		return gitlabRelease{}, errors.New("no releases found")
	}

	// Find the most recent release that is not a nightly or release candidate.
	for _, release := range releases {
		if build.IsVersion(release.Name[1:]) && release.Name[0] == 'v' {
			return release, nil
		}
	}
	return gitlabRelease{}, errors.New("No non-nightly or non-RC releases found")
}

// updateToRelease updates spd and spc to the release specified. spc is
// assumed to be in the same folder as spd.
func updateToRelease(version string) error {
	usr, err := user.Current()
	if err != nil {
		return err
	}
	binaryFolder := usr.HomeDir + "/go/bin/"
	// trim release version for generate correct URL in request, ex v1.5.1 -> 1.5.1
	version = strings.Trim(version, "v")

	// Download file of signed hashes.
	resp, err := http.Get(fmt.Sprintf("https://releases.scpri.me/%s/ScPrime-v%s-%s-%s.zip.asc", version, version,
		runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return err
	}
	// The file should be small enough to store in memory (<1 MiB); use
	// MaxBytesReader to ensure we don't download more than 8 MiB
	signatureBytes, err := ioutil.ReadAll(http.MaxBytesReader(nil, resp.Body, 1<<23))
	resp.Body.Close()
	if err != nil {
		return err
	}
	// Open the developer key for verifying signatures.
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(developerKey))
	if err != nil {
		return errors.AddContext(err, "Error reading keyring")
	}

	// download release archive
	releaseFilePrefix := fmt.Sprintf("ScPrime-v%s-%s-%s", version, runtime.GOOS, runtime.GOARCH)
	zipResp, err := http.Get(fmt.Sprintf("https://releases.scpri.me/%s/%s.zip", version, releaseFilePrefix))
	if err != nil {
		return err
	}
	// release should be small enough to store in memory (<10 MiB); use
	// LimitReader to ensure we don't download more than 32 MiB
	content, err := ioutil.ReadAll(http.MaxBytesReader(nil, zipResp.Body, 1<<25))
	zipResp.Body.Close()
	if err != nil {
		return err
	}

	// verify release with signature and developer key
	_, err = openpgp.CheckArmoredDetachedSignature(keyring, bytes.NewBuffer(content), bytes.NewBuffer(signatureBytes))
	if err != nil {
		return errors.AddContext(err, "release verification error")
	}
	r := bytes.NewReader(content)
	z, err := zip.NewReader(r, r.Size())
	if err != nil {
		return err
	}

	// Process zip, finding spd/spc binaries and validate the checksum against
	// the signed checksums file.
	for _, binary := range []string{"spd", "spc"} {
		var binData io.ReadCloser
		var binaryName string // needed for TargetPath below
		for _, zf := range z.File {
			fileName := path.Base(zf.Name)
			if (fileName != binary) && (fileName != binary+".exe") {
				continue
			}
			binaryName = fileName
			binData, err = zf.Open()
			if err != nil {
				return err
			}
			defer binData.Close()
		}
		if binData == nil {
			return errors.New("could not find " + binary + " binary")
		}

		// Verify the checksum matches the signed checksum.
		// Use io.LimitReader to ensure we don't download more than 32 MiB
		binaryBytes, err := ioutil.ReadAll(io.LimitReader(binData, 1<<25))
		if err != nil {
			return err
		}
		// binData (an io.ReadCloser) is still needed to update the binary.
		binData = ioutil.NopCloser(bytes.NewBuffer(binaryBytes))

		updateOpts := update.Options{
			Signature:  nil,  // Signature verification is skipped because we already verified the signature of the checksum.
			TargetMode: 0775, // executable
			TargetPath: filepath.Join(binaryFolder, binaryName),
		}

		// apply update
		err = update.Apply(binData, updateOpts)
		if err != nil {
			return err
		}
	}
	return nil
}

// daemonAlertsHandlerGET handles the API call that returns the alerts of all
// loaded modules.
func (api *API) daemonAlertsHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// initialize slices to avoid "null" in response.
	crit := make([]modules.Alert, 0, 6)
	err := make([]modules.Alert, 0, 6)
	warn := make([]modules.Alert, 0, 6)
	if api.gateway != nil {
		c, e, w := api.gateway.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.cs != nil {
		c, e, w := api.cs.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.tpool != nil {
		c, e, w := api.tpool.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.wallet != nil {
		c, e, w := api.wallet.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.renter != nil {
		c, e, w := api.renter.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	if api.host != nil {
		c, e, w := api.host.Alerts()
		crit = append(crit, c...)
		err = append(err, e...)
		warn = append(warn, w...)
	}
	// Sort alerts by severity. Critical first, then Error and finally Warning.
	alerts := append(crit, append(err, warn...)...)
	WriteJSON(w, DaemonAlertsGet{
		Alerts:         alerts,
		CriticalAlerts: crit,
		ErrorAlerts:    err,
		WarningAlerts:  warn,
	})
}

// daemonUpdateHandlerGET handles the API call that checks for an update.
func (api *API) daemonUpdateHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		WriteError(w, Error{Message: "Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	latestVersion := release.Name[1:] // delete leading 'v'
	WriteJSON(w, UpdateInfo{
		Available: build.VersionCmp(latestVersion, build.Version) > 0,
		Version:   latestVersion,
	})
}

// daemonUpdateHandlerPOST handles the API call that updates spd and spc.
// There is no safeguard to prevent "updating" to the same release, so callers
// should always check the latest version via daemonUpdateHandlerGET first.
// TODO: add support for specifying version to update to.
func (api *API) daemonUpdateHandlerPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	release, err := fetchLatestRelease()
	if err != nil {
		WriteError(w, Error{Message: "Failed to fetch latest release: " + err.Error()}, http.StatusInternalServerError)
		return
	}
	err = updateToRelease(release.Name)
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			WriteError(w, Error{Message: "Serious error: Failed to rollback from bad update: " + rerr.Error()}, http.StatusInternalServerError)
		} else {
			WriteError(w, Error{Message: "Failed to apply update: " + err.Error()}, http.StatusInternalServerError)
		}
		return
	}
	WriteSuccess(w)
}

// debugConstantsHandler prints a json file containing all of the constants.
func (api *API) daemonConstantsHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	sc := SiaConstants{
		BlockFrequency:         types.BlockFrequency,
		BlockSizeLimit:         types.BlockSizeLimit,
		ExtremeFutureThreshold: types.ExtremeFutureThreshold,
		FutureThreshold:        types.FutureThreshold,
		GenesisTimestamp:       types.GenesisTimestamp,
		MaturityDelay:          types.MaturityDelay,
		MedianTimestampWindow:  types.MedianTimestampWindow,
		SiafundCount:           types.SiafundCount(api.cs.Height()),
		SiafundPortion:         types.SiafundPortion(api.cs.Height()),
		TargetWindow:           types.TargetWindow,

		InitialCoinbase: types.InitialCoinbase,
		MinimumCoinbase: types.MinimumCoinbase,

		RootTarget: types.RootTarget,
		RootDepth:  types.RootDepth,

		DefaultAllowance: modules.DefaultAllowance,

		// DEPRECATED: same values as MaxTargetAdjustmentUp and
		// MaxTargetAdjustmentDown.
		MaxAdjustmentUp:   types.MaxTargetAdjustmentUp,
		MaxAdjustmentDown: types.MaxTargetAdjustmentDown,

		MaxTargetAdjustmentUp:   types.MaxTargetAdjustmentUp,
		MaxTargetAdjustmentDown: types.MaxTargetAdjustmentDown,

		SiacoinPrecision:     types.SiacoinPrecision,
		ScPrimecoinPrecision: types.ScPrimecoinPrecision,
	}

	WriteJSON(w, sc)
}

// daemonStackHandlerGET handles the API call that requests the daemon's stack trace.
func (api *API) daemonStackHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// Get the stack traces of all running goroutines.
	stack := make([]byte, modules.StackSize)
	n := runtime.Stack(stack, true)
	if n == 0 {
		WriteError(w, Error{"no stack trace pulled"}, http.StatusInternalServerError)
		return
	}

	// Return the n bytes of the stack that were used.
	WriteJSON(w, DaemonStackGet{
		Stack: stack[:n],
	})
}

// daemonVersionHandler handles the API call that requests the daemon's version.
func (api *API) daemonVersionHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	version := build.Version
	if build.ReleaseTag != "" {
		version += "-" + build.ReleaseTag
	}
	WriteJSON(w, DaemonVersion{Version: version, GitRevision: build.GitRevision, BuildTime: build.BuildTime})
}

// daemonStopHandler handles the API call to stop the daemon cleanly.
func (api *API) daemonStopHandler(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	// can't write after we stop the server, so lie a bit.
	WriteSuccess(w)

	// Shutdown in a separate goroutine to prevent a deadlock.
	go func() {
		if err := api.Shutdown(); err != nil {
			build.Critical(err)
		}
	}()
}

// daemonSettingsHandlerGET handles the API call asking for the daemon's
// settings.
func (api *API) daemonSettingsHandlerGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	gmds, gmus, _ := modules.GlobalRateLimits.Limits()
	WriteJSON(w, DaemonSettingsGet{
		MaxDownloadSpeed: gmds,
		MaxUploadSpeed:   gmus,
		Modules:          api.staticConfigModules,
	})
}

// daemonSettingsHandlerPOST handles the API call changing daemon specific
// settings.
func (api *API) daemonSettingsHandlerPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	maxDownloadSpeed, maxUploadSpeed, _ := modules.GlobalRateLimits.Limits()
	// Scan the download speed limit. (optional parameter)
	if d := req.FormValue("maxdownloadspeed"); d != "" {
		var downloadSpeed int64
		if _, err := fmt.Sscan(d, &downloadSpeed); err != nil {
			WriteError(w, Error{"unable to parse downloadspeed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		maxDownloadSpeed = downloadSpeed
	}
	// Scan the upload speed limit. (optional parameter)
	if u := req.FormValue("maxuploadspeed"); u != "" {
		var uploadSpeed int64
		if _, err := fmt.Sscan(u, &uploadSpeed); err != nil {
			WriteError(w, Error{"unable to parse uploadspeed: " + err.Error()}, http.StatusBadRequest)
			return
		}
		maxUploadSpeed = uploadSpeed
	}
	// Set the limit.
	if err := api.spdConfig.SetRatelimit(maxDownloadSpeed, maxUploadSpeed); err != nil {
		WriteError(w, Error{"unable to set limits: " + err.Error()}, http.StatusBadRequest)
		return
	}
	WriteSuccess(w)
}
