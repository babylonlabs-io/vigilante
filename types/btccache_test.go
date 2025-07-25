package types_test

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	vdatagen "github.com/babylonlabs-io/vigilante/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/types"
)

// FuzzBtcCache fuzzes the BtcCache type
// 1. Generates BtcCache with random number of blocks.
// 2. Randomly add or remove blocks.
// 3. Find a random block.
// 4. Remove random blocks.
func FuzzBtcCache(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		// Create a new cache
		maxEntries := datagen.RandomInt(r, 1000) + 2 // make sure we have at least 2 entries

		// 1/10 times generate invalid maxEntries
		invalidMaxEntries := false
		if datagen.OneInN(r, 10) {
			maxEntries = 0
			invalidMaxEntries = true
		}

		cache, err := types.NewBTCCache(uint32(maxEntries))
		if err != nil {
			if !invalidMaxEntries {
				t.Fatalf("NewBTCCache returned error %s", err)
			}
			require.ErrorIs(t, err, types.ErrInvalidMaxEntries)
			t.Skip("Skipping test with invalid maxEntries")
		}

		// Generate a random number of blocks
		numBlocks := datagen.RandomIntOtherThan(r, 0, int(maxEntries)) // make sure we have at least 1 entry

		// 1/10 times generate invalid number of blocks
		invalidNumBlocks := false
		if datagen.OneInN(r, 10) {
			numBlocks = maxEntries + 1
			invalidNumBlocks = true
		}

		ibs := vdatagen.GetRandomIndexedBlocks(r, numBlocks)

		// Add all indexed blocks to the cache
		err = cache.Init(ibs)
		if err != nil {
			if !invalidNumBlocks {
				t.Fatalf("Cache init returned error %v", err)
			}
			require.ErrorIs(t, err, types.ErrTooManyEntries)
			t.Skip("Skipping test with invalid numBlocks")
		}

		require.Equal(t, numBlocks, uint64(cache.Size()))

		// Find a random block in the cache
		randIdx := datagen.RandomInt(r, int(numBlocks))
		randIb := ibs[randIdx]
		randIbHeight := randIb.Height
		foundIb := cache.FindBlock(randIbHeight)
		require.NotNil(t, foundIb)
		require.Equal(t, foundIb, randIb)

		// Add random blocks to the cache
		addCount := datagen.RandomIntOtherThan(r, 0, 1000)
		prevCacheHeight := int32(cache.Tip().Height)
		cacheBlocksBeforeAddition := cache.GetAllBlocks()
		blocksToAdd := vdatagen.GetRandomIndexedBlocksFromHeight(r, addCount, int32(cache.Tip().Height), cache.Tip().BlockHash())
		for _, ib := range blocksToAdd {
			cache.Add(ib)
		}
		require.Equal(t, prevCacheHeight+int32(addCount), int32(cache.Tip().Height))
		require.Equal(t, blocksToAdd[addCount-1], cache.Tip())

		// ensure block heights in cache are in increasing order
		var heights []int32
		for _, ib := range cache.GetAllBlocks() {
			heights = append(heights, int32(ib.Height))
		}
		require.IsIncreasing(t, heights)

		// we need to compare block slices before and after addition, there are 3 cases to consider:
		// if addCount+numBlocks>=maxEntries then
		// 1. addCount >= maxEntries
		// 2. addCount < maxEntries
		// else
		// 3. addCount+numBlocks < maxEntries
		// case 2 and 3 are the same, so below is simplified version
		cacheBlocksAfterAddition := cache.GetAllBlocks()
		if addCount >= maxEntries {
			// cache contains only new blocks, so compare all cache blocks with slice of blocksToAdd
			require.Equal(t, blocksToAdd[addCount-maxEntries:], cacheBlocksAfterAddition)
		} else {
			// cache contains both old and all the new blocks, so we need to compare
			// blocksToAdd with slice of cacheBlocksAfterAddition and also compare
			// slice of cacheBlocksBeforeAddition with slice of cacheBlocksAfterAddition

			// compare new blocks
			newBlocksInCache := cacheBlocksAfterAddition[len(cacheBlocksAfterAddition)-int(addCount):]
			require.Equal(t, blocksToAdd, newBlocksInCache)

			// comparing old blocks
			oldBlocksInCache := cacheBlocksAfterAddition[:len(cacheBlocksAfterAddition)-int(addCount)]
			require.Equal(t, cacheBlocksBeforeAddition[len(cacheBlocksBeforeAddition)-(len(cacheBlocksAfterAddition)-int(addCount)):], oldBlocksInCache)
		}

		// Remove random number of blocks from the cache
		prevSize := uint64(cache.Size())
		deleteCount := datagen.RandomInt(r, int(prevSize))
		cacheBlocksBeforeDeletion := cache.GetAllBlocks()
		for i := 0; i < int(deleteCount); i++ {
			err = cache.RemoveLast()
			require.NoError(t, err)
		}
		cacheBlocksAfterDeletion := cache.GetAllBlocks()
		require.Equal(t, prevSize-deleteCount, uint64(cache.Size()))
		require.Equal(t, cacheBlocksBeforeDeletion[:len(cacheBlocksBeforeDeletion)-int(deleteCount)], cacheBlocksAfterDeletion)
	})
}
