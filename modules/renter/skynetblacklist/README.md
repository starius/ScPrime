# Pubaccess Blacklist

The Pubaccess Blacklist modules manages a list of blacklisted Publinks by tracking
their merkleroots.

## Subsystems
The following subsystems help the Pubaccess Blacklist module execute its responsibilities:
 - [Persistence Subsystem](#persistence-subsystem)
 - [Pubaccess Blacklist Subsystem](#pubaccess-blacklist-subsystem)

 ### Persistence Subsystem
 **Key Files**
- [persist.go](./persist.go)

The Persistence subsystem is responsible for the disk interaction and ensuring
safe and performant ACID operations. An append only structure is used with a
length of fsync'd bytes encoded in the metadata.

**Inbound Complexities**
 - `callInitPersist` initializes the persistence file 
    - The Pubaccess Blacklist Subsystem's `New` method uses `callInitPersist`
 - `callUpdateAndAppend` updates the pubaccess blacklist and appends the
   information to the persistence file
    - The Pubaccess Blacklist Subsytem's `Update` method uses `callUpdateAndAppend`

### Pubaccess Blacklist Subsystem
**Key Files**
 - [skynetblacklist.go](./skynetblacklist.go)

The Pubaccess Blacklist subsystem contains the structure of the Pubaccess Blacklist
and is used to create a new Pubaccess Blacklist and return information about the
Blacklist.

**Exports**
 - `Blacklist` returns the list of blacklisted merkle roots
 - `IsBlacklisted` returns whether or not a publink merkleroot is blacklisted
 - `New` creates and returns a new Pubaccess Blacklist
 - `Update` updates the blacklist

**Outbound Complexities**
 - `New` calls the Persistence Subsystem's `callInitPersist` method
 - `Update` calls the Persistence Subsystem's `callUpdateAndAppend` method