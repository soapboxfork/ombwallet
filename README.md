ombwallet
=========

A fork of btcsuite's [btcwallet](https://github.com/btcsuite/btcwallet). 
This project is based off of the 0.5.1 Alpha release of btcwallet.
It has intentionally been held back to maintain my sanity as we work on other code bases.

## Installation

### Linux/BSD/POSIX - Build from Source

- Install Go according to the installation instructions here:
  http://golang.org/doc/install

- Run the following commands to obtain and install btcwallet and all
  dependencies:
```bash
$ go get -u -v github.com/soapboxsys/ombfullnode/...
$ go get -u -v github.com/soapboxsys/ombwallet/...
```

- ombfullnode and ombwallet will now be installed in either ```$GOROOT/bin``` or
  ```$GOPATH/bin``` depending on your configuration.  If you did not already
  add to your system path during the installation, we recommend you do so now.
