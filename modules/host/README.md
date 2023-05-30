# Host
The host takes local disk storage and makes it available to the ScPrime network. It
will do so by announcing itself, and its settings, to the network. Renters
transact with the host through a number of RPC calls.

In order for data to be uploaded and stored, the renter and host must agree on a
file contract. Once they have negotiated the terms of the file contract, it is
signed and put on chain. Any further action related to the data is reflected in
the file contract by means of contract revisions that get signed by both
parties. The host is responsible for managing these contracts, and making sure
the data is safely stored. The host proves that it is storing the data by
providing a segment of the file and a list of hashes from the file's merkletree.

Aside from storage, the host offers another service called ephemeral accounts.
These accounts serve as an alternative payment method to file contracts. Users
can deposit money into them and then later use those funds to transact with the
host. The most common transactions are uploading and downloading data, however
any RPC that requires payment will support receiving payment from an ephemeral
account.

## Submodules

 - [ContractManager](./contractmanager/README.md)

### ContractManager

The ContractManager is responsible for managing contracts that the host has with
renters, including storing the data, submitting storage proofs, and deleting the
data when a contract is complete.
