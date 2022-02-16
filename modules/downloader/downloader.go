package downloader

import (
	"archive/zip"
	"errors"
	"fmt"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var status = "N/A"

// Downloader is a struct that tracks global settings
type Downloader struct {
}

// New creates a new Downloader.
func New(dataDir string) *Downloader {
	downloader := &Downloader{}
	downloader.bootstrap(dataDir)
	return downloader
}

// Progress returns the Downloader's progress as a percentage.
func Progress() string {
	return status
}

// Close will shut down the downloader, giving the module enough time to
// run any required closing routines.
func (downloader *Downloader) Close() error {
	return nil
}

// Download consensus from consensus.scpri.me.
func (downloader *Downloader) bootstrap(dataDir string) {
	consensusDir := filepath.Join(dataDir, modules.ConsensusDir)
	consensusDb := filepath.Join(consensusDir, consensus.DatabaseFilename)
	_, err := os.Stat(consensusDir)
	if errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(consensusDir, os.ModePerm)
	}
	if err != nil {
		// Unable to create the consensus directory.
		// Return early and let the consensus module create the directory.
		return
	}
	_, err = os.Stat(consensusDb)
	if !errors.Is(err, os.ErrNotExist) {
		// Consensus database already exists so there is no need to download it.
		return
	}
	size, err := consensusSize()
	if size == 0 || err != nil {
		// Do not download consensus-latest.zip because something is wrong.
		return
	}
	tmp, err := ioutil.TempFile(os.TempDir(), "scprime-consensus")
	if err != nil {
		// Unable to create the temporary file to download the consensus database to.
		// Return early and let the consensus module create the directory from scratch.
		return
	}
	defer os.Remove(tmp.Name())
	var sem = make(chan int, 1)
	sem <- 1
	go func(filepath string) {
		consensusDownload(filepath)
		<-sem
	}(tmp.Name())
	// Print out the progress so the end user knows the program is not stuck.
	status = `0%`
	for i := 1; true; i++ {
		downloader.progress(tmp.Name(), size)
		if len(sem) == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}
	status = `99%`
	decompress(tmp.Name(), consensusDb)
	status = `100%`
}

// Decompress the zip archive; move consensus.db to the destination.
func decompress(src string, dest string) {
	r, err := zip.OpenReader(src)
	if err != nil {
		return
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name != "consensus.db" {
			continue
		}
		outFile, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return
		}
		rc, err := f.Open()
		if err != nil {
			return
		}
		_, err = io.Copy(outFile, rc)
		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()
	}
}

// Returns the size of the latest consensus database in bytes.
func consensusSize() (int64, error) {
	resp, err := http.Head("https://consensus.scpri.me/releases/consensus-latest.zip")
	if err != nil {
		return 0, err
	}
	return resp.ContentLength, nil
}

// Downloads the consensus databse to a local file without loading the whole file into memory.
func consensusDownload(target string) error {
	// Get the data
	resp, err := http.Get("https://consensus.scpri.me/releases/consensus-latest.zip")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Create the file
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	defer out.Close()
	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

// Prints the progress of the download.
func (downloader *Downloader) progress(filepath string, size int64) {
	fi, _ := os.Stat(filepath)
	progress := int(float64(fi.Size()) / float64(size) * float64(100))
	if progress > 0 && progress < 99 {
		status = fmt.Sprintf("%d%%", progress)
	}
}
