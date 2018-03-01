// Copyright (c) 2017, The dcrdata developers
// See LICENSE for details.

package explorer

import (
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrdata/txhelpers"

	"github.com/dustin/go-humanize"
)

func (exp *explorerUI) mempoolMonitor(txChan chan *NewMempoolTx) {
	exp.storeMempoolInfo()
	for {
		ntx, ok := <-txChan
		if !ok {
			log.Infof("New Tx channel closed")
			return
		}

		// A nil tx is the signal to stop
		if ntx == nil {
			return
		}

		// A tx with an empty hex is the new block signal
		if ntx.Hex == "" {
			exp.storeMempoolInfo()
			exp.wsHub.HubRelay <- sigMempoolUpdate
			continue
		}

		// Ignore this tx if it was received before the last block
		exp.NewBlockDataMtx.Lock()
		lastBlockTime := exp.NewBlockData.BlockTime
		exp.NewBlockDataMtx.Unlock()

		if ntx.Time < lastBlockTime {
			continue
		}

		msgTx, err := txhelpers.MsgTxFromHex(ntx.Hex)
		if err != nil {
			continue
		}

		hash := msgTx.TxHash().String()

		var voteInfo *VoteInfo
		if ok := stake.IsSSGen(msgTx); ok {
			validation, version, bits, choices, err := txhelpers.SSGenVoteChoices(msgTx, exp.ChainParams)
			if err != nil {
				log.Debugf("Cannot get vote choices for %s", hash)
			} else {
				voteInfo = &VoteInfo{
					Validation: BlockValidation{
						Hash:     validation.Hash.String(),
						Height:   validation.Height,
						Validity: validation.Validity,
					},
					Version: version,
					Bits:    bits,
					Choices: choices,
				}
			}
		}

		tx := MempoolTx{
			Hash:     hash,
			Time:     ntx.Time,
			Size:     int32(len(ntx.Hex) / 2),
			TotalOut: txhelpers.TotalOutFromMsgTx(msgTx).ToCoin(),
			Type:     txhelpers.DetermineTxTypeString(msgTx),
			VoteInfo: voteInfo,
		}

		exp.MempoolData.Lock()
		// Add the tx to the appropriate tx slice and update the count
		switch tx.Type {
		case "Ticket":
			exp.MempoolData.Tickets = append([]MempoolTx{tx}, exp.MempoolData.Tickets...)
			exp.MempoolData.NumTickets++
		case "Vote":
			exp.MempoolData.Votes = append([]MempoolTx{tx}, exp.MempoolData.Votes...)
			exp.MempoolData.NumVotes++
		case "Regular":
			exp.MempoolData.Transactions = append([]MempoolTx{tx}, exp.MempoolData.Transactions...)
			exp.MempoolData.NumRegular++
		case "Revocation":
			exp.MempoolData.Revocations = append([]MempoolTx{tx}, exp.MempoolData.Revocations...)
			exp.MempoolData.NumRevokes++
		}

		exp.MempoolData.LatestTransactions = append([]MempoolTx{tx}, exp.MempoolData.LatestTransactions[:len(exp.MempoolData.LatestTransactions)-1]...)

		exp.MempoolData.NumAll++
		exp.MempoolData.TotalOut += tx.TotalOut
		exp.MempoolData.TotalSize += tx.Size
		exp.MempoolData.FormattedTotalSize = humanize.Bytes(uint64(exp.MempoolData.TotalSize))

		exp.MempoolData.Unlock()
		exp.wsHub.HubRelay <- sigNewTx
		exp.wsHub.NewTxChan <- &tx
	}
}

func (exp *explorerUI) StopMempoolMonitor(txChan chan *NewMempoolTx) {
	log.Infof("Stopping mempool monitor")
	txChan <- nil
}

func (exp *explorerUI) StartMempoolMonitor(newTxChan chan *NewMempoolTx) {
	go exp.mempoolMonitor(newTxChan)
}

func (exp *explorerUI) storeMempoolInfo() {

	defer func(start time.Time) {
		log.Debugf("storeMempoolInfo() completed in %v",
			time.Since(start))
	}(time.Now())

	memtxs := exp.blockData.GetMempool()

	if memtxs == nil {
		log.Error("Could not get mempool transactions")
		return
	}
	sort.Sort(byTime(memtxs))

	var latest []MempoolTx
	if len(memtxs) > 5 {
		latest = memtxs[:5]
	} else {
		latest = memtxs
	}

	tickets := make([]MempoolTx, 0)
	votes := make([]MempoolTx, 0)
	revs := make([]MempoolTx, 0)
	regular := make([]MempoolTx, 0)

	var totalOut float64
	var totalSize int32

	for _, tx := range memtxs {
		switch tx.Type {
		case "Ticket":
			tickets = append(tickets, tx)
		case "Vote":
			votes = append(votes, tx)
		case "Revocation":
			revs = append(revs, tx)
		default:
			regular = append(regular, tx)
		}
		totalOut += tx.TotalOut
		totalSize += tx.Size
	}

	exp.NewBlockDataMtx.Lock()
	lastBlock := exp.NewBlockData.Height
	lastBlockTime := exp.NewBlockData.BlockTime
	exp.NewBlockDataMtx.Unlock()

	exp.MempoolData.Lock()
	defer exp.MempoolData.Unlock()

	exp.MempoolData.Transactions = regular
	exp.MempoolData.Tickets = tickets
	exp.MempoolData.Revocations = revs
	exp.MempoolData.Votes = votes

	exp.MempoolData.MempoolShort = MempoolShort{
		LastBlockHeight:    lastBlock,
		LastBlockTime:      lastBlockTime,
		TotalOut:           totalOut,
		TotalSize:          totalSize,
		NumAll:             len(memtxs),
		NumTickets:         len(tickets),
		NumVotes:           len(votes),
		NumRegular:         len(regular),
		NumRevokes:         len(revs),
		LatestTransactions: latest,
		FormattedTotalSize: humanize.Bytes(uint64(totalSize)),
	}
}

type byTime []MempoolTx

func (txs byTime) Less(i, j int) bool {
	return txs[i].Time > txs[j].Time
}

func (txs byTime) Len() int {
	return len(txs)
}

func (txs byTime) Swap(i, j int) {
	txs[i], txs[j] = txs[j], txs[i]
}
