package btcslasher

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/v4/btcstaking"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	bstypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/utils"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/hashicorp/go-multierror"
)

const (
	defaultPaginationLimit = 100
	maxBurnAmountBTC       = float64(50000)
)

var ErrNotSlashable = errors.New("delegation is not slashable")

// StakingInfoProvider interface for types that can provide slashing path spend info
type StakingInfoProvider interface {
	SlashingPathSpendInfo() (*btcstaking.SpendInfo, error)
}

// SlashingConfig holds configuration for building slashing transactions
type SlashingConfig struct {
	TxHex          string
	SlashingTxHex  string
	SlashingSigHex string
	CovenantSigs   []*bstypes.CovenantAdaptorSignatures
	OutputIdx      uint32
	InfoBuilder    func() (StakingInfoProvider, error)
}

type SlashResult struct {
	Del            *bstypes.BTCDelegationResponse
	SlashingTxHash *chainhash.Hash
	Err            error
}

func (bs *BTCSlasher) slashBTCDelegation(
	fpBTCPK *bbn.BIP340PubKey,
	extractedfpBTCSK *btcec.PrivateKey,
	del *bstypes.BTCDelegationResponse,
) {
	var txHash *chainhash.Hash
	ctx, cancel := bs.quitContext()
	defer cancel()

	err := retry.Do(func() error {
		innerCtx, innerCancel := context.WithCancel(ctx)
		defer innerCancel()
		errChan := make(chan error, 2)
		txHashChan := make(chan *chainhash.Hash, 1)

		attemptSlashingTx := func(isUnbonding bool) {
			tx, err := bs.sendSlashingTx(innerCtx, fpBTCPK, extractedfpBTCSK, del, isUnbonding)
			if err == nil {
				select {
				case txHashChan <- tx:
				default:
				}
			} else {
				errChan <- err
			}
		}

		go attemptSlashingTx(false)
		go attemptSlashingTx(true)

		var accumulatedErr error
		select {
		case txHash = <-txHashChan:
			// One of the transactions succeeded, cancel remaining attempts
			innerCancel()
		case <-ctx.Done():
			// If context is canceled externally, stop everything
			accumulatedErr = ctx.Err()
		case err1 := <-errChan:
			// First failure, wait for another
			select {
			case err2 := <-errChan:
				accumulatedErr = multierror.Append(err1, err2)

				// Check if both errors are not slashable
				if errors.Is(err1, ErrNotSlashable) && errors.Is(err2, ErrNotSlashable) {
					bs.logger.Info("Both staking and unbonding transactions are not slashable, skipping",
						"delegation", del.BtcPk.MarshalHex(),
						"finality_provider", fpBTCPK.MarshalHex(),
					)
					accumulatedErr = nil
				}
			case txHash = <-txHashChan:
				// Second transaction succeeded, ignore the first error
				innerCancel()
			}
		}

		// We ignore context canceled errors
		if errors.Is(accumulatedErr, context.Canceled) {
			accumulatedErr = nil
		}

		return accumulatedErr
	},
		retry.Context(ctx),
		retry.Delay(bs.retrySleepTime),
		retry.MaxDelay(bs.maxRetrySleepTime),
		retry.Attempts(0), // inf retries, we exit via context, tx included in chain, or both unspendable
		retry.OnRetry(func(n uint, err error) {
			bs.logger.Warnf(
				"Failed to slash BTC delegation %s under finality provider %s, attempt %d: %v",
				del.BtcPk.MarshalHex(),
				fpBTCPK.MarshalHex(),
				n+1,
				err,
			)
		}),
	)

	slashRes := &SlashResult{
		Del:            del,
		SlashingTxHash: txHash,
		Err:            err,
	}
	utils.PushOrQuit[*SlashResult](bs.slashResultChan, slashRes, bs.quit)
}

func (bs *BTCSlasher) sendSlashingTx(
	ctx context.Context,
	fpBTCPK *bbn.BIP340PubKey,
	extractedfpBTCSK *btcec.PrivateKey,
	del *bstypes.BTCDelegationResponse,
	isUnbondingSlashingTx bool,
) (*chainhash.Hash, error) {
	var (
		err     error
		slashTx *bstypes.BTCSlashingTx
	)
	// check if the slashing tx is known on Bitcoin
	if isUnbondingSlashingTx {
		slashTx, err = bstypes.NewBTCSlashingTxFromHex(del.UndelegationResponse.SlashingTxHex)
		if err != nil {
			return nil, err
		}
	} else {
		slashTx, err = bstypes.NewBTCSlashingTxFromHex(del.SlashingTxHex)
		if err != nil {
			return nil, err
		}
	}

	txHash := slashTx.MustGetTxHash()
	if bs.isTxSubmittedToBitcoin(txHash) {
		// already submitted to Bitcoin, skip
		return txHash, nil
	}

	// check if the staking/unbonding tx's output is indeed spendable
	// TODO: use bbn.GetOutputIdxInBTCTx
	var spendable bool
	if isUnbondingSlashingTx {
		ubondingTx, errDecode := hex.DecodeString(del.UndelegationResponse.UnbondingTxHex)
		if errDecode != nil {
			return nil, errDecode
		}
		spendable, err = bs.isTaprootOutputSpendable(ubondingTx, 0)
	} else {
		stakingTx, errDecode := hex.DecodeString(del.StakingTxHex)
		if errDecode != nil {
			return nil, errDecode
		}
		spendable, err = bs.isTaprootOutputSpendable(stakingTx, del.StakingOutputIdx)
	}
	if err != nil {
		// Warning: this can only be an error in Bitcoin side
		return nil, fmt.Errorf(
			"failed to check if BTC delegation %s under finality provider %s is slashable: %w",
			del.BtcPk.MarshalHex(),
			fpBTCPK.MarshalHex(),
			err,
		)
	}
	// this staking/unbonding tx is no longer slashable on Bitcoin
	if !spendable {
		return nil, fmt.Errorf(
			"the staking/unbonding tx of BTC delegation %s under finality provider %s is not slashable: %w",
			del.BtcPk.MarshalHex(),
			fpBTCPK.MarshalHex(),
			ErrNotSlashable,
		)
	}

	// get BTC staking parameter with the version in this BTC delegation
	bsParamsResp, err := bs.BBNQuerier.BTCStakingParamsByVersion(del.ParamsVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get BTC staking parameter at version %d", del.ParamsVersion)
	}
	bsParams := &bsParamsResp.Params

	// assemble witness for unbonding slashing tx
	var slashingMsgTxWithWitness *wire.MsgTx
	bs.mu.Lock()
	if isUnbondingSlashingTx {
		slashingMsgTxWithWitness, err = BuildUnbondingSlashingTxWithWitness(del, bsParams, bs.netParams, extractedfpBTCSK)
	} else {
		slashingMsgTxWithWitness, err = BuildSlashingTxWithWitness(del, bsParams, bs.netParams, extractedfpBTCSK)
	}
	bs.mu.Unlock()
	if err != nil {
		// Warning: this can only be a programming error in Babylon side
		return nil, fmt.Errorf(
			"failed to build witness for BTC delegation %s under finality provider %s: %w",
			del.BtcPk.MarshalHex(),
			fpBTCPK.MarshalHex(),
			err,
		)
	}
	bs.logger.Debugf(
		"signed and assembled witness for slashing tx of unbonded BTC delegation %s under finality provider %s",
		del.BtcPk.MarshalHex(),
		fpBTCPK.MarshalHex(),
	)

	// submit slashing tx
	txHash, err = bs.BTCClient.SendRawTransactionWithBurnLimit(slashingMsgTxWithWitness, true, maxBurnAmountBTC)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to submit slashing tx of BTC delegation %s under finality provider %s to Bitcoin: %w",
			del.BtcPk.MarshalHex(),
			fpBTCPK.MarshalHex(),
			err,
		)
	}
	bs.logger.Infof(
		"successfully submitted slashing tx (txHash: %s) for BTC delegation %s under finality provider %s",
		txHash.String(),
		del.BtcPk.MarshalHex(),
		fpBTCPK.MarshalHex(),
	)

	checkPointParams, err := bs.BBNQuerier.BTCCheckpointParams()
	if err != nil {
		return nil, fmt.Errorf("failed to get BTC staking parameter at version %d", del.ParamsVersion)
	}

	// Validate staking output index is within bounds
	if int(del.StakingOutputIdx) >= len(slashingMsgTxWithWitness.TxOut) {
		return nil, fmt.Errorf("staking output index %d out of bounds for slashing tx %s with %d outputs",
			del.StakingOutputIdx, txHash.String(), len(slashingMsgTxWithWitness.TxOut))
	}

	ckptParams := checkPointParams.Params
	pkScript := slashingMsgTxWithWitness.TxOut[del.StakingOutputIdx].PkScript
	if err := bs.waitForTxKDeep(ctx, txHash, pkScript, ckptParams.BtcConfirmationDepth); err != nil {
		return nil, err
	}

	return txHash, nil
}

// buildSlashingTxWithWitness is the unified function for building slashing transactions
func buildSlashingTxWithWitness(
	d *bstypes.BTCDelegationResponse,
	bsParams *bstypes.Params,
	fpSK *btcec.PrivateKey,
	config SlashingConfig,
) (*wire.MsgTx, error) {
	msgTx, _, err := bbn.NewBTCTxFromHex(config.TxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to convert tx to wire.MsgTx: %w", err)
	}

	// Build info using the provided builder
	stakingInfo, err := config.InfoBuilder()
	if err != nil {
		return nil, fmt.Errorf("could not create staking info: %w", err)
	}

	// Get slashing spend info
	slashingSpendInfo, err := stakingInfo.SlashingPathSpendInfo()
	if err != nil {
		return nil, fmt.Errorf("could not get slashing spend info: %w", err)
	}

	// Find finality provider index
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpSK.PubKey())
	fpIdx, err := findFPIdxForCovenantSignatures(fpBTCPK, d.FpBtcPkList)
	if err != nil {
		return nil, err
	}

	// Get ordered covenant signatures
	covAdaptorSigs, err := bstypes.GetOrderedCovenantSignatures(fpIdx, config.CovenantSigs, bsParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get ordered covenant adaptor signatures: %w", err)
	}

	// Parse delegator slashing signature
	delSlashingSig, err := bbn.NewBIP340SignatureFromHex(config.SlashingSigHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse delegator slashing signature: %w", err)
	}

	slashTx, err := bstypes.NewBTCSlashingTxFromHex(config.SlashingTxHex)
	if err != nil {
		return nil, err
	}

	slashingMsgTxWithWitness, err := slashTx.BuildSlashingTxWithWitness(
		fpSK,
		d.FpBtcPkList,
		msgTx,
		config.OutputIdx,
		delSlashingSig,
		covAdaptorSigs,
		bsParams.CovenantQuorum,
		slashingSpendInfo,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to build witness for BTC delegation %s under finality provider %s: %w",
			d.BtcPk.MarshalHex(),
			bbn.NewBIP340PubKeyFromBTCPK(fpSK.PubKey()).MarshalHex(),
			err,
		)
	}

	return slashingMsgTxWithWitness, nil
}

// BuildUnbondingSlashingTxWithWitness returns the unbonding slashing tx.
func BuildUnbondingSlashingTxWithWitness(
	d *bstypes.BTCDelegationResponse,
	bsParams *bstypes.Params,
	btcNet *chaincfg.Params,
	fpSK *btcec.PrivateKey,
) (*wire.MsgTx, error) {
	if d.UnbondingTime > uint32(^uint16(0)) {
		panic(fmt.Errorf("unbondingTime (%d) exceeds maximum for uint16", d.UnbondingTime))
	}

	config := SlashingConfig{
		TxHex:          d.UndelegationResponse.UnbondingTxHex,
		SlashingTxHex:  d.UndelegationResponse.SlashingTxHex,
		SlashingSigHex: d.UndelegationResponse.DelegatorSlashingSigHex,
		CovenantSigs:   d.UndelegationResponse.CovenantSlashingSigs,
		OutputIdx:      0,
		InfoBuilder: func() (StakingInfoProvider, error) {
			unbondingMsgTx, _, err := bbn.NewBTCTxFromHex(d.UndelegationResponse.UnbondingTxHex)
			if err != nil {
				return nil, err
			}

			fpBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(d.FpBtcPkList)
			if err != nil {
				return nil, err
			}

			covenantBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(bsParams.CovenantPks)
			if err != nil {
				return nil, err
			}

			// #nosec G115 -- performed the conversion check above
			return btcstaking.BuildUnbondingInfo(
				d.BtcPk.MustToBTCPK(),
				fpBtcPkList,
				covenantBtcPkList,
				bsParams.CovenantQuorum,
				uint16(d.UnbondingTime),
				btcutil.Amount(unbondingMsgTx.TxOut[0].Value),
				btcNet,
			)
		},
	}

	return buildSlashingTxWithWitness(d, bsParams, fpSK, config)
}

// findFPIdxForCovenantSignatures returns the index of the given finality provider
// among all restaked finality providers
func findFPIdxForCovenantSignatures(fpBTCPK *bbn.BIP340PubKey, fpBtcPkList []bbn.BIP340PubKey) (int, error) {
	for i, pk := range fpBtcPkList {
		if pk.Equals(fpBTCPK) {
			return i, nil
		}
	}

	return 0, fmt.Errorf("the given finality provider's PK is not found in the BTC delegation")
}

// BuildSlashingTxWithWitness constructs a Bitcoin slashing transaction with the required witness data
// using the provided finality provider's private key. It handles the conversion and validation of
// various parameters needed for slashing a Bitcoin delegation, including the staking transaction,
// finality provider public keys, and covenant public keys.
// Note: this function is UNSAFE for concurrent accesses as slashTx.BuildSlashingTxWithWitness is not safe for
// concurrent access inside it's calling  asig.NewDecyptionKeyFromBTCSK
func BuildSlashingTxWithWitness(
	d *bstypes.BTCDelegationResponse,
	bsParams *bstypes.Params,
	btcNet *chaincfg.Params,
	fpSK *btcec.PrivateKey,
) (*wire.MsgTx, error) {
	if d.TotalSat > math.MaxInt64 {
		panic(fmt.Errorf("TotalSat %d exceeds int64 range", d.TotalSat))
	}

	config := SlashingConfig{
		TxHex:          d.StakingTxHex,
		SlashingTxHex:  d.SlashingTxHex,
		SlashingSigHex: d.DelegatorSlashSigHex,
		CovenantSigs:   d.CovenantSigs,
		OutputIdx:      d.StakingOutputIdx,
		InfoBuilder: func() (StakingInfoProvider, error) {
			fpBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(d.FpBtcPkList)
			if err != nil {
				return nil, err
			}

			covenantBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(bsParams.CovenantPks)
			if err != nil {
				return nil, err
			}

			// #nosec G115 -- performed the conversion check above
			return btcstaking.BuildStakingInfo(
				d.BtcPk.MustToBTCPK(),
				fpBtcPkList,
				covenantBtcPkList,
				bsParams.CovenantQuorum,
				uint16(d.EndHeight-d.StartHeight),
				btcutil.Amount(d.TotalSat),
				btcNet,
			)
		},
	}

	return buildSlashingTxWithWitness(d, bsParams, fpSK, config)
}

// BTC slasher will try to slash via staking path for active BTC delegations,
// and slash via unbonding path for unbonded delegations.
//
// An unbonded BTC delegation in Babylon's view might still
// have an non-expired timelock in unbonding tx.
func (bs *BTCSlasher) getAllActiveAndUnbondedBTCDelegations(
	fpBTCPK *bbn.BIP340PubKey,
) ([]*bstypes.BTCDelegationResponse, []*bstypes.BTCDelegationResponse, error) {
	activeDels, unbondedDels := make([]*bstypes.BTCDelegationResponse, 0), make([]*bstypes.BTCDelegationResponse, 0)

	// a map where key is the parameter version and value is the parameters
	paramsMap := make(map[uint32]*bstypes.Params)

	// get all active BTC delegations
	pagination := query.PageRequest{Limit: defaultPaginationLimit}
	for {
		resp, err := bs.BBNQuerier.FinalityProviderDelegations(fpBTCPK.MarshalHex(), &pagination)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get BTC delegations under finality provider %s: %w", fpBTCPK.MarshalHex(), err)
		}
		for _, dels := range resp.BtcDelegatorDelegations {
			for i, del := range dels.Dels {
				// get the BTC staking parameter at that version
				bsParams, ok := paramsMap[del.ParamsVersion]
				if !ok {
					bsParamsResp, err := bs.BBNQuerier.BTCStakingParamsByVersion(del.ParamsVersion)
					if err != nil {
						return nil, nil, fmt.Errorf("failed to get BTC staking parameter at version %d", del.ParamsVersion)
					}
					bsParams = &bsParamsResp.Params
				}

				// filter out all active and unbonded BTC delegations
				// NOTE: slasher does not slash BTC delegations who
				//   - is expired in Babylon due to the timelock of <w rest blocks, OR
				//   - has an expired timelock but the delegator hasn't moved its stake yet
				// This is because such BTC delegations do not have voting power thus do not
				// affect Babylon's consensus.
				if del.Active {
					// avoid using del which changes over the iterations
					activeDels = append(activeDels, dels.Dels[i])
				}
				if strings.EqualFold(del.StatusDesc, bstypes.BTCDelegationStatus_UNBONDED.String()) &&
					len(del.UndelegationResponse.CovenantSlashingSigs) >= int(bsParams.CovenantQuorum) {
					// NOTE: Babylon considers a BTC delegation to be unbonded once it
					// receives staker signature for unbonding transaction, no matter
					// whether the unbonding tx's timelock has expired. In monitor's view we need to try to slash every
					// BTC delegation with a non-nil BTC undelegation and with jury/delegator signature on slashing tx
					// avoid using del which changes over the iterations
					unbondedDels = append(unbondedDels, dels.Dels[i])
				}
			}
		}
		if resp.Pagination == nil || resp.Pagination.NextKey == nil {
			break
		}
		pagination.Key = resp.Pagination.NextKey
	}

	return activeDels, unbondedDels, nil
}

func (bs *BTCSlasher) waitForTxKDeep(ctx context.Context, txHash *chainhash.Hash, pkScript []byte, k uint32) error {
	return retry.Do(func() error {
		details, status, err := bs.BTCClient.TxDetails(txHash, pkScript)
		if err != nil {
			return fmt.Errorf("failed to get tx details: %w", err)
		}

		if status != btcclient.TxInChain {
			return fmt.Errorf("tx not in chain: %v", status)
		}

		tip, err := bs.BTCClient.GetBestBlock()
		if err != nil {
			return fmt.Errorf("failed to get best block: %w", err)
		}

		if tip-details.BlockHeight >= k {
			return nil
		}

		return fmt.Errorf("tx: %s, not deep enough: %d/%d", txHash.String(), tip-details.BlockHeight, k)
	},
		retry.Context(ctx),
		retry.Attempts(0), // inf retries, but canceled with ctx
		retry.Delay(bs.retrySleepTime),
		retry.MaxDelay(bs.maxRetrySleepTime),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			bs.logger.Warnf(
				"Waiting for BTC slashing tx %s to be %d blocks deep, attempt %d: %v",
				txHash.String(),
				k,
				n+1,
				err,
			)
		}),
	)
}
