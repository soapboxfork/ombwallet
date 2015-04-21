ombwallet
=========

A fork of btcsuite's [btcwallet](https://github.com/btcsuite/btcwallet). 
This project is based off of the 0.5.1 Alpha release of btcwallet.
It has intentionally been held back to maintain my sanity as we work on other code bases.

## Installation

### Linux/BSD/POSIX - Build from Source

- Install Go according to the installation instructions here:
  http://golang.org/doc/install

- Verify that you have the [godep tool](https://github.com/tools/godep). 

- Install ombfullnode by folllowing the instructions [here](https://github.com/soapboxsys/ombfullnode/blob/master/README.md#installation).

- Run the following commands:
```bash
# Just download the required packages.
> go get -d github.com/soapboxsys/ombwallet/...
# Move into the workspace's path
> cd $GOPATH/src/github.com/soapboxsys/ombwallet
# Use godep to checkout the correct dependent library commits
> godep restore
# Move into your $GOPATH binary directory and build the binary
> cd $GOPATH/bin/
> go build github.com/soapboxsys/ombwallet/...
```

