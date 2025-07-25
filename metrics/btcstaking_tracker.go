package metrics

import (
	"github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type BTCStakingTrackerMetrics struct {
	Registry *prometheus.Registry
	*UnbondingWatcherMetrics
	*SlasherMetrics
	*AtomicSlasherMetrics
}

func NewBTCStakingTrackerMetrics() *BTCStakingTrackerMetrics {
	registry := prometheus.NewRegistry()
	uwMetrics := newUnbondingWatcherMetrics(registry)
	slasherMetrics := newSlasherMetrics(registry)
	atomicSlasherMetrics := newAtomicSlasherMetrics(registry)

	return &BTCStakingTrackerMetrics{registry, uwMetrics, slasherMetrics, atomicSlasherMetrics}
}

type UnbondingWatcherMetrics struct {
	Registry                                    *prometheus.Registry
	ReportedUnbondingTransactionsCounter        prometheus.Counter
	FailedReportedUnbondingTransactions         prometheus.Counter
	NumberOfTrackedActiveDelegations            prometheus.Gauge
	DetectedUnbondedStakeExpansionCounter       prometheus.Counter
	DetectedUnbondingTransactionsCounter        prometheus.Counter
	DetectedNonUnbondingTransactionsCounter     prometheus.Counter
	FailedReportedActivateDelegations           prometheus.Counter
	ReportedActivateDelegationsCounter          prometheus.Counter
	NumberOfActivationInProgress                prometheus.Gauge
	NumberOfVerifiedDelegations                 prometheus.Gauge
	NumberOfVerifiedNotInChainDelegations       prometheus.Gauge
	NumberOfVerifiedInsufficientConfDelegations prometheus.Gauge
	NumberOfVerifiedSufficientConfDelegations   prometheus.Gauge
	MethodExecutionLatency                      *prometheus.HistogramVec
	UnbondingCensorshipGaugeVec                 *prometheus.GaugeVec
	InclusionProofCensorshipGaugeVec            *prometheus.GaugeVec
	BestBlockHeight                             prometheus.Gauge
	BtcLightClientHeight                        prometheus.Gauge
}

func newUnbondingWatcherMetrics(registry *prometheus.Registry) *UnbondingWatcherMetrics {
	registerer := promauto.With(registry)

	uwMetrics := &UnbondingWatcherMetrics{
		Registry: registry,
		ReportedUnbondingTransactionsCounter: registerer.NewCounter(prometheus.CounterOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_reported_unbonding_transactions",
			Help:      "The total number of unbonding transactions successfully reported to Babylon node",
		}),
		FailedReportedUnbondingTransactions: registerer.NewCounter(prometheus.CounterOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_failed_reported_unbonding_transactions",
			Help:      "The total number times reporting unbonding transactions to Babylon node failed",
		}),
		NumberOfTrackedActiveDelegations: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_tracked_active_delegations",
			Help:      "The number of active delegations tracked by unbonding watcher",
		}),
		DetectedUnbondedStakeExpansionCounter: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_detected_unbonded_stake_expansion",
			Help:      "The number of spend staking transaction that are expanding their stake",
		}),
		DetectedUnbondingTransactionsCounter: registerer.NewCounter(prometheus.CounterOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_detected_unbonding_transactions",
			Help:      "The total number of unbonding transactions detected by unbonding watcher",
		}),
		DetectedNonUnbondingTransactionsCounter: registerer.NewCounter(prometheus.CounterOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_detected_non_unbonding_transactions",
			Help:      "The total number of non unbonding (slashing or withdrawal) transactions detected by unbonding watcher",
		}),
		FailedReportedActivateDelegations: registerer.NewCounter(prometheus.CounterOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_failed_reported_activate_delegation",
			Help:      "The total number times reporting activation delegation failed on Babylon node",
		}),
		ReportedActivateDelegationsCounter: registerer.NewCounter(prometheus.CounterOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_reported_activate_delegations",
			Help:      "The total number of unbonding transactions successfully reported to Babylon node",
		}),
		MethodExecutionLatency: registerer.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_method_latency_seconds",
			Help:      "Latency in seconds",
			Buckets:   []float64{0.3, 1, 2, 5, 10, 30, 60, 120, 180, 300},
		}, []string{"method"}),
		NumberOfActivationInProgress: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_number_of_activation_in_progress",
			Help:      "The number of activations in progress",
		}),
		NumberOfVerifiedDelegations: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_number_of_verified_delegations",
			Help:      "The number of verified delegations",
		}),
		NumberOfVerifiedNotInChainDelegations: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_number_of_verified_not_in_chain_delegations",
			Help:      "The number of verified not in chain delegations",
		}),
		NumberOfVerifiedInsufficientConfDelegations: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_number_of_verified_insufficient_conf_delegations",
			Help:      "The number of verified delegations with insufficient confirmations",
		}),
		NumberOfVerifiedSufficientConfDelegations: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_number_of_verified_sufficient_conf_delegations",
			Help:      "The number of verified delegations with sufficient confirmations",
		}),
		UnbondingCensorshipGaugeVec: registerer.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "vigilante",
				Name:      "unbonding_watcher_unbonding_censorship",
				Help:      "The metric of a new unbonding censorship",
			},
			[]string{
				"staking_tx_hash",
			},
		),
		InclusionProofCensorshipGaugeVec: registerer.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "vigilante",
				Name:      "unbonding_watcher_inclusion_proof_censorship",
				Help:      "The metric of a new inclusion proof censorship",
			},
			[]string{
				"staking_tx_hash",
			},
		),
		BestBlockHeight: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_best_block_height",
			Help:      "The height of the best btc block",
		}),
		BtcLightClientHeight: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "unbonding_watcher_btc_light_client_height",
			Help:      "The height of the babylon btc light client",
		}),
	}

	return uwMetrics
}

type SlasherMetrics struct {
	SlashedDelegationGaugeVec       *prometheus.GaugeVec
	SlashedFinalityProvidersCounter prometheus.Counter
	SlashedDelegationsCounter       prometheus.Counter
	SlashedSatsCounter              prometheus.Counter
}

func newSlasherMetrics(registry *prometheus.Registry) *SlasherMetrics {
	registerer := promauto.With(registry)

	metrics := &SlasherMetrics{
		SlashedFinalityProvidersCounter: registerer.NewCounter(prometheus.CounterOpts{
			Name: "slasher_slashed_finality_providers",
			Help: "The number of slashed finality providers",
		}),
		SlashedDelegationsCounter: registerer.NewCounter(prometheus.CounterOpts{
			Name: "slasher_slashed_delegations",
			Help: "The number of slashed delegations",
		}),
		SlashedSatsCounter: registerer.NewCounter(prometheus.CounterOpts{
			Name: "slasher_slashed_sats",
			Help: "The amount of slashed funds in Satoshi",
		}),
		SlashedDelegationGaugeVec: registerer.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "slasher_new_slashed_delegation",
				Help: "The metric of a newly slashed delegation",
			},
			[]string{
				// del_btc_pk is the Bitcoin secp256k1 PK of this BTC delegation in hex string
				"del_btc_pk",
				// fp_btc_pk is the Bitcoin secp256k1 PK of the finality provider that
				// this BTC delegation delegates to, in hex string
				"fp_btc_pk",
			},
		),
	}

	return metrics
}

func (sm *SlasherMetrics) RecordSlashedDelegation(del *types.BTCDelegationResponse) {
	// refresh time of the slashed delegation gauge for each (fp, del) pair
	for _, pk := range del.FpBtcPkList {
		sm.SlashedDelegationGaugeVec.WithLabelValues(
			del.BtcPk.MarshalHex(),
			pk.MarshalHex(),
		).SetToCurrentTime()
	}

	// increment slashed Satoshis and slashed delegations
	sm.SlashedSatsCounter.Add(float64(del.TotalSat))
	sm.SlashedDelegationsCounter.Inc()
}

type AtomicSlasherMetrics struct {
	Registry                            *prometheus.Registry
	TrackedBTCDelegationsGauge          prometheus.Gauge
	SelectiveSlashingCensorshipGaugeVec *prometheus.GaugeVec
}

func newAtomicSlasherMetrics(registry *prometheus.Registry) *AtomicSlasherMetrics {
	registerer := promauto.With(registry)

	asMetrics := &AtomicSlasherMetrics{
		Registry: registry,
		TrackedBTCDelegationsGauge: registerer.NewGauge(prometheus.GaugeOpts{
			Namespace: "vigilante",
			Name:      "atomic_slasher_tracked_delegations_gauge",
			Help:      "The number of BTC delegations the atomic slasher routine is tracking",
		}),
		SelectiveSlashingCensorshipGaugeVec: registerer.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "vigilante",
				Name:      "atomic_slasher_selective_slashing_censorship",
				Help:      "The metric of a new selective slashing censorship",
			},
			[]string{
				"staking_tx_hash",
			},
		),
	}

	return asMetrics
}
