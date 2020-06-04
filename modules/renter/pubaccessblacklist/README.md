# Pubaccess Blacklist

The Pubaccess Blacklist module manages a list of blacklisted Pubaccess links by tracking
their merkleroots.

## Subsystems
The following subsystems help the Pubaccess Blacklist module execute its
responsibilities:
 - [Pubaccess Blacklist Subsystem](#pubaccess-blacklist-subsystem)

### Pubaccess Blacklist Subsystem
**Key Files**
 - [skynetblacklist.go](./skynetblacklist.go)

The Pubaccess Blacklist subsystem contains the structure of the Pubaccess Blacklist
and is used to create a new Pubaccess Blacklist and return information about the
Blacklist. Uses Persist package's Append-Only File subsystem to ensure ACID disk
updates.

**Exports**
 - `Blacklist` returns the list of blacklisted merkle roots
 - `IsBlacklisted` returns whether or not a publink merkleroot is blacklisted
 - `New` creates and returns a new Pubaccess Blacklist
 - `UpdateBlacklist` updates the blacklist
