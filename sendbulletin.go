package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/soapboxsys/ombudslib/protocol/ombproto"
	"github.com/soapboxsys/ombudslib/rpcexten"
	"github.com/soapboxsys/ombwallet/chain"
	"github.com/soapboxsys/ombwallet/txstore"
)

// Handles a sendbulletin json request. Attempts to send a bulletin from the
// specified address. If the address does not have enough funds or there is some
// other problem then the request throws a resonable error.
func SendBulletin(w *Wallet, chainSrv *chain.Client, icmd btcjson.Cmd) (interface{}, error) {
	log.Trace("Starting SendBulletin")

	msgtx, err := createBulletinTx(w, chainSrv, icmd)
	if err != nil {
		return nil, err
	}

	log.Trace("Inserting new tx into the TxStore.")
	// Handle updating the TxStore
	if err = insertIntoStore(w.TxStore, msgtx); err != nil {
		return nil, err
	}

	txSha, err := chainSrv.SendRawTransaction(msgtx, false)
	if err != nil {
		return nil, err
	}
	log.Infof("Successfully sent bulletin %v", txSha)

	return txSha.String(), nil
}

// ComposeBulletin creates a bulletin using the wallet's internal state and the
// the fields specified in the btcjson.cmd struct. It validates the returned tx
// and if successful, returns that wire.msgtx as a raw hex string.
func ComposeBulletin(w *Wallet, chainSrv *chain.Client, icmd btcjson.Cmd) (interface{}, error) {
	log.Trace("Composing a Bulletin")

	msgtx, err := createBulletinTx(w, chainSrv, icmd)
	if err != nil {
		return nil, err
	}

	mhx, err := messageToHex(msgtx)
	if err != nil {
		return nil, err
	}
	return mhx, nil
}

// createBulletinTx uses the wallets internal structures to produce a bulletin
// from the fields provided in the btcjson.cmd. Before the function returns it
// validates the txin scripts against the internal state of the wallet. If the
// scripts execute successfully, the function returns its 'valid' bulletin.
func createBulletinTx(w *Wallet, chainSrv *chain.Client, icmd btcjson.Cmd) (*wire.MsgTx, error) {
	// NOTE because send and compose have the same fields this works.
	cmd := icmd.(rpcexten.BulletinCmd)

	// NOTE Rapid requests will serially block due to locking
	heldUnlock, err := w.HoldUnlock()
	if err != nil {
		return nil, err
	}
	defer heldUnlock.Release()
	log.Trace("Grabbed wallet lock")

	addr, err := btcutil.DecodeAddress(cmd.GetAddress(), activeNet.Params)
	if err != nil {
		return nil, err
	}
	// NOTE checks to see if addr is in the wallet
	_, err = w.Manager.Address(addr)
	if err != nil {
		log.Trace("The address is not in the manager")
		return nil, err
	}

	bs, err := chainSrv.BlockStamp()
	if err != nil {
		return nil, err
	}

	log.Trace("Looking into elgible outputs")
	// NOTE minconf is set to 1
	var eligible []txstore.Credit
	eligible, err = w.findEligibleOutputs(1, bs)
	if err != nil {
		return nil, err
	}

	msgtx := wire.NewMsgTx()

	// Create the bulletin and add bulletin TxOuts to msgtx
	addrStr, board, msg := cmd.GetAddress(), cmd.GetBoard(), cmd.GetMessage()
	bltn, err := ombproto.NewBulletinFromStr(addrStr, board, msg)
	if err != nil {
		return nil, err
	}
	txouts, err := bltn.TxOuts(ombproto.DustAmnt(), activeNet.Params)
	if err != nil {
		return nil, err
	}
	// The amount of bitcoin burned by sending the bulletin
	var totalBurn btcutil.Amount
	for _, txout := range txouts {
		msgtx.AddTxOut(txout)
		totalBurn += btcutil.Amount(txout.Value)
	}

	log.Trace("Searching for a UTXO with target address.")
	// Find the index of the credit with the target address and use that as the
	// first txin in the bulletin.
	i, err := findAddrCredit(eligible, addr)
	if err != nil {
		log.Trace("No eligible credits found for addr: ", addr)
		return nil, err
	}

	authc := eligible[i]
	// Add authoring txin
	msgtx.AddTxIn(wire.NewTxIn(authc.OutPoint(), nil))

	// Remove the author credit
	eligible = append(eligible[:i], eligible[i+1:]...)
	sort.Sort(sort.Reverse(ByAmount(eligible)))
	totalAdded := authc.Amount()
	inputs := []txstore.Credit{authc}
	var input txstore.Credit

	for totalAdded < totalBurn {
		if len(eligible) == 0 {
			return nil, InsufficientFundsError{totalAdded, totalBurn, 0}
		}
		input, eligible = eligible[0], eligible[1:]
		inputs = append(inputs, input)
		msgtx.AddTxIn(wire.NewTxIn(input.OutPoint(), nil))
		totalAdded += input.Amount()
	}

	log.Trace("Estimating fee")
	// Initial fee estimate
	szEst := estimateTxSize(len(inputs), len(msgtx.TxOut))
	feeEst := minimumFee(w.FeeIncrement, szEst, msgtx.TxOut, inputs, bs.Height)

	// Ensure that we cover the fee and the total burn and if not add another
	// input.
	for totalAdded < totalBurn+feeEst {
		if len(eligible) == 0 {
			return nil, InsufficientFundsError{totalAdded, totalBurn, feeEst}
		}
		input, eligible = eligible[0], eligible[1:]
		inputs = append(inputs, input)
		msgtx.AddTxIn(wire.NewTxIn(input.OutPoint(), nil))
		szEst += txInEstimate
		totalAdded += input.Amount()
		feeEst = minimumFee(w.FeeIncrement, szEst, msgtx.TxOut, inputs, bs.Height)
	}

	// Shameless copy from createtx
	// changeIdx is -1 unless there's a change output.
	changeIdx := -1

	log.Trace("Formulating the transaction and computing fees")
	for {
		change := totalAdded - totalBurn - feeEst
		if change > 0 {
			// Send the change back to the authoring addr.
			pkScript, err := txscript.PayToAddrScript(addr)
			if err != nil {
				return nil, err
			}
			msgtx.AddTxOut(wire.NewTxOut(int64(change), pkScript))

			changeIdx = len(msgtx.TxOut) - 1
			if err != nil {
				return nil, err
			}
		}

		log.Trace("Signing the transaction")
		if err = signMsgTx(msgtx, inputs, w.Manager); err != nil {
			return nil, err
		}

		if feeForSize(w.FeeIncrement, msgtx.SerializeSize()) <= feeEst {
			// The required fee for this size is less than or equal to what
			// we guessed, so we're done.
			break
		}

		if change > 0 {
			// Remove the change output since the next iteration will add
			// it again (with a new amount) if necessary.
			tmp := msgtx.TxOut[:changeIdx]
			tmp = append(tmp, msgtx.TxOut[changeIdx+1:]...)
			msgtx.TxOut = tmp
		}

		feeEst += w.FeeIncrement
		for totalAdded < totalBurn+feeEst {
			if len(eligible) == 0 {
				return nil, InsufficientFundsError{totalAdded, totalBurn, feeEst}
			}
			input, eligible = eligible[0], eligible[1:]
			inputs = append(inputs, input)
			msgtx.AddTxIn(wire.NewTxIn(input.OutPoint(), nil))
			szEst += txInEstimate
			totalAdded += input.Amount()
			feeEst = minimumFee(w.FeeIncrement, szEst, msgtx.TxOut, inputs, bs.Height)
		}
	}

	if err := validateMsgTx(msgtx, inputs); err != nil {
		return nil, err
	}

	return msgtx, nil
}

// TODO NOTICE
var ErrNoUnspentForAddr error = errors.New("No unspent outputs for this address")

// TODO NOTICE finds a credit that is a P2PKH to the target address
func findAddrCredit(credits []txstore.Credit, target btcutil.Address) (int, error) {

	var idx int = -1
	for i, credit := range credits {
		class, addrs, _, err := credit.Addresses(activeNet.Params)
		if err != nil {
			return -1, err
		}
		switch class {
		case txscript.PubKeyHashTy:
			if target.EncodeAddress() == addrs[0].EncodeAddress() {
				idx = i
				break
			}

		// Ignore all non P2PKH txouts
		default:
			continue
		}

	}
	if idx == -1 {
		return -1, ErrNoUnspentForAddr
	}

	return idx, nil
}

// Inserts a new transaction into the TxStore, updating credits and debits
// of the store.
func insertIntoStore(store *txstore.Store, tx *wire.MsgTx) error {
	// Add to the transaction store.
	txr, err := store.InsertTx(btcutil.NewTx(tx), nil)
	if err != nil {
		log.Errorf("Error adding sent tx history: %v", err)
		return btcjson.ErrInternal
	}
	_, err = txr.AddDebits()
	if err != nil {
		log.Errorf("Error adding sent tx history: %v", err)
		return btcjson.ErrInternal
	}
	store.MarkDirty()
	return nil
}

// messageToHex serializes a message to the wire protocol encoding using the
// latest protocol version and returns a hex-encoded string of the result.
func messageToHex(msg wire.Message) (string, error) {
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, wire.ProtocolVersion); err != nil {
		context := fmt.Sprintf("Failed to encode msg of type %T", msg)
		return "", errors.New(context)
	}

	return hex.EncodeToString(buf.Bytes()), nil
}
