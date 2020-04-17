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
### v1.4.3.0

**Key Updates**
- Alerts returned by /daemon/alerts route are sorted by severity
- Add `--fee-included` parameter to `siac wallet send siacoins` that allows
   sending an exact wallet balance with the fees included.
- Extend `siac hostdb view` to include all the fields returned from the API.
- `siac renter delete` now accepts a list of files.
- add pause and resume uploads to siac
- Extended `siac renter` to include number of passive and disabled contracts
- Add contract data to `siac renter`
- Add getters and setter to `FileContract` and `FileContractRevision` types to prevent index-out-of-bounds panics after a `RenewAndClear`.
- Add `--dry-run` parameter to Pubaccess upload
- Set ratio for `MinBaseRPCPrice` and `MinSectorAccessPrice` with   `MinDownloadBandwidthPrice`

**Bugs Fixed**
- Fixed file health output of `siac renter -v` not adding to 100% by adding
  parsePercentage function.
- Fix `unlock of unlocked mutex` panic in the download destination writer.
- Fix potential channel double closed panic in DownloadByRootProject 
- Fix divide by zero panic in `renterFileHealthSummary` for `siac renter -v`
- Fix negative currency panic in `siac renter contracts view`
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
 - Add `data-pieces` and `parity-pieces` flags to `siac renter upload`
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
- Add `--root` parameter to `siac renter delete` that allows passing absolute
  instead of relative file paths.
- Add ability to blacklist publinks by merkleroot.
- Uploading resumes more quickly after restart.
- Add `HEAD` request for publink
- Add ability to pack many files into the same or adjacent sectors while
  producing unique publinks for each file.
- Fix default expected upload/download values displaying 0 when setting an
  initial allowance.
- `siac pubaccess upload` now supports uploading directories. All files are
  uploaded individually and result in separate publinks.
- No user-agent needed for Publink downloads.
- Add `go get` command to `make dependencies`.
- Add flags for tag and targz for pubfile streaming.
- Add new endpoint `/pubaccess/stats` that provides statistical information about
  pubaccess, how many files were uploaded and the combined size of said files.
- The `siac renter setallowance` UX is considerably improved.
- Add XChaCha20 CipherKey.
- Add Pubaccesskey Manager.
- Add `siac pubaccess unpin` subcommand.
- Extend `siac renter -v` to show breakdown of file health.
- Add Pubaccess-Disable-Force header to allow disabling the force update feature
  on Pubaccess uploads
- Add bandwidth usage to `siac gateway`

**Bugs Fixed**
- Fixed bug in startup where an error being returned by the renter's blocking
  startup process was being missed
- Fix repair bug where unused hosts were not being properly updated for a
  siafile
- Fix threadgroup violation in the watchdog that allowed writing to the log
  file after a shutdown
- Fix bug where `siac renter -v` wasn't working due to the wrong flag being
  used.
- Fixed bug in siafile snapshot code where the `hostKey()` method was not used
  to safely acquire the host pubkey.
- Fixed `siac pubaccess ls` not working when files were passed as input. It is now
  able to access specific files in the Pubaccess folder.
- Fixed a deadlock when performing a Pubaccess download with no workers
- Fix a parsing bug for malformed publinks
- fix siac update for new release verification
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
- Add `data-pieces` and `parity-pieces` flags to `siac renter upload`
- Integrate SiaMux
- Initialize defaults for the host's ephemeral account settings
- Add SIA_DATA_DIR environment variable for setting the data directory for
  siad/siac
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
 - siac Alerts command
 - Critical alerts displayed on every siac call
 - Single File Get in siac
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