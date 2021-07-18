Version Scheme
--------------
ScPrime uses the following versioning scheme, vX.X.X.X
 - First Digit signifies a major (compatibility breaking) release
 - Second Digit signifies a major (non compatibility breaking) release
 - Third Digit signifies a minor release
 - Fourth Digit signifies a patch release

Version History
---------------

Latest:
### v1.5.4
- Remove NebulousLabs feemanager module


### v1.5.0
**Key Updates**
- Add `zip` download format and set it as default format.
- add support for write MDM programs to host
- Added `defaultpath` - a new optional path parameter when creating Publinks. It
  determines which is the default file to open in a multi-file pubfile.
- Add `configModules` to the API so that the spd modules can be return in `/daemon/settings [GET]`
- Allow the renew window to be larger than the period
- Convert pubaccessblacklist from merkleroots to hashes of the merkleroots
- split up the custom http status code returned by the API for unloaded modules into 2 distinct codes.
- Add `daemon/stack` endpoint to get the current stack trace.
- Add Pubaccesskey delete methods to API.
- Add `disabledefaultpath` - a new optional path parameter when creating 
Publinks. It disables the default path functionality, guaranteeing that the user
will not be automatically redirected to `/index.html` if it exists in the 
pubfile.
- Add 'spc' commands for the FeeManager
- Add `TypePrivateID` Pubaccesskeys with pubfile encryption support
- Added available and priority memory output to `spc renter -v`

**Bugs Fixed**
- Set 'Content-Disposition' header for archives.
- fixed bug in rotation of fingerprint buckets
- fix issue where priority tasks could wait for low priority tasks to complete
- Fix panic in backup code due to not using `newJobGeneric`
- Public access filenames are now validated when uploading. Previously you could upload files called e.g. "../foo" which would be inaccessible.
- The Pubaccesskey encryption API docs were updated to fix some discrepancies. In particular, the pubaccesskeyid section was removed.
- The createpubaccesskey endpoint was fixed as it was not returning the full Pubaccesskey that was created.
- integrade download cooldown system into download jobs
- fix bug which could prevent downloads from making progress
- Fix panic in the wal of the spdir and siafile if a delete update was
  submitted as the last update in a set of updates.

**Other**
- Add `EphemeralAccountExpiry` and `MaxEphemeralAccountBalance` to the Host's ExternalSettings
- Add testing infrastructure to validate the output of spc commands.
- Add root spc Cobra command test with subtests.
- Optimise writes when we execute an MDM program on the host to lower overall
  (upload) bandwidth consumption.
- Change status returned when module is not loaded from 404 to 490
- Add `spc renter workers ea` command to spc
- Add `spc renter workers pt` command to spc
- Add `spc renter workers rj` command to spc
- Add `spc renter workers hsj` command to spc
- Add testing for blacklisting skylinks associated with siafile conversions
- Rename `Gateway` `blacklist` to `blocklist`
- Allow host netAddress and announcements with local network IP on dev builds.
- Add default timeouts to opening a stream on the mux
- Update to bolt version with upstream fixes. This enables builds with Go 1.16.

### v1.4.4.0
**Key Updates**
- Merged Sia 1.4.11
**Other**

## May 29, 2020:
### v1.4.3.1
**Key Updates**
- Add `FeeManager` to spd to allow for applications to charge a fee
- Add start time for the API server for spd uptime
- Add new `/consensus/subscribe/:id` endpoint to allow subscribing to consensus
  change events
- Add /pubaccesskeys endpoint and `spc pubaccesskey ls` command
- Updated pubaccesskey encoding and format

**Bugs Fixed**
- fix call to expensive operation in tight loop
- fix an infinite loop which would block uploads from progressing

**Other**
- Optimize bandwidth consumption for RPC write calls
- Extend `/daemon/alerts` with `criticalalerts`, `erroralerts` and
  `warningalerts` fields along with `alerts`.
- Update pubaccesskey spc functions to accept httpClient and remove global httpClient
  reference from spc testing
- Pubaccesskeycmd test broken down to subtests.
- Create spc testing helpers.
- Add engineering guidelines to /doc
- Introduce PaymentProvider interface on the renter.
- Pubaccess persistence subsystems into shared system.
- Update Cobra from v0.0.5 to v1.0.0.

### v1.4.3.0
**Key Updates**
- Alerts returned by /daemon/alerts route are sorted by severity
- Add `--fee-included` parameter to `spc wallet send scprimecoins` that allows
   sending an exact wallet balance with the fees included.
- Extend `spc hostdb view` to include all the fields returned from the API.
- `spc renter delete` now accepts a list of files.
- add pause and resume uploads to spc
- Extended `spc renter` to include number of passive and disabled contracts
- Add contract data to `spc renter`
- Add getters and setter to `FileContract` and `FileContractRevision` types to prevent index-out-of-bounds panics after a `RenewAndClear`.
- Add `--dry-run` parameter to Pubaccess upload
- Set ratio for `MinBaseRPCPrice` and `MinSectorAccessPrice` with   `MinDownloadBandwidthPrice`

**Bugs Fixed**
- Fixed file health output of `spc renter -v` not adding to 100% by adding
  parsePercentage function.
- Fix `unlock of unlocked mutex` panic in the download destination writer.
- Fix potential channel double closed panic in DownloadByRootProject 
- Fix divide by zero panic in `renterFileHealthSummary` for `spc renter -v`
- Fix negative currency panic in `spc renter contracts view`
- Fix panic when metadata of pubfile upload exceeds modules.SectorSize
- Fix curl example for `/pubaccess/pubfile/` post
- Don't delete hosts the renter has a contract with from hostdb 
- Initiate a hostdb rescan on startup if a host the renter has a contract with isn't in the host tree 
- Increase max host downtime in hostbd from 10 days to 20 days.
- Remove `build.Critical` and update to a metadata update

**Other**
- Add timeout parameter to Publink pin route - Also apply timeout when fetching the individual chunks
- Add SiaMux stream handler to the host
- Fix TestAccountExpiry NDF
- Add benchmark test for bubble metadata
- Add additional format instructions to the API docs and fix format errors
- Created Minor Merge Request template.
- Updated `Resources.md` with links to filled out README files
- Add version information to the stats endpoint
- Extract environment variables to constants and add to API docs.
 - Add PaymentProcessor interface (host-side)
- Move golangci-lint to `make lint` and remove `make lint-all`.
- Add whitespace lint to catch extraneous whitespace and newlines.
- Expand `SiaPath` unit testing to address more edge cases.

**Key Updates**
 - Introduced Pubaccess with initial feature set for portals, web portals, pubfiles,
   publinks, uploads, downloads, and pinning
 - Add `data-pieces` and `parity-pieces` flags to `spc renter upload`
 - Integrate SiaMux
 - Initialize defaults for the host's ephemeral account settings
 - Add SCPRIME_DATA_DIR environment variable for setting the data directory for
   spd/spc
 - Made build process deterministic. Moved related scripts into `release-scripts`
 - Add directory support to Publinks.
 - Enabled Lockcheck code anaylzer
 - Added Bandwidth monitoring to the host module

- Add a delay when modifying large contracts on hosts to prevent hosts from
  becoming unresponsive due to massive disk i/o.
- Add `--root` parameter to `spc renter delete` that allows passing absolute
  instead of relative file paths.
- Add ability to blacklist publinks by merkleroot.
- Uploading resumes more quickly after restart.
- Add `HEAD` request for publink
- Add ability to pack many files into the same or adjacent sectors while
  producing unique publinks for each file.
- Fix default expected upload/download values displaying 0 when setting an
  initial allowance.
- `spc pubaccess upload` now supports uploading directories. All files are
  uploaded individually and result in separate publinks.
- No user-agent needed for Publink downloads.
- Add `go get` command to `make dependencies`.
- Add flags for tag and targz for pubfile streaming.
- Add new endpoint `/pubaccess/stats` that provides statistical information about
  pubaccess, how many files were uploaded and the combined size of said files.
- The `spc renter setallowance` UX is considerably improved.
- Add XChaCha20 CipherKey.
- Add Pubaccesskey Manager.
- Add `spc pubaccess unpin` subcommand.
- Extend `spc renter -v` to show breakdown of file health.
- Add Pubaccess-Disable-Force header to allow disabling the force update feature
  on Pubaccess uploads
- Add bandwidth usage to `spc gateway`

**Bugs Fixed**
- Fixed bug in startup where an error being returned by the renter's blocking
  startup process was being missed
- Fix repair bug where unused hosts were not being properly updated for a
  siafile
- Fix threadgroup violation in the watchdog that allowed writing to the log
  file after a shutdown
- Fix bug where `spc renter -v` wasn't working due to the wrong flag being
  used.
- Fixed bug in siafile snapshot code where the `hostKey()` method was not used
  to safely acquire the host pubkey.
- Fixed `spc pubaccess ls` not working when files were passed as input. It is now
  able to access specific files in the Pubaccess folder.
- Fixed a deadlock when performing a Pubaccess download with no workers
- Fix a parsing bug for malformed publinks
- fix spc update for new release verification
- Fix parameter delimiter for publinks
- Fixed race condition in host's `RPCLoopLock`
- Fixed a bug which caused a call to `build.Critical` in the case that a
  contract in the renew set was marked `!GoodForRenew` while the contractor
  lock was not held

**Other**
- Split out renter siatests into 2 groups for faster pipelines.
- Add README to the `siatest` package 
- Bump golangci-lint version to v1.23.8
- Add timeout parameter to Publink route - Add `go get` command to `make
  dependencies`.
- Update repair loop to use `uniqueRefreshPaths` to reduce unnecessary bubble
  calls
- Add Pubaccess-Disable-Force header to allow disabling the force update feature
  on Pubaccess uploads
- Create generator for Changelog to improve changelog update process

## Feb 2020:
### v1.4.3
**Key Updates**
- Introduced Pubaccess with initial feature set for portals, web portals, pubfiles,
  publinks, uploads, downloads, and pinning
- Add `data-pieces` and `parity-pieces` flags to `spc renter upload`
- Integrate SiaMux
- Initialize defaults for the host's ephemeral account settings
- Add SIA_DATA_DIR environment variable for setting the data directory for
  spd/spc
- Made build process deterministic. Moved related scripts into `release-scripts`
- Add directory support to Publinks.
- Enabled Lockcheck code anaylzer
- Added Bandwidth monitoring to the host module
 
**Bugs Fixed**
- HostDB Data race fixed and documentation updated to explain the data race
  concern
- `Name` and `Dir` methods of the Siapath used the `filepath` package when they
  should have used the `strings` package to avoid OS path separator bugs
- Fixed panic where the Host's contractmanager `AddSectorBatch` allowed for
  writing to a file after the contractmanager had shutdown
- Fixed panic where the watchdog would try to write to the contractor's log
  after the contractor had shutdown

**Other**
- Upgrade host metadata to v1.4.3
- Removed stubs from testing


### v1.4.2.0
**Key Updates**

 - Allowance in Backups
 - Wallet Password Reset
 - Bad Contract Utility Add
 - FUSE
 - Renter Watchdog
 - Contract Churn Limiter
 - Serving Downloads from Disk
 - Verify Wallet Password Endpoint
 - Siafilesystem
 - ScPrime node scanner
 - Gateway blacklisting
 - Contract Extortion Checker
 - "Instant" Boot
 - Alert System
 - Remove siafile chunks from memory
 - Additional price change protection for the Renter
 - spc Alerts command
 - Critical alerts displayed on every spc call
 - Single File Get in spc
 - Gateway bandwidth monitoring
 - Ability to pause uploads/repairs

**Bugs Fixed**
 - Repair operations would sometimes perform useless and redundant repairs
 - Siafiles were not pruning hosts correctly
 - Unable to upload a new file if 'force' is set and no file exists to delete
 - Siac would not always delete a file or folder correctly
 - Divide by zero error when setting the allowance with an empty period
 - Host would sometimes deadlock upon shutdown due to thread group misuse
 - Crash preventing host from starting up correctly after an unclean shutdown
   while resizing a storage folder
 - Missing return statements in API (http: superfluous response.WriteHeader call)
 - Stuck Loop fixes (chunks not being added due to directory siapath never being set)
 - Rapid Cycle repair loop on start up
 - Wallet Init with force flag when no wallet exists previous would error
...