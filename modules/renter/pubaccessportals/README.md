# Public Access Portals

The Public Access Portals module manages a list of known Public Access portals 
and whether they are public or not.

## Subsystems
The following subsystems help the Public Access Portals module execute its
responsibilities:
 - [Persistence Subsystem](#persistence-subsystem)
 - [Public Access Portals Subsystem](#bubaccess-portals-subsystem)

 ### Persistence Subsystem
 **Key Files**
- [persist.go](./persist.go)

The Persistence subsystem is responsible for the disk interaction and ensuring
safe and performant ACID operations. An append only structure is used with a
length of fsync'd bytes encoded in the metadata.

**Inbound Complexities**
 - `callInitPersist` initializes the persistence file
    - The Pubaccess Portals Subsystem's `New` method uses `callInitPersist`
 - `callUpdateAndAppend` updates the pubaccess portal list and appends the
   information to the persistence file
    - The Pubaccess Portals Subsytem's `Update` method uses `callUpdateAndAppend`

### Pubaccess Portals Subsystem
**Key Files**
 - [pubaccessportals.go](./pubaccessportals.go)

The Pubaccess Portals subsystem contains the structure of the Pubaccess Portals List
and is used to create a new Pubaccess Portals List and return information about the
Portals.

**Exports**
 - `Portals` returns the list of known Pubaccess portals and whether they are
   public
 - `New` creates and returns a new Pubaccess Portals List
 - `Update` updates the Portals List

**Outbound Complexities**
 - `New` calls the Persistence Subsystem's `callInitPersist` method
 - `Update` calls the Persistence Subsystem's `callUpdateAndAppend` method
