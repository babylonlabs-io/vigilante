package monitor_test

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/monitor"
	"github.com/babylonlabs-io/vigilante/types"
)

// FuzzQueryInfoForNextEpoch generates validator set with BLS keys and raw checkpoints
// and check whether they are the same as the queried epoch info
func FuzzQueryInfoForNextEpoch(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 10)
	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))
		n := r.Intn(100) + 1
		valSet, blsprivkeys := datagen.GenerateValidatorSetWithBLSPrivKeys(n)
		ckpt := datagen.GenerateLegitimateRawCheckpoint(r, blsprivkeys)
		e := ckpt.EpochNum
		ckptWithMeta := &ckpttypes.RawCheckpointWithMeta{Ckpt: ckpt}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		bbnCli := monitor.NewMockBabylonQueryClient(ctrl)
		bbnCli.EXPECT().BlsPublicKeyList(gomock.Eq(e), gomock.Nil()).Return(
			&ckpttypes.QueryBlsPublicKeyListResponse{
				ValidatorWithBlsKeys: convertToBlsPublicKeyListResponse(valSet.ValSet),
			},
			nil,
		).AnyTimes()
		bbnCli.EXPECT().RawCheckpoint(gomock.Eq(e)).Return(
			&ckpttypes.QueryRawCheckpointResponse{
				RawCheckpoint: ckptWithMeta.ToResponse(),
			},
			nil,
		).AnyTimes()
		expectedEI := types.NewEpochInfo(e, *valSet)
		m := &monitor.Monitor{
			// to disable the retry
			ComCfg: &config.CommonConfig{
				RetrySleepTime:    1,
				MaxRetrySleepTime: 0,
			},
			BBNQuerier: bbnCli,
		}
		ei, err := m.QueryInfoForNextEpoch(e)
		require.NoError(t, err)
		require.True(t, expectedEI.Equal(ei))
	})
}
