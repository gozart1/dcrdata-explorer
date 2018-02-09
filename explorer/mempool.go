package explorer

import (
	"sort"

	"github.com/decred/dcrd/dcrjson"
)

func (exp *explorerUI) mempoolMonitor(txChan chan *NewMempoolTx) {
	exp.storeMempoolInfo()
	for {
		tx, ok := <-txChan
		if !ok {
			log.Infof("New Tx channel closed")
			return
		}

		// A nil tx is the signal to stop
		if tx == nil {
			return
		}

		// A tx with an empty Type is the new block signal
		if tx.Type == "" {
			exp.storeMempoolInfo()
			exp.wsHub.HubRelay <- sigMempoolUpdate
			continue
		}

		// Ignore this tx if it was received before the last block
		exp.NewBlockDataMtx.Lock()
		lastBlockTime := exp.NewBlockData.BlockTime
		exp.NewBlockDataMtx.Unlock()

		if tx.Time > lastBlockTime {
			continue
		}

		exp.MempoolData.Lock()
		// Add the tx to the appropriate tx slice and update the count
		switch tx.Type {
		case "Ticket":
			exp.MempoolData.NumTickets++
			exp.MempoolData.Tickets = append([]MempoolTx{tx.MempoolTx}, exp.MempoolData.Tickets...)
		case "Vote":
			exp.MempoolData.NumVotes++
		case "Regular":
			exp.MempoolData.Transactions = append([]MempoolTx{tx.MempoolTx}, exp.MempoolData.Transactions...)
		case "Revocation":
			log.Trace("Received revoke transaction")
		}
		exp.MempoolData.Unlock()
		exp.wsHub.HubRelay <- sigNewTx
		exp.wsHub.NewTxChan <- tx
	}
}

func (exp *explorerUI) StopMempoolMonitor(txChan chan *NewMempoolTx) {
	log.Infof("Stopping mempool monitor")
	txChan <- nil
}

func (exp *explorerUI) storeMempoolInfo() {
	exp.MempoolData.Lock()
	tickets := mempoolTxs(exp.blockData.GetMempool(dcrjson.GRMTickets))
	if tickets == nil {
		log.Error("Could not get mempool tickets")
	}

	sort.Sort(tickets)

	txs := mempoolTxs(exp.blockData.GetMempool(dcrjson.GRMRegular))
	if txs == nil {
		log.Error("Could not get mempool transactions")
	}
	sort.Sort(txs)

	votes := exp.blockData.GetMempool(dcrjson.GRMVotes)
	if votes == nil {
		log.Error("Could not get mempool votes")
	}

	exp.MempoolData.NumTickets = uint32(tickets.Len())
	exp.MempoolData.NumVotes = uint32(len(votes))
	exp.MempoolData.Tickets = tickets
	exp.MempoolData.Transactions = txs

	exp.MempoolData.Unlock()
}

type mempoolTxs []MempoolTx

func (txs mempoolTxs) Less(i, j int) bool {
	return txs[i].Time > txs[j].Time
}

func (txs mempoolTxs) Len() int {
	return len(txs)
}

func (txs mempoolTxs) Swap(i, j int) {
	txs[i], txs[j] = txs[j], txs[i]
}
