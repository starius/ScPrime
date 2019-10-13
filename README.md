# [![SiaPrime Logo](https://siaprime.net/imagestore/SPRho_256x256.png)](http://siaprime.net) v1.4.1

[![Build Status](https://gitlab.com/SiaPrime/SiaPrime/badges/master/build.svg)](https://gitlab.com/SiaPrime/SiaPrime/commits/master)
[![Coverage Report](https://gitlab.com/SiaPrime/SiaPrime/badges/master/coverage.svg)](https://gitlab.com/SiaPrime/SiaPrime/commits/master)
[![GoDoc](https://godoc.org/gitlab.com/SiaPrime/SiaPrime?status.svg)](https://godoc.org/gitlab.com/SiaPrime/SiaPrime)
[![Go Report Card](https://goreportcard.com/badge/gitlab.com/SiaPrime/SiaPrime)](https://goreportcard.com/report/gitlab.com/SiaPrime/SiaPrime)
[![License MIT](https://img.shields.io/badge/License-MIT-brightgreen.svg)](https://img.shields.io/badge/License-MIT-brightgreen.svg)

SiaPrime is a decentralized cloud storage platform based on the Sia core 
protocol. Leveraging smart contracts, client-side encryption, and sophisticated
redundancy (via Reed-Solomon codes), SiaPrime allows users to safely store their 
data with hosts that they do not know or trust. The result is a cloud storage 
marketplace where hosts compete to offer the best service at the lowest price. 
Hosts can include everyone from individual hobbyists up to enterprise-scale 
datacenters with excess capacity to sell. 

The SiaPrime protocol currently provides very basic storage capability. We expect
the SiaPrime product suite will begin to roll out in 2020 as we work in parallel
with Sia core developers to deliver live products as soon as the protocol can 
support them. As time passes, we expect the SiaPrime protocol to diverge from 
the Sia protocol in some ways as we build an industrial strength version.

Traditional cloud storage is dominated by a small number of companies.
SiaPrime is an initiative to provide small and medium sized business (SMB) a 
comprehensive cloud storage environment uncompromised by inside 
or outside interference. This is achieved through fragmenting user data and 
spreading it across a distributed network of hosts. No single datacenter has 
access to your complete data.

These fragments are duplicated over enough hosts that even if several hosts 
disappear, your data is still accessible. Switching to different hosts for 
better geographic coverage, price, speed or reliability is easy as hosts 
compete with each other to provide the best service offering. 

SiaPrime will offer a complete cloud storage solution for file backup, long term 
archival needs, CDN use cases as well as bridging the gap between online and 
offline strategies. The SiaPrime network allows for highly parallel data transfer, 
which when coupled with geographic targeting can significantly reduce latency
and access to your data no matter if you are storing family photos or large 
database backups.

The process of choosing hosts on the SiaPrime network allows customers to 
choose based on latency, lowest price, widest geographic coverage, or even a 
strict whitelist of IP addresses or public keys. We expect to provide customers 
multiple ways to purchase storage on the SiaPrime network, including simple 
Dropbox-like web interfaces up to sophisticated client dashboards for 
enterprise clients.

Purchasing storage on the network will have several variants, though all 
ultimately use our core cryptocurrency utility coin called a SiaPrime Coin. 
SiaPrime Coins will be available on cryptocurrency exchanges. We will also be 
creating fiat to storage gateways to encourage broad adoption of the SiaPrime 
storage model. 


Usage
-----
This release comes with 2 binaries, `spd` and `spc`. `spd` is a background
service, or "daemon," that runs the SiaPrime protocol and exposes an HTTP API on
port 4280. `spc` is a command-line client that can be used to interact with
`spd` in a user-friendly way. There is also a graphical client, [SiaPrime-UI](https://gitlab.com/SiaPrime/SiaPrime-UI), 
which is the preferred way of using SiaPrime for most users. For interested 
developers, the `spd` API is documented [here](doc/API.md).

On Windows, double-click `spd.exe`. For command line operation, navigate to the
containing folder and click File->Open command prompt. Start the `spd` service 
by entering `spd` and pressing Enter. The command prompt may appear to freeze; 
this means `spd` is waiting for requests. Windows users may see a warning from 
Windows Firewall; be sure to check both boxes ("Private networks" and "Public 
networks") and click "Allow access." You can now run `spc` (in a separate command
prompt) or SiaPrime-UI to interact with `spd`. 

Building From Source
--------------------

To build from source, [Go 1.13 or above must be installed](https://golang.org/doc/install)
on the system. Clone the repo and run `make`:

```
git clone https://gitlab.com/SiaPrime/SiaPrime
cd SiaPrime && make
```

This will install the`spd` and `spc` binaries in your `$GOPATH/bin` folder.
(By default, this is `$HOME/go/bin`.)

You can also run `make test` and `make test-long` to run the short and full test
suites, respectively. Finally, `make cover` will generate code coverage reports
for each package; they are stored in the `cover` folder and can be viewed in
your browser.
