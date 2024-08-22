package btcscanner

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/wire"
)

// blockEventHandler handles connected and disconnected blocks from the BTC client.
func (bs *BtcScanner) blockEventHandler() {
	defer bs.wg.Done()

	if err := bs.btcNotifier.Start(); err != nil {
		bs.logger.Errorf("Failed starting notifier")
		return
	}

	blockNotifier, err := bs.btcNotifier.RegisterBlockEpochNtfn(nil)
	if err != nil {
		bs.logger.Errorf("Failed registering block epoch notifier")
		return
	}

	defer blockNotifier.Cancel()

	for {
		select {
		case <-bs.quit:
			bs.BtcClient.Stop()
			return
		case epoch, open := <-blockNotifier.Epochs:
			if !open {
				bs.logger.Errorf("Block event channel is closed")
				return // channel closed
			}

			tip := bs.UnconfirmedBlockCache.Tip()

			// Determine if a reorg happened to we know which flow to continue
			// if the new block has the same height but a different hash.
			reorg := false
			if tip != nil {
				if epoch.Height < tip.Height ||
					(epoch.Height == tip.Height && epoch.BlockHeader.BlockHash() != tip.Header.BlockHash()) {
					reorg = true
				}
			}

			if !reorg {
				if err := bs.handleConnectedBlocks(epoch.BlockHeader); err != nil {
					bs.logger.Warnf("failed to handle a connected block at height %d: %s, "+
						"need to restart the bootstrapping process", epoch.Height, err.Error())
					if bs.Synced.Swap(false) {
						bs.Bootstrap()
					}
				}
			} else {
				if err := bs.handleDisconnectedBlocks(epoch.BlockHeader); err != nil {
					bs.logger.Warnf("failed to handle a disconnected block at height %d: %s,"+
						"need to restart the bootstrapping process", epoch.Height, err.Error())
					if bs.Synced.Swap(false) {
						bs.Bootstrap()
					}
				}
			}
		}
	}
}

// handleConnectedBlocks handles connected blocks from the BTC client
// if new confirmed blocks are found, send them through the channel
func (bs *BtcScanner) handleConnectedBlocks(header *wire.BlockHeader) error {
	if !bs.Synced.Load() {
		return errors.New("the btc scanner is not synced")
	}

	// get the block from hash
	blockHash := header.BlockHash()
	ib, _, err := bs.BtcClient.GetBlockByHash(&blockHash)
	if err != nil {
		// failing to request the block, which means a bug
		panic(err)
	}

	// get cache tip
	cacheTip := bs.UnconfirmedBlockCache.Tip()
	if cacheTip == nil {
		return errors.New("no unconfirmed blocks found")
	}

	parentHash := ib.Header.PrevBlock

	// if the parent of the block is not the tip of the cache, then the cache is not up-to-date
	if parentHash != cacheTip.BlockHash() {
		return errors.New("cache is not up-to-date")
	}

	// otherwise, add the block to the cache
	bs.UnconfirmedBlockCache.Add(ib)

	// still unconfirmed
	if bs.UnconfirmedBlockCache.Size() <= bs.K {
		return nil
	}

	confirmedBlocks := bs.UnconfirmedBlockCache.TrimConfirmedBlocks(int(bs.K))
	if confirmedBlocks == nil {
		return nil
	}

	confirmedTipHash := bs.confirmedTipBlock.BlockHash()
	if !confirmedTipHash.IsEqual(&confirmedBlocks[0].Header.PrevBlock) {
		panic("invalid canonical chain")
	}

	bs.sendConfirmedBlocksToChan(confirmedBlocks)

	return nil
}

// handleDisconnectedBlocks handles disconnected blocks from the BTC client.
func (bs *BtcScanner) handleDisconnectedBlocks(header *wire.BlockHeader) error {
	// get cache tip
	cacheTip := bs.UnconfirmedBlockCache.Tip()
	if cacheTip == nil {
		return errors.New("cache is empty")
	}

	// if the block to be disconnected is not the tip of the cache, then the cache is not up-to-date,
	if header.BlockHash() != cacheTip.BlockHash() {
		return errors.New("cache is out-of-sync")
	}

	// otherwise, remove the block from the cache
	if err := bs.UnconfirmedBlockCache.RemoveLast(); err != nil {
		return fmt.Errorf("failed to remove last block from cache: %v", err)
	}

	return nil
}
