package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcutil/hdkeychain"
	"github.com/btcsuite/btcwallet/legacy/keystore"
	"github.com/soapboxsys/ombudslib/rpcexten"
	"github.com/soapboxsys/ombwallet/chain"
)

var setupChan chan struct{}

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
	setupChan = make(chan struct{}, 10)

	server, err := newRPCServer(cfg.SvrListeners, cfg.RPCMaxClients, 5)
	if err != nil {
		return err
	}
	server.limitedStart()
	//time.Sleep(10 * time.Second)
	//server.Stop()
	<-setupChan
	server.Stop()

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
	"getinfo":        Unsupported,
	"walletsetup":    WalletSetupParams,
	"getwalletstate": uninitializedState,
}

// initializedState checks to see if the wallet and chain server is
// connected and responds appropriately.
func initializedState(w *Wallet, chain *chain.Client, cmd btcjson.Cmd) (interface{}, error) {

	res := &rpcexten.GetWalletStateResult{}
	if chain != nil {
		res.HasChainSvr = chain.IsConnected()
	}

	if w != nil {
		res.HasWallet = true
		res.ChainSynced = w.ChainSynced()
	}

	return res, nil
}

// uninitializedState responds to walletstate check requests with a
// response that indicates the wallet is in its initial un-created
// state.
func uninitializedState(*Wallet, *chain.Client, btcjson.Cmd) (interface{}, error) {
	stateRes := &rpcexten.GetWalletStateResult{
		HasWallet:   false,
		HasChainSvr: false,
	}
	return stateRes, nil
}

func WalletSetupParams(w *Wallet, c *chain.Client, icmd btcjson.Cmd) (interface{}, error) {
	cmd := icmd.(*rpcexten.WalletSetupCmd)

	// Assert that the passphrase is at least 6 characters
	var privPass = []byte(cmd.Passphrase)
	if len(privPass) < 6 {
		msg := "Wallet Passphrase is too short!"
		log.Infof(msg)
		return nil, btcjson.Error{
			Code:    -1,
			Message: msg,
		}
	}

	// Configure the wallet
	seed, err := hdkeychain.GenerateSeed(hdkeychain.RecommendedSeedLen)
	if err != nil {
		log.Infof("Generating a seed failed with: %s", err)
		return nil, btcjson.ErrWallet
	}

	err = CreateWallet(cfg, seed, privPass, []byte(defaultPubPassphrase))
	if err != nil {
		log.Infof("Creating wallet failed with: %s", err)
		return nil, btcjson.ErrWallet
	}

	addr, err := GetInitialAddress()
	if err != nil {
		log.Infof("Creating the first addr failed: %s", err)
		return nil, btcjson.ErrWallet
	}

	// If successful pull the stop lever.
	close(setupChan)

	return addr.String(), nil
}
