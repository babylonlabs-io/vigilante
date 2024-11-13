package btcscanner_test

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/monitor/btcscanner"
	vdatagen "github.com/babylonlabs-io/vigilante/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func FuzzBootStrap(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))
		k := uint32(datagen.RandomIntOtherThan(r, 0, 10))
		// Generate a random number of blocks
		numBlocks := datagen.RandomIntOtherThan(r, 0, 100) + uint64(k) // make sure we have at least k+1 entry
		chainIndexedBlocks := vdatagen.GetRandomIndexedBlocks(r, numBlocks)
		baseHeight := chainIndexedBlocks[0].Height
		bestHeight := chainIndexedBlocks[len(chainIndexedBlocks)-1].Height
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockBtcClient := mocks.NewMockBTCClient(ctl)
		confirmedBlocks := chainIndexedBlocks[:numBlocks-uint64(k)]
		mockBtcClient.EXPECT().GetBestBlock().Return(bestHeight, nil)
		for i := 0; i < int(numBlocks); i++ {
			mockBtcClient.EXPECT().GetBlockByHeight(gomock.Eq(chainIndexedBlocks[i].Height)).
				Return(chainIndexedBlocks[i], nil, nil).AnyTimes()
		}

		cache, err := types.NewBTCCache(uint32(numBlocks))
		require.NoError(t, err)
		var btcScanner btcscanner.BtcScanner
		btcScanner.SetBtcClient(mockBtcClient)
		btcScanner.SetBaseHeight(baseHeight)
		btcScanner.SetK(k)
		btcScanner.SetConfirmedBlocksChan(make(chan *types.IndexedBlock))
		btcScanner.SetUnconfirmedBlockCache(cache)

		logger, err := config.NewRootLogger("auto", "debug")
		require.NoError(t, err)
		btcScanner.SetLogger(logger.Sugar())

		go func() {
			for i := 0; i < len(confirmedBlocks); i++ {
				b := <-btcScanner.GetConfirmedBlocksChan()
				require.Equal(t, confirmedBlocks[i].BlockHash(), b.BlockHash())
			}
		}()
		btcScanner.Bootstrap()
	})
}
