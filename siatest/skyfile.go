package siatest

import (
	"bytes"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/node/api"
)

// UploadNewSkyfileBlocking attempts to upload a pubfile of given size. After it
// has successfully performed the upload, it will verify the file can be
// downloaded using its Publink. Returns the publink, the parameters used for
// the upload and potentially an error.
func (tn *TestNode) UploadNewSkyfileBlocking(filename string, filesize uint64, force bool) (publink string, sup modules.SkyfileUploadParameters, sshp api.SkynetSkyfileHandlerPOST, err error) {
	// create the siapath
	skyfilePath, err := modules.NewSiaPath(filename)
	if err != nil {
		err = errors.AddContext(err, "Failed to create siapath")
		return
	}

	// create random data and wrap it in a reader
	data := fastrand.Bytes(int(filesize))
	reader := bytes.NewReader(data)
	sup = modules.SkyfileUploadParameters{
		SiaPath:             skyfilePath,
		BaseChunkRedundancy: 2,
		FileMetadata: modules.SkyfileMetadata{
			Filename: filename,
			Mode:     modules.DefaultFilePerm,
		},
		Reader: reader,
		Force:  force,
		Root:   false,
	}

	// upload a pubfile
	publink, sshp, err = tn.SkynetSkyfilePost(sup)
	if err != nil {
		err = errors.AddContext(err, "Failed to upload pubfile")
		return
	}

	if !sup.Root {
		skyfilePath, err = modules.SkynetFolder.Join(skyfilePath.String())
		if err != nil {
			err = errors.AddContext(err, "Failed to rebase pubfile path")
			return
		}
	}
	rf := &RemoteFile{
		checksum: crypto.HashBytes(data),
		siaPath:  skyfilePath,
		root:     true,
	}

	// Wait until upload reached the specified progress
	if err = tn.WaitForUploadProgress(rf, 1); err != nil {
		err = errors.AddContext(err, "Pubfile upload failed, progress did not reach a value of 1")
		return
	}

	// wait until upload reaches a certain health
	if err = tn.WaitForUploadHealth(rf); err != nil {
		err = errors.AddContext(err, "Pubfile upload failed, health did not reach the repair threshold")
		return
	}

	return
}
