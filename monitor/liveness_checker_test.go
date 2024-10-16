package monitor_test

import (
	"math/rand"
	"testing"

	bbndatagen "github.com/babylonlabs-io/babylon/testutil/datagen"
	bbntypes "github.com/babylonlabs-io/babylon/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	monitortypes "github.com/babylonlabs-io/babylon/x/monitor/types"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/monitor"
	"github.com/babylonlabs-io/vigilante/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func FuzzLivenessChecker(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockBabylonClient := monitor.NewMockBabylonQueryClient(ctl)
		cr := datagen.GenerateRandomCheckpointRecord(r)
		maxGap := bbndatagen.RandomIntOtherThan(r, 0, 50) + 200
		cfg := &config.MonitorConfig{MaxLiveBtcHeights: maxGap}
		m := &monitor.Monitor{
			Cfg: cfg,
			// to disable the retry
			ComCfg: &config.CommonConfig{
				RetrySleepTime:    1,
				MaxRetrySleepTime: 0,
				MaxRetryTimes:     1,
			},
			BBNQuerier: mockBabylonClient,
		}
		logger, err := config.NewRootLogger("auto", "debug")
		require.NoError(t, err)
		m.SetLogger(logger.Sugar())

		// 1. normal case, checkpoint is reported, h1 < h2 < h3, h3 - h1 < MaxLiveBtcHeights
		h1 := bbndatagen.RandomIntOtherThan(r, 0, 50)
		h2 := bbndatagen.RandomIntOtherThan(r, 0, 50) + h1
		cr.FirstSeenBtcHeight = uint32(h2)
		h3 := bbndatagen.RandomIntOtherThan(r, 0, 50) + h2
		mockBabylonClient.EXPECT().EndedEpochBTCHeight(gomock.Eq(cr.EpochNum())).Return(
			&monitortypes.QueryEndedEpochBtcHeightResponse{BtcLightClientHeight: uint32(h1)}, nil,
		).AnyTimes()
		mockBabylonClient.EXPECT().ReportedCheckpointBTCHeight(gomock.Eq(cr.ID())).Return(
			&monitortypes.QueryReportedCheckpointBtcHeightResponse{BtcLightClientHeight: uint32(h3)}, nil,
		)
		err = m.CheckLiveness(cr)
		require.NoError(t, err)

		// 2. attack case, checkpoint is reported, h1 < h2 < h3, h3 - h1 > MaxLiveBtcHeights
		h3 = bbndatagen.RandomIntOtherThan(r, 0, 50) + h2 + maxGap
		mockBabylonClient.EXPECT().ReportedCheckpointBTCHeight(gomock.Eq(cr.ID())).Return(
			&monitortypes.QueryReportedCheckpointBtcHeightResponse{BtcLightClientHeight: uint32(h3)}, nil,
		)
		err = m.CheckLiveness(cr)
		require.ErrorIs(t, err, types.ErrLivenessAttack)

		// 3. normal case, checkpoint is not reported, h1 < h2 < h4, h4 - h1 < MaxLiveBtcHeights
		h4 := bbndatagen.RandomIntOtherThan(r, 0, 50) + h2
		mockBabylonClient.EXPECT().ReportedCheckpointBTCHeight(gomock.Eq(cr.ID())).Return(
			&monitortypes.QueryReportedCheckpointBtcHeightResponse{BtcLightClientHeight: uint32(0)},
			monitortypes.ErrCheckpointNotReported,
		)
		randHashBytes := bbntypes.BTCHeaderHashBytes(bbndatagen.GenRandomByteArray(r, 32))
		mockBabylonClient.EXPECT().BTCHeaderChainTip().Return(
			&btclctypes.QueryTipResponse{
				Header: &btclctypes.BTCHeaderInfoResponse{Height: uint32(h4), HashHex: randHashBytes.MarshalHex()}},
			nil,
		)
		err = m.CheckLiveness(cr)
		require.NoError(t, err)

		// 4. attack case, checkpoint is not reported, h1 < h2 < h4, h4 - h1 > MaxLiveBtcHeights
		h4 = bbndatagen.RandomIntOtherThan(r, 0, 50) + h2 + maxGap
		mockBabylonClient.EXPECT().ReportedCheckpointBTCHeight(gomock.Eq(cr.ID())).Return(
			&monitortypes.QueryReportedCheckpointBtcHeightResponse{BtcLightClientHeight: uint32(0)},
			monitortypes.ErrCheckpointNotReported,
		)
		randHashBytes = bbntypes.BTCHeaderHashBytes(bbndatagen.GenRandomByteArray(r, 32))
		mockBabylonClient.EXPECT().BTCHeaderChainTip().Return(
			&btclctypes.QueryTipResponse{
				Header: &btclctypes.BTCHeaderInfoResponse{Height: uint32(h4), HashHex: randHashBytes.MarshalHex()},
			},
			nil,
		)
		err = m.CheckLiveness(cr)
		require.ErrorIs(t, err, types.ErrLivenessAttack)
	})
}
