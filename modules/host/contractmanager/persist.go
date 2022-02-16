package contractmanager

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/persist"
)

type (
	// savedStorageFolder contains fields that are saved automatically to disk
	// for each storage folder.
	savedStorageFolder struct {
		Index uint16
		Path  string
		Usage []uint64
	}

	// savedSettings contains fields that are saved atomically to disk inside
	// of the contract manager directory, alongside the WAL and log.
	savedSettings struct {
		SectorSalt     crypto.Hash
		StorageFolders map[uint16]savedStorageFolder
	}

	// savedSettings120 is the old version (1.2.0) to read when upgrading to map based instead of slice
	savedSettings120 struct {
		SectorSalt     crypto.Hash
		StorageFolders []savedStorageFolder
	}
)

// equals tests if all settings are equal between two savedSettings.
func (s *savedSettings) equals(sb savedSettings) bool {
	if s.SectorSalt != sb.SectorSalt || len(s.StorageFolders) != len(sb.StorageFolders) {
		return false
	}
	for i, sf := range s.StorageFolders {
		sfb, there := sb.StorageFolders[i]
		if !there {
			return false
		}

		if sf.Index != sfb.Index || sf.Path != sfb.Path || len(sf.Usage) != len(sfb.Usage) {
			return false
		}

		for u := range sf.Usage {
			if sf.Usage[u] != sfb.Usage[u] {
				return false
			}
		}
	}

	return true
}

// savedStorageFolder returns the persistent version of the storage folder.
func (sf *storageFolder) savedStorageFolder() savedStorageFolder {
	ssf := savedStorageFolder{
		Index: sf.index,
		Path:  sf.path,
		Usage: make([]uint64, len(sf.usage)),
	}
	copy(ssf.Usage, sf.usage)
	return ssf
}

// initSettings will set the default settings for the contract manager.
// initSettings should only be run for brand new contract maangers.
func (cm *ContractManager) initSettings() error {
	// Initialize the sector salt to a random value.
	rand.Read(cm.sectorSalt[:])

	// Ensure that the initialized defaults have stuck.
	ss := cm.savedSettings()
	settingspath := filepath.Join(cm.persistDir, settingsFile)
	cm.log.Debugf("Initializing contractmanager settings, saving to %v", settingspath)
	err := persist.SaveJSON(settingsMetadata, &ss, settingspath)
	if err != nil {
		cm.log.Println("ERROR: unable to initialize settings file for contract manager:", err)
		return build.ExtendErr("error saving contract manager after initialization", err)
	}
	return nil
}

// loadSettings will load the contract manager settings.
func (cm *ContractManager) loadSettings() error {
	settingspath := filepath.Join(cm.persistDir, settingsFile)
	var ss savedSettings
	err := cm.dependencies.LoadFile(settingsMetadata, &ss, settingspath)
	if err != nil {
		cm.log.Debugf("Loading contractmanager settings from %v returned error %v", settingspath, err.Error())
	}
	if os.IsNotExist(err) {
		// There is no settings file, this must be the first time that the
		// contract manager has been run. Initialize with default settings.
		return cm.initSettings()
	} else if errors.Is(err, persist.ErrBadHeader) || errors.Is(err, persist.ErrBadVersion) {
		//Try to load old version structure
		cm.log.Printf("Upgrading contractmanager settings")
		var ss120 savedSettings120
		err120 := cm.dependencies.LoadFile(settingsMetadata120, &ss120, filepath.Join(cm.persistDir, settingsFile))
		if err120 != nil {
			return fmt.Errorf("cannot upgrade contractmanager: %w", err120)
		}
		ss.SectorSalt = ss120.SectorSalt
		ss.StorageFolders = make(map[uint16]savedStorageFolder)
		for _, osf := range ss120.StorageFolders {
			ss.StorageFolders[osf.Index] = osf
		}
	} else if err != nil {
		cm.log.Printf("ERROR: unable to load the contract manager settings from %s : %v", settingspath, err.Error())
		return fmt.Errorf("failed to load contract manager settings file: %w", err)
	}
	cm.log.Debugf("Loading contractmanager settings from %v done, %v folders", settingspath, len(ss.StorageFolders))

	// Copy the saved settings into the contract manager.
	cm.sectorSalt = ss.SectorSalt
	for _, psf := range ss.StorageFolders {
		sf := new(storageFolder)
		sf.index = psf.Index
		sf.path = psf.Path
		sf.usage = psf.Usage
		sf.metadataFile, err = cm.dependencies.OpenFile(filepath.Join(psf.Path, metadataFile), os.O_RDWR, 0700)
		if err != nil {
			// Mark the folder as unavailable and log an error.
			atomic.StoreUint64(&sf.atomicUnavailable, 1)
			cm.log.Printf("ERROR: unable to open the %v sector metadata file: %v\n", sf.path, err)
		}
		sf.sectorFile, err = cm.dependencies.OpenFile(filepath.Join(psf.Path, sectorFile), os.O_RDWR, 0700)
		if err != nil {
			// Mark the folder as unavailable and log an error.
			atomic.StoreUint64(&sf.atomicUnavailable, 1)
			cm.log.Printf("ERROR: unable to open the %v sector file: %v\n", sf.path, err)
			if sf.metadataFile != nil {
				sf.metadataFile.Close()
			}
		}
		sf.availableSectors = make(map[sectorID]uint32)
		cm.storageFolders[sf.index] = sf
	}
	return nil
}

// loadSectorLocations will read the metadata portion of each storage folder
// file and load the sector location information into memory.
func (cm *ContractManager) loadSectorLocations(sf *storageFolder) {
	// Read the sector lookup table for this storage folder into memory.
	sectorLookupBytes, err := readFullMetadata(sf.metadataFile, len(sf.usage)*storageFolderGranularity)
	if err != nil {
		atomic.AddUint64(&sf.atomicFailedReads, 1)
		atomic.StoreUint64(&sf.atomicUnavailable, 1)
		err = build.ComposeErrors(err, sf.metadataFile.Close())
		err = build.ComposeErrors(err, sf.sectorFile.Close())
		cm.log.Printf("ERROR: unable to read sector metadata for folder %v: %v\n", sf.path, err)
		return
	}
	atomic.AddUint64(&sf.atomicSuccessfulReads, 1)

	// Iterate through the sectors that are in-use and read their storage
	// locations into memory.
	sf.sectors = 0 // may be non-zero from WAL operations - they will be double counted here if not reset.
	for _, sectorIndex := range usageSectors(sf.usage) {
		readHead := sectorMetadataDiskSize * sectorIndex
		var id sectorID
		copy(id[:], sectorLookupBytes[readHead:readHead+12])
		count := binary.LittleEndian.Uint16(sectorLookupBytes[readHead+12 : readHead+14])
		sl := sectorLocation{
			index:         sectorIndex,
			storageFolder: sf.index,
			count:         count,
		}

		// Add the sector to the sector location map.
		cm.sectorLocations[id] = sl
		sf.sectors++
	}
	atomic.StoreUint64(&sf.atomicUnavailable, 0)
}

// savedSettings returns the settings of the contract manager in an
// easily-serializable form.
func (cm *ContractManager) savedSettings() savedSettings {
	ss := savedSettings{
		SectorSalt:     cm.sectorSalt,
		StorageFolders: make(map[uint16]savedStorageFolder),
	}
	for _, sf := range cm.storageFolders {
		// Unset all of the usage bits in the storage folder for the queued sectors.
		for _, sectorIndex := range sf.availableSectors {
			sf.clearUsage(sectorIndex)
		}

		// Copy over the storage folder.
		//ss.StorageFolders = append(ss.StorageFolders, sf.savedStorageFolder())
		ss.StorageFolders[sf.index] = sf.savedStorageFolder()

		// Re-set all of the usage bits for the queued sectors.
		for _, sectorIndex := range sf.availableSectors {
			sf.setUsage(sectorIndex)
		}
	}
	return ss
}
