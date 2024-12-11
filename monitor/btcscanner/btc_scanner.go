package btcscanner

import (
	"fmt"
	"sync"

	notifier "github.com/lightningnetwork/lnd/chainntnfs"

	"github.com/babylonlabs-io/babylon/btctxformatter"
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/types"
)

var _ Scanner = (*BtcScanner)(nil)

type Scanner interface {
	Start(startHeight uint32)
	Stop()

	GetCheckpointsChan() chan *types.CheckpointRecord
	GetConfirmedBlocksChan() chan *types.IndexedBlock
	GetBaseHeight() uint32
}

type BtcScanner struct {
	mu     sync.Mutex
	logger *zap.SugaredLogger

	// connect to BTC node
	btcClient   btcclient.BTCClient
	btcNotifier notifier.ChainNotifier

	// the BTC height the scanner starts
	baseHeight uint32
	// the BTC confirmation depth
	k uint32

	confirmedTipBlock   *types.IndexedBlock
	confirmedBlocksChan chan *types.IndexedBlock

	// cache of a sequence of checkpoints
	ckptCache *types.CheckpointCache
	// cache of a sequence of unconfirmed blocks
	unconfirmedBlockCache *types.BTCCache

	// communicate with the monitor
	blockHeaderChan chan *wire.BlockHeader
	checkpointsChan chan *types.CheckpointRecord

	wg      sync.WaitGroup
	started *atomic.Bool
	quit    chan struct{}
}

func New(
	monitorCfg *config.MonitorConfig,
	parentLogger *zap.Logger,
	btcClient btcclient.BTCClient,
	btcNotifier notifier.ChainNotifier,
	checkpointTag []byte,
) (*BtcScanner, error) {
	headersChan := make(chan *wire.BlockHeader, monitorCfg.BtcBlockBufferSize)
	confirmedBlocksChan := make(chan *types.IndexedBlock, monitorCfg.BtcBlockBufferSize)
	ckptsChan := make(chan *types.CheckpointRecord, monitorCfg.CheckpointBufferSize)
	ckptCache := types.NewCheckpointCache(checkpointTag, btctxformatter.CurrentVersion)
	unconfirmedBlockCache, err := types.NewBTCCache(monitorCfg.BtcCacheSize)
	if err != nil {
		panic(fmt.Errorf("failed to create BTC cache for tail blocks: %w", err))
	}

	return &BtcScanner{
		logger:                parentLogger.With(zap.String("module", "btcscanner")).Sugar(),
		btcClient:             btcClient,
		btcNotifier:           btcNotifier,
		k:                     monitorCfg.BtcConfirmationDepth,
		ckptCache:             ckptCache,
		unconfirmedBlockCache: unconfirmedBlockCache,
		confirmedBlocksChan:   confirmedBlocksChan,
		blockHeaderChan:       headersChan,
		checkpointsChan:       ckptsChan,
		started:               atomic.NewBool(false),
		quit:                  make(chan struct{}),
	}, nil
}

// Start starts the scanning process from curBTCHeight to tipHeight
func (bs *BtcScanner) Start(startHeight uint32) {
	if bs.started.Load() {
		bs.logger.Info("the BTC scanner is already started")

		return
	}

	bs.SetBaseHeight(startHeight)

	bs.started.Store(true)
	bs.logger.Info("the BTC scanner is started")

	if err := bs.btcNotifier.Start(); err != nil {
		bs.logger.Errorf("Failed starting notifier")

		return
	}

	// start handling new blocks
	bs.wg.Add(1)
	go bs.bootstrapAndBlockEventHandler()

	for bs.started.Load() {
		select {
		case <-bs.quit:
			bs.started.Store(false)
		case block := <-bs.confirmedBlocksChan:
			bs.logger.Debugf("found a confirmed BTC block at height %d", block.Height)
			// send the header to the Monitor for consistency check
			bs.blockHeaderChan <- block.Header
			ckptBtc := bs.tryToExtractCheckpoint(block)
			if ckptBtc == nil {
				bs.logger.Debugf("checkpoint not found at BTC block %v", block.Height)
				// move to the next BTC block
				continue
			}
			bs.logger.Infof("found a checkpoint at BTC block %d", ckptBtc.FirstSeenBtcHeight)

			bs.checkpointsChan <- ckptBtc
		}
	}

	bs.wg.Wait()
	bs.logger.Info("the BTC scanner is stopped")
}

// Bootstrap syncs with BTC by getting the confirmed blocks and the caching the unconfirmed blocks
func (bs *BtcScanner) Bootstrap() {
	var (
		firstUnconfirmedHeight uint32
		confirmedBlock         *types.IndexedBlock
		err                    error
	)

	if bs.confirmedTipBlock != nil {
		firstUnconfirmedHeight = bs.confirmedTipBlock.Height + 1
	} else {
		firstUnconfirmedHeight = bs.GetBaseHeight()
	}

	bs.logger.Infof("the bootstrapping starts at %d", firstUnconfirmedHeight)

	// clear all the blocks in the cache to avoid forks
	bs.unconfirmedBlockCache.RemoveAll()

	bestHeight, err := bs.btcClient.GetBestBlock()
	if err != nil {
		panic(fmt.Errorf("cannot get the best BTC block %w", err))
	}

	bestConfirmedHeight := bestHeight - bs.k
	// process confirmed blocks
	for i := firstUnconfirmedHeight; i <= bestConfirmedHeight; i++ {
		ib, _, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			panic(err)
		}

		// this is a confirmed block
		confirmedBlock = ib

		// if the scanner was bootstrapped before, the new confirmed canonical chain must connect to the previous one
		if bs.confirmedTipBlock != nil {
			confirmedTipHash := bs.confirmedTipBlock.BlockHash()
			if !confirmedTipHash.IsEqual(&confirmedBlock.Header.PrevBlock) {
				panic("invalid canonical chain")
			}
		}

		bs.sendConfirmedBlocksToChan([]*types.IndexedBlock{confirmedBlock})
	}

	// add unconfirmed blocks into the cache
	for i := bestConfirmedHeight + 1; i <= bestHeight; i++ {
		ib, _, err := bs.btcClient.GetBlockByHeight(i)
		if err != nil {
			panic(err)
		}

		// the unconfirmed blocks must follow the canonical chain
		tipCache := bs.unconfirmedBlockCache.Tip()
		if tipCache != nil {
			tipHash := tipCache.BlockHash()
			if !tipHash.IsEqual(&ib.Header.PrevBlock) {
				panic("invalid canonical chain")
			}
		}

		bs.unconfirmedBlockCache.Add(ib)
	}

	bs.logger.Infof("bootstrapping is finished at the best confirmed height: %d", bestConfirmedHeight)
}

func (bs *BtcScanner) SetLogger(logger *zap.SugaredLogger) {
	bs.logger = logger
}

func (bs *BtcScanner) GetConfirmedBlocksChan() chan *types.IndexedBlock {
	return bs.confirmedBlocksChan
}

func (bs *BtcScanner) sendConfirmedBlocksToChan(blocks []*types.IndexedBlock) {
	for i := 0; i < len(blocks); i++ {
		bs.confirmedBlocksChan <- blocks[i]
		bs.confirmedTipBlock = blocks[i]
	}
}

func (bs *BtcScanner) tryToExtractCheckpoint(block *types.IndexedBlock) *types.CheckpointRecord {
	found := bs.tryToExtractCkptSegment(block)
	if !found {
		return nil
	}

	rawCheckpointWithBtcHeight, err := bs.matchAndPop()
	if err != nil {
		// if a raw checkpoint is found, it should be decoded. Otherwise
		// this means there are bugs in the program, should panic here
		panic(err)
	}

	return rawCheckpointWithBtcHeight
}

func (bs *BtcScanner) matchAndPop() (*types.CheckpointRecord, error) {
	bs.ckptCache.Match()
	ckptSegments := bs.ckptCache.PopEarliestCheckpoint()
	if ckptSegments == nil {
		return nil, nil
	}
	connectedBytes, err := btctxformatter.ConnectParts(bs.ckptCache.Version, ckptSegments.Segments[0].Data, ckptSegments.Segments[1].Data)
	if err != nil {
		return nil, fmt.Errorf("failed to connect two checkpoint parts: %w", err)
	}
	// found a pair, check if it is a valid checkpoint
	rawCheckpoint, err := ckpttypes.FromBTCCkptBytesToRawCkpt(connectedBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode raw checkpoint bytes: %w", err)
	}

	return &types.CheckpointRecord{
		RawCheckpoint:      rawCheckpoint,
		FirstSeenBtcHeight: ckptSegments.Segments[0].AssocBlock.Height,
	}, nil
}

func (bs *BtcScanner) tryToExtractCkptSegment(b *types.IndexedBlock) bool {
	found := false
	for _, tx := range b.Txs {
		if tx == nil {
			continue
		}

		// cache the segment to ckptCache
		ckptSeg := types.NewCkptSegment(bs.ckptCache.Tag, bs.ckptCache.Version, b, tx)
		if ckptSeg != nil {
			err := bs.ckptCache.AddSegment(ckptSeg)
			if err != nil {
				bs.logger.Errorf("Failed to add the ckpt segment in tx %v to the ckptCache: %v", tx.Hash(), err)

				continue
			}
			found = true
		}
	}

	return found
}

func (bs *BtcScanner) GetCheckpointsChan() chan *types.CheckpointRecord {
	return bs.checkpointsChan
}

func (bs *BtcScanner) Stop() {
	close(bs.quit)
}

func (bs *BtcScanner) GetBaseHeight() uint32 {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	return bs.baseHeight
}

func (bs *BtcScanner) SetBaseHeight(h uint32) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.baseHeight = h
}

// GetBtcClient returns the btcClient.
func (bs *BtcScanner) GetBtcClient() btcclient.BTCClient {
	return bs.btcClient
}

// SetBtcClient sets the btcClient.
func (bs *BtcScanner) SetBtcClient(c btcclient.BTCClient) {
	bs.btcClient = c
}

// GetK returns the value of k - confirmation depth
func (bs *BtcScanner) GetK() uint32 {
	return bs.k
}

// SetK sets the value of k - confirmation depth
func (bs *BtcScanner) SetK(k uint32) {
	bs.k = k
}

// SetConfirmedBlocksChan sets the confirmedBlocksChan.
func (bs *BtcScanner) SetConfirmedBlocksChan(ch chan *types.IndexedBlock) {
	bs.confirmedBlocksChan = ch
}

// GetUnconfirmedBlockCache returns the unconfirmedBlockCache.
func (bs *BtcScanner) GetUnconfirmedBlockCache() *types.BTCCache {
	return bs.unconfirmedBlockCache
}

// SetUnconfirmedBlockCache sets the unconfirmedBlockCache.
func (bs *BtcScanner) SetUnconfirmedBlockCache(c *types.BTCCache) {
	bs.unconfirmedBlockCache = c
}
