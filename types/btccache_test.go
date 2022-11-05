package types_test

import (
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"

	"github.com/babylonchain/babylon/testutil/datagen"
	vdatagen "github.com/babylonchain/vigilante/testutil/datagen"
	"github.com/babylonchain/vigilante/types"
)

// FuzzBtcCache fuzzes the BtcCache type
// 1. Generates BtcCache with random number of blocks.
// 2. Randomly add or remove blocks.
// 3. Find a random block.
// 4. Remove random blocks.
func FuzzBtcCache(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		rand.Seed(seed)

		// Create a new cache
		maxEntries := datagen.RandomInt(1000)
		cache, err := types.NewBTCCache(maxEntries)
		require.NoError(t, err)

		// Generate a random number of blocks
		numBlocks := datagen.RandomInt(1000)
		ibs := vdatagen.GetRandomIndexedBlocks(numBlocks)

		// Add all indexed blocks to the cache
		err = cache.Init(ibs)
		if numBlocks > maxEntries {
			// if init fails, quit early
			require.ErrorIs(t, err, types.ErrTooManyEntries)
			return
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, numBlocks, cache.Size())

		// Find a random block in the cache
		randIdx := datagen.RandomInt(int(numBlocks))
		randIb := ibs[randIdx]
		randIbHeight := uint64(randIb.Height)
		foundIb := cache.FindBlock(randIbHeight)
		require.NotNil(t, foundIb)
		require.Equal(t, foundIb, randIb)

		// Add random blocks to the cache
		addCount := datagen.RandomIntOtherThan(0, 1000)
		prevCacheHeight := cache.Tip().Height
		newIbs := vdatagen.GetRandomIndexedBlocksFromHeight(addCount, cache.Tip().Height, cache.Tip().BlockHash())
		for _, ib := range newIbs {
			cache.Add(ib)
		}
		require.Equal(t, prevCacheHeight+int32(addCount), cache.Tip().Height)
		require.Equal(t, newIbs[addCount-1], cache.Tip())
		if addCount >= maxEntries {
			// if the number of added blocks is larger than maxEntries, full cache should be compared with slice of newIbs
			require.Equal(t, newIbs[addCount-maxEntries:], cache.GetAllBlocks())
		} else {
			// if the number of added blocks is smaller than maxEntries, new ibs should be compared with slice of cache blocks
			insertedIbs, err := cache.GetLastBlocks(uint64(prevCacheHeight) + 1)
			require.NoError(t, err)
			require.Equal(t, newIbs, insertedIbs)
		}

		// Remove random number of blocks from the cache
		prevSize := cache.Size()
		deleteCount := datagen.RandomInt(int(prevSize))
		for i := 0; i < int(deleteCount); i++ {
			err = cache.RemoveLast()
			require.NoError(t, err)
		}
		require.Equal(t, prevSize-deleteCount, cache.Size())
	})
}
