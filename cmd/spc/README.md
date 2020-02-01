spc usage
==========

`spc` is the command line interface to ScPrime, for use by power users
and those on headless servers. It comes as a part of the command line
package, and can be run as `./spc` from the same folder, or just by
calling `spc` if you move the binary into your path.

Most of the following commands have online help. For example, executing
`spc wallet send help` will list the arguments for that command,
while `spc host help` will list the commands that can be called
pertaining to hosting. `spc help` will list all of the top level
command groups that can be used.

You can change the address of where siad is pointing using the `-a`
flag. For example, `spc -a :9000 status` will display the status of
the siad instance launched on the local machine with `siad -a :9000`.

Common tasks
------------
* `spc consensus` view block height

Wallet:
* `spc wallet init [-p]` initilize a wallet
* `spc wallet unlock` unlock a wallet
* `spc wallet balance` retrieve wallet balance
* `spc wallet address` get a wallet address
* `spc wallet send [amount] [dest]` sends scprimecoins to an address

Renter:
* `spc renter list` list all renter files
* `spc renter upload [filepath] [nickname]` upload a file
* `spc renter download [nickname] [filepath]` download a file


Full Descriptions
-----------------

#### Wallet tasks

* `spc wallet init [-p]` encrypts and initializes the wallet. If the
`-p` flag is provided, an encryption password is requested from the
user. Otherwise the initial seed is used as the encryption
password. The wallet must be initialized and unlocked before any
actions can be performed on the wallet.

Examples:
```bash
user@hostname:~$ spc -a :4220 wallet init
Seed is:
 cider sailor incur sober feast unhappy mundane sadness hinder aglow imitate amaze duties arrow gigantic uttered inflamed girth myriad jittery hexagon nail lush reef sushi pastry southern inkling acquire

Wallet encrypted with password: cider sailor incur sober feast unhappy mundane sadness hinder aglow imitate amaze duties arrow gigantic uttered inflamed girth myriad jittery hexagon nail lush reef sushi pastry southern inkling acquire
```

```bash
user@hostname:~$ spc -a :4220 wallet init -p
Wallet password:
Seed is:
 potato haunted fuming lordship library vane fever powder zippers fabrics dexterity hoisting emails pebbles each vampire rockets irony summon sailor lemon vipers foxes oneself glide cylinder vehicle mews acoustic

Wallet encrypted with given password
```

* `spc wallet unlock` prompts the user for the encryption password
to the wallet, supplied by the `init` command. The wallet must be
initialized and unlocked before any actions can take place.

* `spc wallet balance` prints information about your wallet.

Example:
```bash
user@hostname:~$ spc wallet balance
Wallet status:
Encrypted, Unlocked
Confirmed Balance:   61516458.00 SPC
Unconfirmed Balance: 64516461.00 SPC
Exact:               61516457999999999999999999999999 H
```

* `spc wallet address` returns a never seen before address for sending
scprimecoins to.

* `spc wallet send [amount] [dest]` Sends `amount` scprimecoins to
`dest`. `amount` is in the form XXXXUU where an X is a number and U is
a unit, for example MS, S, mS, ps, etc. If no unit is given hastings
is assumed. `dest` must be a valid scprimecoin address.

* `spc wallet lock` locks a wallet. After calling, the wallet must be unlocked
using the encryption password in order to use it further

* `spc wallet seeds` returns the list of secret seeds in use by the
wallet. These can be used to regenerate the wallet

* `spc wallet addseed` prompts the user for his encryption password,
as well as a new secret seed. The wallet will then incorporate this
seed into itself. This can be used for wallet recovery and merging.

#### Host tasks
* `host config [setting] [value]`

is used to configure hosting.

In version `1.4.1`, scprime hosting is configured as follows:

| Setting                  | Value                                           |
| -------------------------|-------------------------------------------------|
| acceptingcontracts       | Yes or No                                       |
| maxduration              | in weeks, at least 12                           |
| collateral               | in SPC / TB / Month, 10-1000                    |
| collateralbudget         | in SPC                                          |
| maxcollateral            | in SPC, max per contract                        |
| mincontractprice         | minimum price in SC per contract                |
| mindownloadbandwidthprice| in SPC / TB                                     |
| minstorageprice          | in SPC / TB                                     |
| minuploadbandwidthprice  | in SPC / TB                                     |

You can call this many times to configure you host before
announcing. Alternatively, you can manually adjust these parameters
inside the `host/config.json` file.

* `spc host announce` makes an host announcement. You may optionally
supply a specific address to be announced; this allows you to announce a domain
name. Announcing a second time after changing settings is not necessary, as the
announcement only contains enough information to reach your host.

* `spc host -v` outputs some of your hosting settings.

Example:
```bash
user@hostname:~$ spc host -v
Host settings:
Storage:      2.0000 TB (1.524 GB used)
Price:        0.000 SPC per GB per month
Collateral:   0
Max Filesize: 10000000000
Max Duration: 8640
Contracts:    32
```

* `spc hostdb -v` prints a list of all the know active hosts on the
network.

#### Renter tasks
* `spc renter upload [filename] [nickname]` uploads a file to the sia
network. `filename` is the path to the file you want to upload, and
nickname is what you will use to refer to that file in the
network. For example, it is common to have the nickname be the same as
the filename.

* `spc renter list` displays a list of the your uploaded files
currently on the sia network by nickname, and their filesizes.

* `spc renter download [nickname] [destination]` downloads a file
from the scprime network onto your computer. `nickname` is the name
used to refer to your file in the scprime network, and `destination` is 
the path to where the file will be. If a file already exists there, it
will be overwritten.

* `spc renter rename [nickname] [newname]` changes the nickname of a
  file.

* `spc renter delete [nickname]` removes a file from your list of
stored files. This does not remove it from the network, but only from
your saved list.

* `spc renter queue` shows the download queue. This is only relevant
if you have multiple downloads happening simultaneously.

#### Gateway tasks
* `spc gateway` prints info about the gateway, including its address and how
many peers it's connected to.

* `spc gateway list` prints a list of all currently connected peers.

* `spc gateway connect [address:port]` manually connects to a peer and adds it
to the gateway's node list.

* `spc gateway disconnect [address:port]` manually disconnects from a peer, but
leaves it in the gateway's node list.

#### Miner tasks
* `spc miner status` returns information about the miner. It is only
valid for when siad is running.

* `spc miner start` starts running the CPU miner on one thread. This
is virtually useless outside of debugging.

* `spc miner stop` halts the CPU miner.

#### General commands
* `spc consensus` prints the current block ID, current block height, and
current target.

* `spc stop` sends the stop signal to ScPrime daemon to safely terminate.
This has the same affect as C^c on the terminal.

* `spc version` displays the version string of spc and daemon.

* `spc update` checks the server for updates.
