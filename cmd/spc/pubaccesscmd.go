package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v5"
	"github.com/vbauerster/mpb/v5/decor"
	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/renter/filesystem"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
)

var (
	skynetCmd = &cobra.Command{
		Use:   "pubaccess",
		Short: "Perform actions related to Pubaccess",
		Long: `Perform actions related to Pubaccess, a file sharing and data publication platform
on top of ScPrime.`,
		Run: skynetcmd,
	}

	skynetBlacklistCmd = &cobra.Command{
		Use:   "blacklist",
		Short: "Do actionson publink blacklist.",
		Long:  "Add, remove, or list blacklisted publinks.",
		Run:   skynetblacklistgetcmd,
	}

	skynetBlacklistAddCmd = &cobra.Command{
		Use:   "add [publink] ...",
		Short: "Add publinks to the blacklist",
		Long:  "Add space separated publinks to the blacklist.",
		Run:   skynetblacklistaddcmd,
	}

	skynetBlacklistRemoveCmd = &cobra.Command{
		Use:   "remove [publink] ...",
		Short: "Remove publinks from the blacklist",
		Long:  "Remove space separated publinks from the blacklist.",
		Run:   skynetblacklistremovecmd,
	}

	skynetConvertCmd = &cobra.Command{
		Use:   "convert [source siaPath] [destination siaPath]",
		Short: "Convert a siafile to a pubaccess file with a publink.",
		Long: `Convert a siafile to a public accessible file and then generate its publink. A new publink
	will be created in the user's pubfile directory. The pubfile and the original
	siafile are both necessary to pin the file and keep the publink active. The
	pubfile will consume an additional 40 MiB of storage.`,
		Run: wrap(skynetconvertcmd),
	}

	skynetDownloadCmd = &cobra.Command{
		Use:   "download [publink] [destination]",
		Short: "Download a publink from Pubaccess.",
		Long: `Download a public access file by using a publink. The download may fail unless this
node is configured as a pubaccess portal. Use the --portal flag to fetch a publink
file from a chosen pubaccess portal.`,
		Run: skynetdownloadcmd,
	}

	skynetLsCmd = &cobra.Command{
		Use:   "ls",
		Short: "List all pubfiles that the user has pinned.",
		Long: `List all pubfiles that the user has pinned along with the corresponding
publinks. By default, only files in var/pubaccess/ will be displayed. The --root
flag can be used to view pubfiles pinned in other folders.`,
		Run: skynetlscmd,
	}

	skynetPinCmd = &cobra.Command{
		Use:   "pin [publink] [destination siapath]",
		Short: "Pin a publink from pubaccess by re-uploading it yourself.",
		Long: `Pin the file associated with this publink by re-uploading an exact copy. This
ensures that the file will still be available for public access as long as you continue
maintaining the file in your renter.`,
		Run: wrap(skynetpincmd),
	}

	skynetUnpinCmd = &cobra.Command{
		Use:   "unpin [siapath]",
		Short: "Unpin pinned pubfiles or directories.",
		Long: `Unpin one or more pinned pubfiles or directories at the given siapaths. The
files and directories will continue to be available on Pubaccess if other nodes have pinned them.`,
		Run: skynetunpincmd,
	}

	skynetUploadCmd = &cobra.Command{
		Use:   "upload [source path] [destination siapath]",
		Short: "Upload a file or a directory to Pubaccess.",
		Long: `Upload a file or a directory for Public access. A publink will be produced which can be
shared and used to retrieve the file. If the given path is a directory all files under that directory will
be uploaded individually and an individual publink will be produced for each. All files that get uploaded
will be pinned to this Sia node, meaning that this node will pay for storage and repairs until the files 
are manually deleted. Use the --dry-run flag to fetch the publink without actually uploading the file.`,
		Run: wrap(skynetuploadcmd),
	}
)

// skynetcmd displays the usage info for the command.
//
// TODO: Could put some stats or summaries or something here.
func skynetcmd(cmd *cobra.Command, args []string) {
	cmd.UsageFunc()(cmd)
	os.Exit(exitCodeUsage)
}

// skynetblacklistaddcmd adds publinks to the blacklist
func skynetblacklistaddcmd(cmd *cobra.Command, args []string) {
	skynetblacklistUpdate(args, nil)
}

// skynetblacklistremovecmd removes publinks from the blacklist
func skynetblacklistremovecmd(cmd *cobra.Command, args []string) {
	skynetblacklistUpdate(nil, args)
}

// skynetblacklistUpdate adds/removes trimmed publinks to the blacklist
func skynetblacklistUpdate(additions, removals []string) {
	additions = skynetblacklistTrimLinks(additions)
	removals = skynetblacklistTrimLinks(removals)

	err := httpClient.SkynetBlacklistPost(additions, removals)
	if err != nil {
		die("Unable to update pubaccess blacklist:", err)
	}

	fmt.Println("Pubaccess publink blacklist updated")
}

// skynetblacklistTrimLinks will trim away `scp://` from publinks
func skynetblacklistTrimLinks(links []string) []string {
	var result []string

	for _, link := range links {
		trimmed := strings.TrimPrefix(link, "scp://")
		result = append(result, trimmed)
	}

	return result
}

// skynetblacklistgetcmd will return the list of hashed merkleroots that are blocked
// from Pubaccess.
func skynetblacklistgetcmd(cmd *cobra.Command, args []string) {
	response, err := httpClient.SkynetBlacklistGet()
	if err != nil {
		die("Unable to get pubaccess blacklist:", err)
	}

	fmt.Printf("Listing %d blacklisted publink(s) merkleroots:\n", len(response.Blacklist))
	for _, hash := range response.Blacklist {
		fmt.Printf("\t%s\n", hash)
	}
}

// skynetconvertcmd will convert an existing siafile to a pubfile and publink on
// the Sia network.
func skynetconvertcmd(sourceSiaPathStr, destSiaPathStr string) {
	// Create the siapaths.
	sourceSiaPath, err := modules.NewSiaPath(sourceSiaPathStr)
	if err != nil {
		die("Could not parse source siapath:", err)
	}
	destSiaPath, err := modules.NewSiaPath(destSiaPathStr)
	if err != nil {
		die("Could not parse destination siapath:", err)
	}

	// Perform the conversion and print the result.
	sup := modules.SkyfileUploadParameters{
		SiaPath: destSiaPath,
	}
	publink, err := httpClient.SkynetConvertSiafileToSkyfilePost(sup, sourceSiaPath)
	if err != nil {
		die("could not convert siafile to pubfile:", err)
	}

	// Calculate the siapath that was used for the upload.
	var skypath modules.SiaPath
	if skynetUploadRoot {
		skypath = destSiaPath
	} else {
		skypath, err = modules.SkynetFolder.Join(destSiaPath.String())
		if err != nil {
			die("could not calculate path:", err)
		}
	}
	fmt.Printf("Pubfile uploaded successfully to %v\nSPublink: scp://%v\n", skypath, publink)
}

// skynetdownloadcmd will perform the download of a publink.
func skynetdownloadcmd(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		cmd.UsageFunc()(cmd)
		os.Exit(exitCodeUsage)
	}

	// Open the file.
	publink := args[0]
	publink = strings.TrimPrefix(publink, "scp://")
	filename := args[1]
	file, err := os.Create(filename)
	if err != nil {
		die("Unable to create destination file:", err)
	}

	// Check whether the portal flag is set, if so use the portal download
	// method.
	var reader io.ReadCloser
	if skynetDownloadPortal != "" {
		url := skynetDownloadPortal + "/" + publink
		resp, err := http.Get(url) //nolint:gosec
		if err != nil {
			die("Unable to download from portal:", errors.Compose(err, file.Close()))
		}
		reader = resp.Body
	} else {
		// Try to perform a download using the client package.
		reader, err = httpClient.SkynetPublinkReaderGet(publink)
		if err != nil {
			die("Unable to fetch publink:", errors.Compose(err, file.Close()))
		}
	}

	_, err = io.Copy(file, reader)
	err = errors.Compose(err, reader.Close(), file.Close())
	if err != nil {
		die("Unable to write full data:", err)
	}
}

// skynetlscmd is the handler for the command `spc pubaccess ls`. Works very
// similar to 'spc renter ls' but defaults to the PubaccessFolder and only
// displays files that are pinning publinks.
func skynetlscmd(cmd *cobra.Command, args []string) {
	var path string
	switch len(args) {
	case 0:
		path = "."
	case 1:
		path = args[0]
	default:
		cmd.UsageFunc()(cmd)
		os.Exit(exitCodeUsage)
	}
	// Parse the input siapath.
	var sp modules.SiaPath
	var err error
	if path == "." || path == "" || path == "/" {
		sp = modules.RootSiaPath()
	} else {
		sp, err = modules.NewSiaPath(path)
		if err != nil {
			die("could not parse siapath:", err)
		}
	}

	// Check whether the command is based in root or based in the pubaccess folder.
	if !skynetLsRoot {
		if sp.IsRoot() {
			sp = modules.SkynetFolder
		} else {
			sp, err = modules.SkynetFolder.Join(sp.String())
			if err != nil {
				die("could not build siapath:", err)
			}
		}
	}

	// Check if the command is hitting a single file.
	if !sp.IsRoot() {
		rf, err := httpClient.RenterFileRootGet(sp)
		if err == nil {
			if len(rf.File.Publinks) == 0 {
				fmt.Println("File is not pinning any publinks")
				return
			}
			json, err := json.MarshalIndent(rf.File, "", "  ")
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println()
			fmt.Println(string(json))
			fmt.Println()
			return
		} else if !strings.Contains(err.Error(), filesystem.ErrNotExist.Error()) {
			die(fmt.Sprintf("Error getting file %v: %v", path, err))
		}
	}

	// Get the full set of files and directories.
	//
	// NOTE: Always query recursively so that we can filter out non-tracked
	// files and get accurate, consistent sizes for dirs. If the --recursive
	// flag was not passed, we limit the directory output later.
	dirs := getDir(sp, true, true)

	// Sort the directories and the files.
	sort.Sort(byDirectoryInfo(dirs))
	for i := 0; i < len(dirs); i++ {
		sort.Sort(bySiaPathDir(dirs[i].subDirs))
		sort.Sort(bySiaPathFile(dirs[i].files))
	}

	// Keep track of the aggregate sizes for dirs as we may be adjusting them.
	sizePerDir := make(map[modules.SiaPath]uint64)
	for _, dir := range dirs {
		sizePerDir[dir.dir.SiaPath] = dir.dir.AggregateSize
	}

	// Drop any files that are not tracking publinks.
	var numDropped uint64
	numOmittedPerDir := make(map[modules.SiaPath]int)
	for j := 0; j < len(dirs); j++ {
		for i := 0; i < len(dirs[j].files); i++ {
			file := dirs[j].files[i]
			if len(file.Publinks) != 0 {
				continue
			}

			siaPath := dirs[j].dir.SiaPath
			numDropped++
			numOmittedPerDir[siaPath]++
			// Subtract the size from the aggregate size for the dir and all
			// parent dirs.
			for {
				sizePerDir[siaPath] -= file.Filesize
				if siaPath.IsRoot() {
					break
				}
				siaPath, err = siaPath.Dir()
				if err != nil {
					die("could not parse parent dir:", err)
				}
				if _, exists := sizePerDir[siaPath]; !exists {
					break
				}
			}
			// Remove the file.
			dirs[j].files = append(dirs[j].files[:i], dirs[j].files[i+1:]...)
			i--
		}
	}

	// Get the total number of listings (subdirs and files).
	root := dirs[0] // Root directory we are querying.
	var numFilesDirs uint64
	if skynetLsRecursive {
		numFilesDirs = root.dir.AggregateNumFiles + root.dir.AggregateNumSubDirs
		numFilesDirs -= numDropped
	} else {
		numFilesDirs = root.dir.NumFiles + root.dir.NumSubDirs
		numFilesDirs -= uint64(numOmittedPerDir[root.dir.SiaPath])
	}

	// Print totals.
	totalStoredStr := modules.FilesizeUnits(sizePerDir[root.dir.SiaPath])
	fmt.Printf("\nListing %v files/dirs:\t%9s\n\n", numFilesDirs, totalStoredStr)

	// Print dirs.
	for _, dir := range dirs {
		fmt.Printf("%v/", dir.dir.SiaPath)
		if numOmitted := numOmittedPerDir[dir.dir.SiaPath]; numOmitted > 0 {
			fmt.Printf("\t(%v omitted)", numOmitted)
		}
		fmt.Println()

		// Print subdirs.
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, subDir := range dir.subDirs {
			subDirName := subDir.SiaPath.Name() + "/"
			sizeUnits := modules.FilesizeUnits(sizePerDir[subDir.SiaPath])
			fmt.Fprintf(w, "  %v\t\t%9v\n", subDirName, sizeUnits)
		}

		// Print files.
		for _, file := range dir.files {
			name := file.SiaPath.Name()
			firstSkylink := file.Publinks[0]
			size := modules.FilesizeUnits(file.Filesize)
			fmt.Fprintf(w, "  %v\t%v\t%9v\n", name, firstSkylink, size)
			for _, publink := range file.Publinks[1:] {
				fmt.Fprintf(w, "\t%v\t\n", publink)
			}
		}
		w.Flush()
		fmt.Println()

		if !skynetLsRecursive {
			// If not --recursive, finish early after the first dir.
			break
		}
	}
}

// skynetpincmd will pin the file from this publink.
func skynetpincmd(sourceSkylink, destSiaPath string) {
	publink := strings.TrimPrefix(sourceSkylink, "scp://")
	// Create the siapath.
	siaPath, err := modules.NewSiaPath(destSiaPath)
	if err != nil {
		die("Could not parse destination siapath:", err)
	}

	spp := modules.SkyfilePinParameters{
		SiaPath: siaPath,
		Root:    skynetUploadRoot,
	}

	err = httpClient.SkynetPublinkPinPost(publink, spp)
	if err != nil {
		die("could not pin file to Pubaccess:", err)
	}

	fmt.Printf("Pubfile pinned successfully \nPublink: scp://%v\n", publink)
}

// skynetunpincmd will unpin and delete either a single or multiple files or
// directories from the Renter.
func skynetunpincmd(cmd *cobra.Command, skyPathStrs []string) {
	if len(skyPathStrs) == 0 {
		cmd.UsageFunc()(cmd)
		os.Exit(exitCodeUsage)
	}

	for _, skyPathStr := range skyPathStrs {
		// Create the pubaccesspath.
		skyPath, err := modules.NewSiaPath(skyPathStr)
		if err != nil {
			die("Could not parse pubacces path:", err)
		}

		// Parse out the intended siapath.
		if !skynetUnpinRoot {
			skyPath, err = modules.SkynetFolder.Join(skyPath.String())
			if err != nil {
				die("could not build siapath:", err)
			}
		}

		// Try to delete file.
		//
		// In the case where the path points to a dir, this will fail and we
		// silently move on to deleting it as a dir. This is more efficient than
		// querying the renter first to see if it is a file or a dir, as that is
		// guaranteed to always be two renter calls.
		errFile := httpClient.RenterFileDeleteRootPost(skyPath)
		if errFile == nil {
			fmt.Printf("Unpinned pubfile '%v'\n", skyPath)
			continue
		} else if !(strings.Contains(errFile.Error(), filesystem.ErrNotExist.Error()) || strings.Contains(errFile.Error(), filesystem.ErrDeleteFileIsDir.Error())) {
			die(fmt.Sprintf("Failed to unpin pubfile %v: %v", skyPath, errFile))
		}
		// Try to delete dir.
		errDir := httpClient.RenterDirDeleteRootPost(skyPath)
		if errDir == nil {
			fmt.Printf("Unpinned Pubaccess directory '%v'\n", skyPath)
			continue
		} else if !strings.Contains(errDir.Error(), filesystem.ErrNotExist.Error()) {
			die(fmt.Sprintf("Failed to unpin Pubaccess directory %v: %v", skyPath, errDir))
		}

		// Unknown file/dir.
		die(fmt.Sprintf("Unknown path '%v'", skyPath))
	}
}

// skynetuploadcmd will upload a file or directory to Pubaccess. If --dry-run is
// passed, it will fetch the publinks without uploading.
func skynetuploadcmd(sourcePath, destSiaPath string) {
	// Open the source file.
	file, err := os.Open(sourcePath)
	if err != nil {
		die("Unable to open source path:", err)
	}

	fi, err := file.Stat()
	errors.AddContext(err, "Unable to fetch source fileinfo")
	file.Close()
	// create a new progress bar set:
	pbs := mpb.New(mpb.WithWidth(40))
	if !fi.IsDir() {
		skynetUploadFile(sourcePath, sourcePath, destSiaPath, pbs)
		if skynetUploadDryRun {
			fmt.Print("[dry run] ")
		}
		pbs.Wait()
		fmt.Printf("Successfully uploaded file for public access!\n")
		return
	}

	if err != nil {
		die(err)
	}

	// Walk the target directory and collect all files that are going to be
	// uploaded.
	filesToUpload := make([]string, 0)
	err = filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println("Warning: skipping file:", err)
			return nil
		}
		if !info.IsDir() {
			filesToUpload = append(filesToUpload, path)
		}
		return nil
	})
	if err != nil {
		die(err)
	}
	// Confirm with the user that they want to upload all of them.
	if skynetUploadDryRun {
		fmt.Print("[dry run] ")
	}
	ok := askForConfirmation(fmt.Sprintf("Are you sure that you want to upload %d files to Pubaccess?", len(filesToUpload)))
	if !ok {
		os.Exit(0)
	}

	// Start the workers.
	filesChan := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < SimultaneousSkynetUploads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filename := range filesChan {
				// get only the filename and path, relative to the original destSiaPath
				// in order to figure out where to put the file
				newDestSiaPath := filepath.Join(destSiaPath, strings.TrimPrefix(filename, sourcePath))
				skynetUploadFile(sourcePath, filename, newDestSiaPath, pbs)
			}
		}()
	}
	// Send all files for upload.
	for _, path := range filesToUpload {
		filesChan <- path
	}
	// Signal the workers that there is no more work.
	close(filesChan)
	wg.Wait()
	pbs.Wait()
	if skynetUploadDryRun {
		fmt.Print("[dry run] ")
	}
	fmt.Printf("Successfully uploaded %d pubfiles!\n", len(filesToUpload))
}

// skynetUploadFile uploads a file to Pubaccess
func skynetUploadFile(basePath, sourcePath string, destSiaPath string, pbs *mpb.Progress) (publink string) {
	// Create the siapath.
	siaPath, err := modules.NewSiaPath(destSiaPath)
	if err != nil {
		die("Could not parse destination siapath:", err)
	}
	filename := filepath.Base(sourcePath)

	// Open the source file.
	file, err := os.Open(sourcePath)
	if err != nil {
		die("Unable to open source path:", err)
	}
	fi, err := file.Stat()
	if err != nil {
		die("Unable to fetch source fileinfo:", errors.Compose(err, file.Close()))
	}

	if skynetUploadSilent {
		// Silently upload the file and print a simple source -> publink
		// matching after it's done.
		publink = skynetUploadFileFromReader(file, filename, siaPath, fi.Mode())
		if err = file.Close(); err != nil {
			die("File error:", err)
		}
		fmt.Printf("%s -> %s\n", sourcePath, publink)
		return
	}

	// Display progress bars while uploading and processing the file.
	var relPath string
	if sourcePath == basePath {
		// when uploading a single file we only display the filename
		relPath = filename
	} else {
		// when uploading multiple files we strip the common basePath
		relPath, err = filepath.Rel(basePath, sourcePath)
		if err != nil {
			die("Could not get relative path:", err)
		}
	}
	rc := io.ReadCloser(file)
	// Wrap the file reader in a progress bar reader
	pUpload, rc := newProgressReader(pbs, fi.Size(), relPath, rc)
	// Set a spinner to start after the upload is finished
	pSpinner := newProgressSpinner(pbs, pUpload, relPath)
	// Perform the upload
	publink = skynetUploadFileFromReader(rc, filename, siaPath, fi.Mode())
	if err = rc.Close(); err != nil {
		die("File error:", err)
	}
	// Replace the spinner with the publink and stop it
	newProgressSkylink(pbs, pSpinner, relPath, publink)
	return
}

// skynetUploadFileFromReader is a helper method that uploads a file to Pubaccess
func skynetUploadFileFromReader(source io.Reader, filename string, siaPath modules.SiaPath, mode os.FileMode) (publink string) {
	// Upload the file and return a publink
	sup := modules.SkyfileUploadParameters{
		SiaPath: siaPath,
		Root:    skynetUploadRoot,

		DryRun: skynetUploadDryRun,

		FileMetadata: modules.SkyfileMetadata{
			Filename: filename,
			Mode:     mode,
		},

		Reader: source,
	}

	if skykeyName != "" && skykeyID != "" {
		die("Can only use either pubaccess keyname or keyid flag, not both.")
	}
	// Set Encrypt param to true if a pubaccesskey ID or name is set.
	if skykeyName != "" {
		sup.SkykeyName = skykeyName
	} else if skykeyID != "" {
		var ID pubaccesskey.PubaccesskeyID
		err := ID.FromString(skykeyID)
		if err != nil {
			die("Unable to parse pubaccesskey ID")
		}
		sup.PubaccesskeyID = ID
	}

	publink, _, err := httpClient.SkynetSkyfilePost(sup)
	if err != nil {
		die("could not upload file for Public access:", err)
	}
	return publink
}

// newProgressReader is a helper method for adding a new progress bar to an
// existing *mpb.Progress object.
func newProgressReader(pbs *mpb.Progress, size int64, filename string, file io.Reader) (*mpb.Bar, io.ReadCloser) {
	bar := pbs.AddBar(
		size,
		mpb.PrependDecorators(
			decor.Name(pBarJobUpload, decor.WC{W: 10}),
			decor.Percentage(decor.WC{W: 6}),
		),
		mpb.AppendDecorators(
			decor.Name(filename, decor.WC{W: len(filename) + 1, C: decor.DidentRight}),
		),
	)
	return bar, bar.ProxyReader(file)
}

// newProgressSpinner creates a spinner that is queued after `afterBar` is
// complete.
func newProgressSpinner(pbs *mpb.Progress, afterBar *mpb.Bar, filename string) *mpb.Bar {
	return pbs.AddSpinner(
		1,
		mpb.SpinnerOnMiddle,
		mpb.SpinnerStyle([]string{"∙∙∙", "●∙∙", "∙●∙", "∙∙●", "∙∙∙"}),
		mpb.BarQueueAfter(afterBar),
		mpb.BarFillerClearOnComplete(),
		mpb.PrependDecorators(
			decor.OnComplete(decor.Name(pBarJobProcess, decor.WC{W: 10}), pBarJobDone),
			decor.Name("", decor.WC{W: 6, C: decor.DidentRight}),
		),
		mpb.AppendDecorators(
			decor.Name(filename, decor.WC{W: len(filename) + 1, C: decor.DidentRight}),
		),
	)
}

// newProgressSkylink creates a static progress bar that starts after `afterBar`
// and displays the publink. The bar is stopped immediately.
func newProgressSkylink(pbs *mpb.Progress, afterBar *mpb.Bar, filename, publink string) *mpb.Bar {
	bar := pbs.AddBar(
		1, // we'll increment it once to stop it
		mpb.BarQueueAfter(afterBar),
		mpb.BarFillerClearOnComplete(),
		mpb.PrependDecorators(
			decor.Name(pBarJobDone, decor.WC{W: 10}),
			decor.Name(publink),
		),
		mpb.AppendDecorators(
			decor.Name(filename, decor.WC{W: len(filename) + 1, C: decor.DidentRight}),
		),
	)
	afterBar.Increment()
	bar.Increment()
	return bar
}
