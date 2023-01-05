Initial setup
=============

Checkout [dev-wallet](https://gitlab.com/scpcorp/ScPrime/-/blob/dev-wallet) branch if it is not yet merged into master.

Build:
```shell
make dev
```

Launch (replace <spd-dir> with something like `/home/user/spd1`):
```shell
spd -M gctwhrm --scprime-directory <spd-dir> --host-addr=localhost:4282 --rpc-addr=localhost:4281 --api-addr=localhost:4280 --siamux-addr=localhost:4283 --siamux-addr-ws=localhost:4284 --no-bootstrap
```

To listen on 0.0.0.0 add `--disable-api-security` as well.

For this node seed and wallet password would be this:
```
hookup under total towel equip deity wrong nuisance lectures musical waking succeed pouch corrode damp butter dime hacksaw snake haunted fuzzy alchemy dagger tasked hemlock soccer needed hijack academy
```

Init wallet:
```shell
spc wallet init-seed
```

Unlock wallet. You need to do this every time spd restarts.
```shell
spc wallet unlock
```

After this you should have around 10MS in your wallet:
```shell
spc wallet
```

Start the miner:
```shell
spc miner start
```

Adding storage
==============

```shell
mkdir -p <storage-folder>
spc host folder add <storage-folder> 300mb
spc host config netaddress 127.0.0.1:4282
spc host announce
spc miner start
```

You should see your host as Active:
```shell
spc hostdb -v
```

Renting storage
===============

```shell
spc renter setallowance
spc renter upload <source-path> <dest-name>
spc renter download /<dest-name> <save-to-path>
```

Add more nodes
==============

Use unique dir and ports. Note that there is no `--no-bootstrap` argument as we need it to bootstrap.
```shell
spd -M gct --scprime-directory <spd-dir> --host-addr=localhost:5282 --rpc-addr=localhost:5281 --api-addr=localhost:5280 --siamux-addr=localhost:5283 --siamux-addr-ws=localhost:5284 --host-api-addr=localhost:5285
```

If you need more nodes just make directory for it and use another set of ports.

When node starts it tries to connect to `BootstrapPeers`. If you run all nodes locally and you have node at "127.0.0.1:4281", then you should be fine. But if you are building testnet over multiple hosts, then you should edit BootstrapPeers in modules/gateway.go and make nodes available at those addresses.
