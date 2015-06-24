package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/btcsuite/btcwallet/legacy/keystore"
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
			return startInitServer(cfg)
		}
	}
	return nil
}

func startInitServer(cfg *config) error {
	listenAddr := net.JoinHostPort("", cfg.Profile)
	log.Infof("Initialization server listening on %s", listenAddr)
	time.Sleep(5 * time.Second)
	return fmt.Errorf("Timeout fired")
}
