package filesystem

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"gitlab.com/NebulousLabs/Sia/build"
	"gitlab.com/NebulousLabs/Sia/crypto"
	"gitlab.com/NebulousLabs/Sia/modules"
	"gitlab.com/NebulousLabs/Sia/modules/renter/siadir"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/NebulousLabs/writeaheadlog"
)

var (
	// ErrNotExist is returned when a file or folder can't be found on disk.
	ErrNotExist = errors.New("path does not exist")

	// ErrExists is returned when a file or folder already exists at a given
	// location.
	ErrExists = errors.New("a file or folder already exists at the specified path")
)

type (
	// FileSystem implements a thread-safe filesystem for Sia for loading
	// SiaFiles, SiaDirs and potentially other supported Sia types in the
	// future.
	FileSystem struct {
		dNode
	}

	// node is a struct that contains the commmon fields of every node.
	node struct {
		staticParent *dNode
		staticName   string
		staticWal    *writeaheadlog.WAL
		threads      map[threadUID]threadInfo
		threadUID    threadUID
		mu           *sync.Mutex
	}

	// threadInfo contains useful information about the thread accessing the
	// SiaDirSetEntry
	threadInfo struct {
		callingFiles []string
		callingLines []int
		lockTime     time.Time
	}

	threadUID uint64
)

// newNode is a convenience function to initialize a node.
func newNode(parent *dNode, name string, uid threadUID, wal *writeaheadlog.WAL) node {
	return node{
		staticParent: parent,
		staticName:   name,
		staticWal:    wal,
		threads:      make(map[threadUID]threadInfo),
		threadUID:    uid,
		mu:           new(sync.Mutex),
	}
}

// newThreadType created a threadInfo entry for the threadMap
func newThreadType() threadInfo {
	tt := threadInfo{
		callingFiles: make([]string, threadDepth+1),
		callingLines: make([]int, threadDepth+1),
		lockTime:     time.Now(),
	}
	for i := 0; i <= threadDepth; i++ {
		_, tt.callingFiles[i], tt.callingLines[i], _ = runtime.Caller(2 + i)
	}
	return tt
}

// newThreadUID returns a random threadUID to be used as the threadUID in the
// threads map of the node.
func newThreadUID() threadUID {
	return threadUID(fastrand.Uint64n(math.MaxUint64))
}

// close removes a thread from the node's threads map. This should only be
// called from within other 'close' methods.
func (n *node) _close() {
	if _, exists := n.threads[n.threadUID]; !exists {
		build.Critical("threaduid doesn't exist in threads map: ", n.threadUID, len(n.threads))
	}
	delete(n.threads, n.threadUID)
}

// staticPath returns the absolute path of the node on disk.
func (n *node) staticPath() string {
	path := n.staticName
	for parent := n.staticParent; parent != nil; parent = parent.staticParent {
		path = filepath.Join(parent.staticName, path)
	}
	return path
}

// New creates a new FileSystem at the specified root path. The folder will be
// created if it doesn't exist already.
func New(root string, wal *writeaheadlog.WAL) (*FileSystem, error) {
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return nil, errors.AddContext(err, "failed to create root dir")
	}
	return &FileSystem{
		dNode: dNode{
			// The root doesn't require a parent, the name is its absolute path for convenience and it doesn't require a uid.
			node:        newNode(nil, root, 0, wal),
			directories: make(map[string]*dNode),
			files:       make(map[string]*fNode),
		},
	}, nil
}

// DeleteDir deletes a dir from the filesystem. The dir will be marked as
// 'deleted' which should cause all remaining instances of the dir to be close
// shortly. Only when all instances of the dir are closed it will be removed
// from the tree. This means that as long as the deletion is in progress, no new
// file of the same path can be created and the existing file can't be opened
// until all instances of it are closed.
func (fs *FileSystem) DeleteDir(siaPath modules.SiaPath) error {
	return fs.managedDeleteDir(siaPath.String())
}

// DeleteFile deletes a file from the filesystem. The file will be marked as
// 'deleted' which should cause all remaining instances of the file to be closed
// shortly. Only when all instances of the file are closed it will be removed
// from the tree. This means that as long as the deletion is in progress, no new
// file of the same path can be created and the existing file can't be opened
// until all instances of it are closed.
func (fs *FileSystem) DeleteFile(siaPath modules.SiaPath) error {
	return fs.managedDeleteFile(siaPath.String())
}

// NewSiaDir creates the folder for the specified siaPath. This doesn't create
// the folder metadata since that will be created on demand as the individual
// folders are accessed.
func (fs *FileSystem) NewSiaDir(siaPath modules.SiaPath) error {
	dirPath := siaPath.SiaDirSysPath(fs.staticName)
	_, err := siadir.New(dirPath, fs.staticName, fs.staticWal)
	if os.IsExist(err) {
		return nil // nothing to do
	}
	return err
}

// NewSiaFile creates a SiaFile at the specified siaPath.
func (fs *FileSystem) NewSiaFile(siaPath modules.SiaPath, source string, ec modules.ErasureCoder, mk crypto.CipherKey, fileSize uint64, fileMode os.FileMode, disablePartialUpload bool) error {
	// Create SiaDir for file.
	dirSiaPath, err := siaPath.Dir()
	if err != nil {
		return err
	}
	if err := fs.NewSiaDir(dirSiaPath); err != nil {
		return errors.AddContext(err, fmt.Sprintf("failed to create SiaDir %v for SiaFile %v", dirSiaPath.String(), siaPath.String()))
	}
	return fs.managedNewSiaFile(siaPath.String(), source, ec, mk, fileSize, fileMode, disablePartialUpload)
}

// OpenSiaDir opens a SiaDir and adds it and all of its parents to the
// filesystem tree.
func (fs *FileSystem) OpenSiaDir(siaPath modules.SiaPath) (*dNode, error) {
	return fs.dNode.managedOpenDir(siaPath.String())
}

// OpenSiaFile opens a SiaFile and adds it and all of its parents to the
// filesystem tree.
func (fs *FileSystem) OpenSiaFile(siaPath modules.SiaPath) (*fNode, error) {
	return fs.managedOpenFile(siaPath.String())
}

// managedDeleteFile opens the parent folder of the file to delete and calls
// managedDeleteFile on it.
func (fs *FileSystem) managedDeleteFile(path string) error {
	// Open the folder that contains the file.
	dirPath, fileName := filepath.Split(path)
	var dir *dNode
	if dirPath == string(filepath.Separator) || dirPath == "." || dirPath == "" {
		dir = &fs.dNode // file is in the root dir
	} else {
		var err error
		dir, err = fs.managedOpenDir(filepath.Dir(path))
		if err != nil {
			return errors.AddContext(err, "failed to open parent dir of file")
		}
		// Close the dir since we are not returning it. The open file keeps it
		// loaded in memory.
		defer dir.Close()
	}
	return dir.managedDeleteFile(fileName)
}

// managedDeleteDir opens the parent folder of the dir to delete and calls
// managedDelete on it.
func (fs *FileSystem) managedDeleteDir(path string) error {
	// Open the folder that contains the file.
	dirPath, _ := filepath.Split(path)
	var dir *dNode
	if dirPath == string(filepath.Separator) || dirPath == "." || dirPath == "" {
		dir = &fs.dNode // file is in the root dir
	} else {
		var err error
		dir, err = fs.managedOpenDir(filepath.Dir(path))
		if err != nil {
			return errors.AddContext(err, "failed to open parent dir of file")
		}
		// Close the dir since we are not returning it. The open file keeps it
		// loaded in memory.
		defer dir.Close()
	}
	return dir.managedDelete()
}

// managedOpenFile opens a SiaFile and adds it and all of its parents to the
// filesystem tree.
func (fs *FileSystem) managedOpenFile(path string) (*fNode, error) {
	// Open the folder that contains the file.
	dirPath, fileName := filepath.Split(path)
	var dir *dNode
	if dirPath == string(filepath.Separator) || dirPath == "." || dirPath == "" {
		dir = &fs.dNode // file is in the root dir
	} else {
		var err error
		dir, err = fs.managedOpenDir(filepath.Dir(path))
		if err != nil {
			return nil, errors.AddContext(err, "failed to open parent dir of file")
		}
		// Close the dir since we are not returning it. The open file keeps it
		// loaded in memory.
		defer dir.Close()
	}
	return dir.managedOpenFile(fileName)
}

// managedNewSiaFile opens the parent folder of the new SiaFile and calls
// managedNewSiaFile on it.
func (fs *FileSystem) managedNewSiaFile(path string, source string, ec modules.ErasureCoder, mk crypto.CipherKey, fileSize uint64, fileMode os.FileMode, disablePartialUpload bool) error {
	// Open the folder that contains the file.
	dirPath, fileName := filepath.Split(path)
	var dir *dNode
	if dirPath == string(filepath.Separator) || dirPath == "." || dirPath == "" {
		dir = &fs.dNode // file is in the root dir
	} else {
		var err error
		dir, err = fs.managedOpenDir(filepath.Dir(path))
		if err != nil {
			return errors.AddContext(err, "failed to open parent dir of new file")
		}
		defer dir.Close()
	}
	return dir.managedNewSiaFile(fileName, source, ec, mk, fileSize, fileMode, disablePartialUpload)
}
