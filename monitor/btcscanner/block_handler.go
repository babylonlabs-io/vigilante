package btcscanner

import (
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/chainntnfs"
)

// bootstrapAndBlockEventHandler handles connected and disconnected blocks from the BTC client.
func (bs *BtcScanner) bootstrapAndBlockEventHandler() {
	defer bs.wg.Done()

	bs.Bootstrap()

	var blockEpoch *chainntnfs.BlockEpoch
	bestKnownBlock := bs.UnconfirmedBlockCache.Tip()
	if bestKnownBlock != nil {
		hash := bestKnownBlock.BlockHash()
		blockEpoch = &chainntnfs.BlockEpoch{
			Hash:        &hash,
			Height:      bestKnownBlock.Height,
			BlockHeader: bestKnownBlock.Header,
		}
	}
	// register the notifier with the best known tip
	blockNotifier, err := bs.btcNotifier.RegisterBlockEpochNtfn(blockEpoch)
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

			if err := bs.handleNewBlock(epoch.Height, epoch.BlockHeader); err != nil {
				bs.logger.Warnf("failed to handle block at height %d: %s, "+
					"need to restart the bootstrapping process", epoch.Height, err.Error())
				bs.Bootstrap()
			}
		}
	}
}

// handleNewBlock handles blocks from the BTC client
// if new confirmed blocks are found, send them through the channel
func (bs *BtcScanner) handleNewBlock(height int32, header *wire.BlockHeader) error {
	// get cache tip
	cacheTip := bs.UnconfirmedBlockCache.Tip()
	if cacheTip == nil {
		return errors.New("no unconfirmed blocks found")
	}

	if cacheTip.Height >= height {
		bs.logger.Debugf(
			"the connecting block (height: %d, hash: %s) is too early, skipping the block",
			height,
			header.BlockHash().String(),
		)
		return nil
	}

	if cacheTip.Height+1 < height {
		return fmt.Errorf("missing blocks, expected block height: %d, got: %d", cacheTip.Height+1, height)
	}

	parentHash := header.PrevBlock
	// if the parent of the block is not the tip of the cache, then the cache is not up-to-date
	if parentHash != cacheTip.BlockHash() {
		return errors.New("cache is not up-to-date")
	}

	// get the block from hash
	blockHash := header.BlockHash()
	ib, _, err := bs.BtcClient.GetBlockByHash(&blockHash)
	if err != nil {
		// failing to request the block, which means a bug
		panic(fmt.Errorf("failed to request block by hash: %s", blockHash.String()))
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
