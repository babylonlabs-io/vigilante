package btcslasher

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/v3/btcstaking"
	bbn "github.com/babylonlabs-io/babylon/v3/types"
	bstypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
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

	ckptParams := checkPointParams.Params
	pkScript := slashingMsgTxWithWitness.TxOut[del.StakingOutputIdx].PkScript
	if err := bs.waitForTxKDeep(ctx, txHash, pkScript, ckptParams.BtcConfirmationDepth); err != nil {
		return nil, err
	}

	return txHash, nil
}

// BuildUnbondingSlashingTxWithWitness returns the unbonding slashing tx.
func BuildUnbondingSlashingTxWithWitness(
	d *bstypes.BTCDelegationResponse,
	bsParams *bstypes.Params,
	btcNet *chaincfg.Params,
	fpSK *btcec.PrivateKey,
) (*wire.MsgTx, error) {
	unbondingMsgTx, _, err := bbn.NewBTCTxFromHex(d.UndelegationResponse.UnbondingTxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to convert a Babylon unbonding tx to wire.MsgTx: %w", err)
	}

	fpBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(d.FpBtcPkList)
	if err != nil {
		return nil, fmt.Errorf("failed to convert finality provider pks to BTC pks: %w", err)
	}

	covenantBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(bsParams.CovenantPks)
	if err != nil {
		return nil, fmt.Errorf("failed to convert covenant pks to BTC pks: %w", err)
	}

	if d.UnbondingTime > uint32(^uint16(0)) {
		panic(fmt.Errorf("unbondingTime (%d) exceeds maximum for uint16", d.UnbondingTime))
	}

	// get unbonding info
	unbondingInfo, err := btcstaking.BuildUnbondingInfo(
		d.BtcPk.MustToBTCPK(),
		fpBtcPkList,
		covenantBtcPkList,
		bsParams.CovenantQuorum,
		uint16(d.UnbondingTime),
		btcutil.Amount(unbondingMsgTx.TxOut[0].Value),
		btcNet,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create BTC unbonding info: %w", err)
	}
	slashingSpendInfo, err := unbondingInfo.SlashingPathSpendInfo()
	if err != nil {
		return nil, fmt.Errorf("could not get unbonding slashing spend info: %w", err)
	}

	// get the list of covenant signatures encrypted by the given finality provider's PK
	fpPK := fpSK.PubKey()
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpPK)
	fpIdx, err := findFPIdxInWitness(fpBTCPK, d.FpBtcPkList)
	if err != nil {
		return nil, err
	}

	covAdaptorSigs, err := bstypes.GetOrderedCovenantSignatures(fpIdx, d.UndelegationResponse.CovenantSlashingSigs, bsParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get ordered covenant adaptor signatures: %w", err)
	}

	delSlashingSig, err := bbn.NewBIP340SignatureFromHex(d.UndelegationResponse.DelegatorSlashingSigHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Delegator slashing signature: %w", err)
	}

	slashTx, err := bstypes.NewBTCSlashingTxFromHex(d.UndelegationResponse.SlashingTxHex)
	if err != nil {
		return nil, err
	}

	// assemble witness for unbonding slashing tx
	slashingMsgTxWithWitness, err := slashTx.BuildSlashingTxWithWitness(
		fpSK,
		d.FpBtcPkList,
		unbondingMsgTx,
		0,
		delSlashingSig,
		covAdaptorSigs,
		bsParams.CovenantQuorum,
		slashingSpendInfo,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to build witness for unbonding BTC delegation %s under finality provider %s: %w",
			d.BtcPk.MarshalHex(),
			bbn.NewBIP340PubKeyFromBTCPK(fpSK.PubKey()).MarshalHex(),
			err,
		)
	}

	return slashingMsgTxWithWitness, nil
}

// findFPIdxInWitness returns the index of the given finality provider
// among all restaked finality providers
func findFPIdxInWitness(fpBTCPK *bbn.BIP340PubKey, fpBtcPkList []bbn.BIP340PubKey) (int, error) {
	sortedFPBTCPKList := bbn.SortBIP340PKs(fpBtcPkList)
	for i, pk := range sortedFPBTCPKList {
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
	stakingMsgTx, _, err := bbn.NewBTCTxFromHex(d.StakingTxHex)
	if err != nil {
		return nil, fmt.Errorf("failed to convert a Babylon staking tx to wire.MsgTx: %w", err)
	}

	fpBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(d.FpBtcPkList)
	if err != nil {
		return nil, fmt.Errorf("failed to convert finality provider pks to BTC pks: %w", err)
	}

	covenantBtcPkList, err := bbn.NewBTCPKsFromBIP340PKs(bsParams.CovenantPks)
	if err != nil {
		return nil, fmt.Errorf("failed to convert covenant pks to BTC pks: %w", err)
	}

	if d.TotalSat > math.MaxInt64 {
		panic(fmt.Errorf("TotalSat %d exceeds int64 range", d.TotalSat))
	}

	// get staking info
	// #nosec G115 -- performed the conversion check above
	stakingInfo, err := btcstaking.BuildStakingInfo(
		d.BtcPk.MustToBTCPK(),
		fpBtcPkList,
		covenantBtcPkList,
		bsParams.CovenantQuorum,
		uint16(d.EndHeight-d.StartHeight),
		btcutil.Amount(d.TotalSat),
		btcNet,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create BTC staking info: %w", err)
	}
	slashingSpendInfo, err := stakingInfo.SlashingPathSpendInfo()
	if err != nil {
		return nil, fmt.Errorf("could not get slashing spend info: %w", err)
	}

	// get the list of covenant signatures encrypted by the given finality provider's PK
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpSK.PubKey())
	sortedFPBTCPKList := bbn.SortBIP340PKs(d.FpBtcPkList)
	fpIdx, err := findFPIdxInWitness(fpBTCPK, sortedFPBTCPKList)
	if err != nil {
		return nil, err
	}

	covAdaptorSigs, err := bstypes.GetOrderedCovenantSignatures(fpIdx, d.CovenantSigs, bsParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get ordered covenant adaptor signatures: %w", err)
	}

	delSigSlash, err := bbn.NewBIP340SignatureFromHex(d.DelegatorSlashSigHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Delegator slashing signature: %w", err)
	}

	slashTx, err := bstypes.NewBTCSlashingTxFromHex(d.SlashingTxHex)
	if err != nil {
		return nil, err
	}

	// assemble witness for slashing tx
	slashingMsgTxWithWitness, err := slashTx.BuildSlashingTxWithWitness(
		fpSK,
		sortedFPBTCPKList,
		stakingMsgTx,
		d.StakingOutputIdx,
		delSigSlash,
		covAdaptorSigs,
		bsParams.CovenantQuorum,
		slashingSpendInfo,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to build witness for BTC delegation of %s under finality provider %s: %w",
			d.BtcPk.MarshalHex(),
			bbn.NewBIP340PubKeyFromBTCPK(fpSK.PubKey()).MarshalHex(),
			err,
		)
	}

	return slashingMsgTxWithWitness, nil
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
		retry.Attempts(0), // inf retries, but cancelled with ctx
		retry.Delay(bs.retrySleepTime),
		retry.MaxDelay(bs.maxRetrySleepTime),
		retry.LastErrorOnly(true),
	)
}
