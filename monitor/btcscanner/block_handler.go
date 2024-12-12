package btcscanner

import (
	"errors"
	"fmt"
	"math"

	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/chainntnfs"
)

// bootstrapAndBlockEventHandler handles connected and disconnected blocks from the BTC client.
func (bs *BtcScanner) bootstrapAndBlockEventHandler() {
	defer bs.wg.Done()

	bs.Bootstrap()

	var blockEpoch *chainntnfs.BlockEpoch
	bestKnownBlock := bs.unconfirmedBlockCache.Tip()
	if bestKnownBlock != nil {
		if bestKnownBlock.Height > math.MaxInt32 {
			panic(fmt.Errorf("block height exceeds int32 range: %d", bestKnownBlock.Height))
		}
		hash := bestKnownBlock.BlockHash()
		blockEpoch = &chainntnfs.BlockEpoch{
			Hash:        &hash,
			Height:      int32(bestKnownBlock.Height),
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
			bs.btcClient.Stop()

			return
		case epoch, open := <-blockNotifier.Epochs:
			if !open {
				bs.logger.Errorf("Block event channel is closed")

				return // channel closed
			}

			if epoch.Height < 0 {
				panic(fmt.Errorf("received negative epoch height: %d", epoch.Height)) // software bug, panic
			}

			if err := bs.handleNewBlock(uint32(epoch.Height), epoch.BlockHeader); err != nil {
				bs.logger.Warnf("failed to handle block at height %d: %s, "+
					"need to restart the bootstrapping process", epoch.Height, err.Error())
				bs.Bootstrap()
			}
		}
	}
}

// handleNewBlock handles blocks from the BTC client
// if new confirmed blocks are found, send them through the channel
func (bs *BtcScanner) handleNewBlock(height uint32, header *wire.BlockHeader) error {
	// get cache tip
	cacheTip := bs.unconfirmedBlockCache.Tip()
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
	ib, _, err := bs.btcClient.GetBlockByHash(&blockHash)
	if err != nil {
		// failing to request the block, which means a bug
		panic(fmt.Errorf("failed to request block by hash: %s", blockHash.String()))
	}

	// otherwise, add the block to the cache
	bs.unconfirmedBlockCache.Add(ib)

	if bs.unconfirmedBlockCache.Size() > math.MaxInt32 {
		panic(fmt.Errorf("unconfirmedBlockCache exceeds uint32"))
	}

	// still unconfirmed
	// #nosec G115 -- performed the conversion check above
	if uint32(bs.unconfirmedBlockCache.Size()) <= bs.k {
		return nil
	}

	confirmedBlocks := bs.unconfirmedBlockCache.TrimConfirmedBlocks(int(bs.k))
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
