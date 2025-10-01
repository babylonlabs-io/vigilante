package monitor

import (
	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/chainntnfs"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"math/rand"
	"testing"
	"time"
)

type ActivationMonitorTestSuite struct {
	t                 *testing.T
	ctrl              *gomock.Controller
	mockBClient       *MockBabylonAdaptorClient
	mockBTCClient     *mocks.MockBTCClient
	activationMonitor *ActivationUnbondingMonitor
	metrics           *metrics.ActivationUnbondingMonitorMetrics
	cfg               config.MonitorConfig
	logger            *zap.Logger
}

func NewActivationMonitorTestSuite(t *testing.T) *ActivationMonitorTestSuite {
	t.Parallel()
	ctl := gomock.NewController(t)
	cfg := config.DefaultMonitorConfig()
	mockBClient := NewMockBabylonAdaptorClient(ctl)
	mockBtcClient := mocks.NewMockBTCClient(ctl)
	logger := zap.NewNop()
	activationMetrics := metrics.NewActivationUnbondingMonitorMetrics()

	activationMonitor := NewActivationUnbondingMonitor(
		mockBClient,
		mockBtcClient,
		&cfg,
		logger,
		activationMetrics,
	)

	return &ActivationMonitorTestSuite{
		t:                 t,
		ctrl:              ctl,
		mockBClient:       mockBClient,
		mockBTCClient:     mockBtcClient,
		activationMonitor: activationMonitor,
		metrics:           activationMetrics,
		cfg:               cfg,
		logger:            logger,
	}
}

func (s *ActivationMonitorTestSuite) CreateDelegations(count int,
	status btcstakingtypes.BTCDelegationStatus) []Delegation {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	dels := make([]Delegation, 0, count)
	for i := 0; i < count; i++ {
		stk := datagen.GenRandomTx(r)
		dels = append(dels, Delegation{
			StakingTx:             stk,
			StakingOutputIdx:      0,
			DelegationStartHeight: 0,
			UnbondingOutput:       nil,
			HasProof:              false,
			Status:                status.String(),
		})
	}

	s.mockBClient.EXPECT().DelegationsByStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(dels, nil, nil).AnyTimes()
	s.mockBClient.EXPECT().BTCDelegation(gomock.Any()).Return(&dels[0], nil).AnyTimes()
	s.mockBTCClient.EXPECT().TxDetails(gomock.Any(), gomock.Any()).Return(
		&chainntnfs.TxConfirmation{
			BlockHeight: 850000,
			BlockHash:   &chainhash.Hash{},
			TxIndex:     1,
		},
		btcclient.TxInChain,
		nil,
	).AnyTimes()

	s.mockBTCClient.EXPECT().GetBestBlock().Return(uint32(850006), nil).AnyTimes()
	s.mockBClient.EXPECT().GetConfirmationDepth().Return(uint32(4), nil).AnyTimes()

	return dels
}

func TestActivationFlow(t *testing.T) {
	t.Parallel()
	s := NewActivationMonitorTestSuite(t)
	defer s.TearDown()

	_ = s.CreateDelegations(5, btcstakingtypes.BTCDelegationStatus_ACTIVE)

	// 0 from point of initialisation
	require.Equal(t, 0.0, promtestutil.ToFloat64(s.metrics.ActivationTimeoutsCounter))
	require.Equal(t, 0.0, promtestutil.ToFloat64(s.metrics.TrackedActivationGauge))
	metric := &dto.Metric{}
	err := s.metrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)

	hist := metric.GetHistogram()
	require.Equal(t, uint64(0), hist.GetSampleCount())
	require.Equal(t, 0.0, hist.GetSampleSum())

	err = s.activationMonitor.CheckActivationTiming()
	require.NoError(t, err)

	// refresh hist
	metric2 := &dto.Metric{}
	err = s.metrics.ActivationDelayHistogram.Write(metric2)
	require.NoError(t, err)
	hist2 := metric2.GetHistogram()

	// should be 5 for the 5 delegations
	require.Equal(t, uint64(5), hist2.GetSampleCount())
	require.Equal(t, -5.0, promtestutil.ToFloat64(s.metrics.TrackedActivationGauge))
	// should not be changed
	require.Equal(t, 0.0, promtestutil.ToFloat64(s.metrics.ActivationTimeoutsCounter))
}

func TestVerifiedFlow(t *testing.T) {
	t.Parallel()
	s := NewActivationMonitorTestSuite(t)
	defer s.TearDown()

	_ = s.CreateDelegations(5, btcstakingtypes.BTCDelegationStatus_VERIFIED)

	// 0 from point of initialisation
	require.Equal(t, 0.0, promtestutil.ToFloat64(s.metrics.ActivationTimeoutsCounter))
	require.Equal(t, 0.0, promtestutil.ToFloat64(s.metrics.TrackedActivationGauge))
	metric := &dto.Metric{}
	err := s.metrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)

	hist := metric.GetHistogram()
	require.Equal(t, uint64(0), hist.GetSampleCount())
	require.Equal(t, 0.0, hist.GetSampleSum())

	err = s.activationMonitor.CheckActivationTiming()
	require.NoError(t, err)

	//refresh hist
	metric2 := &dto.Metric{}
	err = s.metrics.ActivationDelayHistogram.Write(metric2)
	require.NoError(t, err)
	hist2 := metric2.GetHistogram()

	// should not be changed as no active
	require.Equal(t, uint64(0), hist2.GetSampleCount())
	require.Equal(t, 0.0, promtestutil.ToFloat64(s.metrics.TrackedActivationGauge))
	require.Equal(t, 0.0, promtestutil.ToFloat64(s.metrics.ActivationTimeoutsCounter))
}

func (s *ActivationMonitorTestSuite) TearDown() {
	s.ctrl.Finish()
}
