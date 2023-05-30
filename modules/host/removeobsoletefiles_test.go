package host

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/scpcorp/ScPrime/modules"
)

// TestRemoveObsoletedFiles checks that the host can remove ephemeral accounts files
func TestRemoveObsoletedFiles(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = ht.Close()
		if err != nil {
			t.Fatal(err)
		}
	}()
	tdir := filepath.Join(ht.persistDir, modules.HostDir)

	err = removeObsoletedFiles(tdir)
	if err != nil {
		t.Errorf("Running removeObsoletedFiles on the affected files not existing shouldn't return error but got %v", err)
	}
	//create accounts.dat
	accountsdat := filepath.Join(tdir, "accounts.dat")
	acdf, err := os.Create(accountsdat)
	if err != nil {
		t.Fatalf("Cannot create %v for removal test: %v", accountsdat, err)
	}
	_, err = acdf.WriteString("some contents of accounts.dat for testing")
	if err != nil {
		t.Fatalf("Error writing to accounts.dat file: %v", err)
	}
	// On linux removing open files doesn't fail
	//err = removeObsoletedFiles(tdir)
	//if err == nil {
	//	t.Errorf("Running removeObsoletedFiles on not closed accounts.dat file should return error but got none")
	//}
	acdf.Close()
	err = removeObsoletedFiles(tdir)
	if err != nil {
		t.Errorf("Running removeObsoletedFiles on existing and closed accounts.dat shouldn't return error but got %v", err)
	}
	//Test fp directory removal
	fpd := filepath.Join(tdir, "fp")
	err = os.Mkdir(fpd, os.ModeDir)
	if err != nil {
		t.Fatalf("Cannot create %v for removal test: %v", accountsdat, err)
	}
	err = removeObsoletedFiles(tdir)
	if err != nil {
		t.Errorf("Running removeObsoletedFiles on existing empty fp directory shouldn't return error but got %v", err)
	}
	//create some files in the dir as well as in base persist dir
	fpd = filepath.Join(tdir, "fp")
	err = os.Mkdir(fpd, os.ModeExclusive|os.ModePerm)
	if err != nil {
		t.Fatalf("Cannot create %v for removal test: %v", fpd, err)
	}
	for i := 1; i < 6; i++ {
		fn := fmt.Sprintf("fingerprintsbucket_%v", i)
		fi, err := os.Create(filepath.Join(fpd, fn))
		if err != nil {
			t.Fatalf("Cannot create %v for removal test: %v", fn, err)
		}
		_, err = fi.WriteString(fn)
		if err != nil {
			t.Fatalf("Cannot write to %v for removal test: %v", fi.Name(), err)
		}
		err = fi.Close()
		if err != nil {
			t.Fatalf("Cannot close %v for removal test: %v", fi.Name(), err)
		}
	}

	fi, err := os.Create(filepath.Join(tdir, "fingerprintsbucket_6"))
	if err != nil {
		t.Fatalf("Cannot create fingerprintsbucket_6 for removal test: %v", err)
	}
	_, err = fi.WriteString("content of fingerprintsbucket_6")
	if err != nil {
		t.Fatalf("Cannot write to %v for removal test: %v", fi.Name(), err)
	}
	err = fi.Close()
	if err != nil {
		t.Fatalf("Cannot close %v for removal test: %v", fi.Name(), err)
	}

	fi, err = os.Create(filepath.Join(tdir, "fingerprintsbucket_232720-232739.db"))
	if err != nil {
		t.Fatalf("Cannot create fingerprintsbucket_232720-232739.db for removal test: %v", err)
	}
	_, err = fi.WriteString("content of fingerprintsbucket_232720-232739.db")
	if err != nil {
		t.Fatalf("Cannot write to %v for removal test: %v", fi.Name(), err)
	}
	err = fi.Close()
	if err != nil {
		t.Fatalf("Cannot close %v for removal test: %v", fi.Name(), err)
	}

	shouldremainfile := filepath.Join(tdir, "shouldremain.file")
	fi, err = os.Create(shouldremainfile)
	if err != nil {
		t.Fatalf("Cannot create shouldremain.file for removal test: %v", err)
	}
	_, err = fi.WriteString("This file should remain after the removal test")
	if err != nil {
		t.Fatalf("Cannot write to %v for removal test: %v", fi.Name(), err)
	}
	err = fi.Close()
	if err != nil {
		t.Fatalf("Cannot close %v for removal test: %v", fi.Name(), err)
	}

	//verify no accounts.dat, no fingerprintbuckets and no fp dir remains after running
	err = removeObsoletedFiles(tdir)
	if err != nil {
		t.Errorf("No error expected on removeObsoletedFiles but got %v", err)
	}
	_, err = os.Stat(accountsdat)
	if !os.IsNotExist(err) {
		t.Errorf("%v shouldn't be there after removeObsoleteFiles()", accountsdat)
	}
	_, err = os.Stat(fpd)
	if !os.IsNotExist(err) {
		t.Errorf("%v shouldn't be there after removeObsoleteFiles()", fpd)
	}
	fpfiles, err := filepath.Glob(filepath.Join(tdir, "fingerprintsbucket*"))
	if err != nil {
		t.Fatalf("Error searching for fingerprintbuskets files: %v", err)
	}
	if len(fpfiles) > 0 {
		t.Errorf("No fingerprintsbucket* files should be there anymore but see %v", fpfiles)
	}

	_, err = os.Stat(shouldremainfile)
	if err != nil {
		t.Errorf("%v should still be there after removeObsoleteFiles() but got error: %v", shouldremainfile, err)
	}
}
