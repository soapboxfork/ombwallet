package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcwallet/legacy/keystore"
	"github.com/soapboxsys/ombwallet/chain"
)

func waitForSetup(cfg *config) error {

	// Ensure the wallet exists and create it if it does not exist
	netDir := networkDir(cfg.DataDir, activeNet.Params)
	dbPath := filepath.Join(netDir, walletDbName)
	keystorePath := filepath.Join(netDir, keystore.Filename)

	if !fileExists(dbPath) && !fileExists(keystorePath) {

		// Ensure the data directory for the network exists.
		if err := checkCreateDir(netDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return err
		}

		if cfg.Wizard {
			// Run the wallet creation wizard.
			if err := createWalletWizard(cfg); err != nil {
				printWalletErrorSetup(err)
				return err
			}
		} else {
			// Run the rpc wallet creation server
			err := startInitServer(cfg)
			if err != nil {
				log.Infof("Init Server threw: %s", err)
				return err
			}
		}
	}
	return nil
}

func startInitServer(cfg *config) error {
	listenAddr := net.JoinHostPort("", cfg.Profile)
	log.Infof("Initialization server listening on %s", listenAddr)

	server, err := newRPCServer(cfg.SvrListeners, cfg.RPCMaxClients, 1)
	if err != nil {
		return err
	}
	server.limitedStart()
	//time.Sleep(10 * time.Second)
	//server.Stop()
	<-server.quit

	return nil
}

func (s *rpcServer) limitedStart() {
	s.handlerLookup = limitedHandlerLookup
	s.Start()
}

func limitedHandlerLookup(method string) (f requestHandler, ok bool) {
	f, ok = limitedRpcHandlers[method]
	return
}

var limitedRpcHandlers = map[string]requestHandler{
	"getinfo":          Unsupported,
	"walletsetup":      WalletSetupParams,
	"walletstatecheck": uninitializedState,
}

// uninitializedState responds to walletstate check requests with a
// response that indicates the wallet is in its initial un-created
// state.
func uninitializedState(*Wallet, *chain.Client, btcjson.Cmd) (interface{}, error) {
	infoResult := &btcjson.InfoResult{
		Proxy: version(),
	}
	return infoResult, nil
}

func WalletSetupParams(*Wallet, *chain.Client, btcjson.Cmd) (interface{}, error) {
	infoResult := &btcjson.InfoResult{
		Proxy:  version(),
		Blocks: 9001,
	}
	// Configure the wallet

	// If successful pull the stop channel lever.
	return infoResult, nil
}
