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
 - Introduced Skynet with initial feature set for portals, web portals, skyfiles,
   skylinks, uploads, downloads, and pinning
 - Add `data-pieces` and `parity-pieces` flags to `siac renter upload`
 - Integrate SiaMux
 - Initialize defaults for the host's ephemeral account settings
 - Add SIA_DATA_DIR environment variable for setting the data directory for
   siad/siac
 - Made build process deterministic. Moved related scripts into `release-scripts`
 - Add directory support to Skylinks.
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
 - Add Skynet-Disable-Force header to allow disabling the force update feature
   on Skynet uploads

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
 - Sia node scanner
 - Gateway blacklisting
 - Contract Extortion Checker
 - Instant Boot
 - Alert System
 - Remove siafile chunks from memory
 - Additional price change protection for the Renter
 - siac Alerts command
 - Critical alerts displayed on every siac call
 - Single File Get in siac
 - Gateway bandwidth monitoring
 - Ability to pause uploads/repairs

**Bugs Fixed**
 - Missing return statements in API (http: superfluous response.WriteHeader call)
 - Stuck Loop fixes (chunks not being added due to directory siapath never being set)
 - Rapid Cycle repair loop on start up
 - Wallet Init with force flag when no wallet exists previous would error

**Other**
 - Module READMEs
 - staticcheck and gosec added
 - Security.md file created
 - Community images added for Built On Sia
 - JSON tag code analyzer 
 - ResponseWriter code analyzer
 - boltdb added to gitlab.com/NebulousLabs
