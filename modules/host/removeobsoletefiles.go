package host

import (
	"fmt"
	"os"
	"path/filepath"
)

// removeObsoletedFiles removes files related to ephemeral accounts from
// the host persistence dir
// persistDir/accounts.dat
// persistDir/fingerprintsbucket*
// persistDir/fp/*
func removeObsoletedFiles(persistDir string) error {
	dir, err := filepath.Abs(persistDir)
	if err != nil {
		return fmt.Errorf("Error parsing persistDir: %w", err)
	}

	//find the fingerprintbuckets files
	fpb, err := filepath.Glob(filepath.Join(dir, "fingerprintsbucket*"))
	if err != nil {
		return fmt.Errorf("Error searching for fingerprintbuskets files: %w", err)
	}
	//add the accounts.dat to the list of to removes
	fpb = append(fpb, filepath.Join(dir, "accounts.dat"))
	for _, f := range fpb {
		if err := os.Remove(f); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("Error removing %v: %w", f, err)
		}
	}

	//remove the old fp directory
	path := filepath.Join(dir, "fp")
	fpdir, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil {
		//It exists and should be removed
		if fpdir.IsDir() {
			err = os.RemoveAll(path)
		} else {
			err = fmt.Errorf("cannot remove %v as it is expected to be a directory", path)
		}
	}
	return err
}
