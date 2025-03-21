package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/babylonlabs-io/babylon/app"
	btccheckpointtypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclightclienttypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	checkpointingtypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	epochingtypes "github.com/babylonlabs-io/babylon/x/epoching/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
)

type GenesisInfo struct {
	baseBTCHeight uint32
	epochInterval uint64
	checkpointTag string
	valSet        checkpointingtypes.ValidatorWithBlsKeySet
}

func NewGenesisInfo(
	baseBTCHeight uint32,
	epochInterval uint64,
	checkpointTag string,
	valSet *checkpointingtypes.ValidatorWithBlsKeySet,
) *GenesisInfo {
	return &GenesisInfo{
		baseBTCHeight: baseBTCHeight,
		epochInterval: epochInterval,
		checkpointTag: checkpointTag,
		valSet:        *valSet,
	}
}

// GetGenesisInfoFromFile reads genesis info from the provided genesis file
func GetGenesisInfoFromFile(filePath string) (*GenesisInfo, error) {
	var (
		baseBTCHeight uint32
		epochInterval uint64
		checkpointTag string
		valSet        checkpointingtypes.ValidatorWithBlsKeySet
		err           error
	)

	appState, _, err := genutiltypes.GenesisStateFromGenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read genesis file %v, %w", filePath, err)
	}

	tmpBabylon := app.NewTmpBabylonApp()
	gentxModule, ok := tmpBabylon.BasicModuleManager[genutiltypes.ModuleName].(genutil.AppModuleBasic)
	if !ok {
		return nil, fmt.Errorf("unexpected message type: %T", gentxModule)
	}

	checkpointingGenState := checkpointingtypes.GetGenesisStateFromAppState(tmpBabylon.AppCodec(), appState)
	err1 := checkpointingGenState.Validate()
	err2 := validateGenesisKeys(checkpointingGenState.GenesisKeys)
	if err1 != nil && err2 != nil {
		return nil, fmt.Errorf("invalid checkpointing genesis %w ", errors.Join(err1, err2))
	}

	if err1 != nil && !errors.Is(err1, checkpointingtypes.ErrInvalidPoP) {
		return nil, fmt.Errorf("invalid checkpointing genesis %w", err1)
	}

	if err2 != nil && !errors.Is(err2, checkpointingtypes.ErrInvalidPoP) {
		return nil, fmt.Errorf("invalid checkpointing genesis %w", err2)
	}

	genutilGenState := genutiltypes.GetGenesisStateFromAppState(tmpBabylon.AppCodec(), appState)
	gentxs := genutilGenState.GenTxs

	valSet.ValSet = make([]*checkpointingtypes.ValidatorWithBlsKey, 0)
	for _, tx := range gentxs {
		var validatorFunc func(msgs []sdk.Msg) error
		if err1 != nil {
			validatorFunc = messageValidatorCreateValidator
		} else {
			validatorFunc = gentxModule.GenTxValidator
		}
		tx, err := genutiltypes.ValidateAndGetGenTx(tx, tmpBabylon.TxConfig().TxJSONDecoder(), validatorFunc)
		if err != nil {
			return nil, fmt.Errorf("invalid genesis tx %w", err)
		}
		msgs := tx.GetMsgs()
		if len(msgs) == 0 {
			return nil, errors.New("invalid genesis transaction")
		}

		var msgCreateValidator *types.MsgCreateValidator
		if err1 != nil {
			msgCreateValidator, ok = msgs[0].(*types.MsgCreateValidator)
			if !ok {
				return nil, fmt.Errorf("unexpected message type: %T", msgs[0])
			}
		} else {
			wrappedCreateValidator, ok := msgs[0].(*checkpointingtypes.MsgWrappedCreateValidator)
			if !ok {
				return nil, fmt.Errorf("unexpected message type: %T", msgs[0])
			}

			msgCreateValidator = wrappedCreateValidator.MsgCreateValidator
		}

		power := sdk.TokensToConsensusPower(msgCreateValidator.Value.Amount, sdk.DefaultPowerReduction)
		if power < 0 {
			return nil, fmt.Errorf("consensus power calculation returned a negative value")
		}
		keyWithPower := &checkpointingtypes.ValidatorWithBlsKey{
			ValidatorAddress: msgCreateValidator.ValidatorAddress,
			BlsPubKey:        msgCreateValidator.Pubkey.Value,
			VotingPower:      uint64(power),
		}
		valSet.ValSet = append(valSet.ValSet, keyWithPower)
	}

	btclightclientGenState := GetBtclightclientGenesisStateFromAppState(tmpBabylon.AppCodec(), appState)
	err = btclightclientGenState.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid btclightclient genesis %w", err)
	}
	baseBTCHeight = btclightclientGenState.BtcHeaders[0].Height

	epochingGenState := GetEpochingGenesisStateFromAppState(tmpBabylon.AppCodec(), appState)
	err = epochingGenState.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid epoching genesis %w", err)
	}
	epochInterval = epochingGenState.Params.EpochInterval

	btccheckpointGenState := GetBtccheckpointGenesisStateFromAppState(tmpBabylon.AppCodec(), appState)
	err = btccheckpointGenState.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid btccheckpoint genesis %w", err)
	}
	checkpointTag = btccheckpointGenState.Params.CheckpointTag

	genesisInfo := &GenesisInfo{
		baseBTCHeight: baseBTCHeight,
		epochInterval: epochInterval,
		checkpointTag: checkpointTag,
		valSet:        valSet,
	}

	return genesisInfo, nil
}

// GetBtclightclientGenesisStateFromAppState returns x/btclightclient GenesisState given raw application
// genesis state.
func GetBtclightclientGenesisStateFromAppState(cdc codec.Codec, appState map[string]json.RawMessage) btclightclienttypes.GenesisState {
	var genesisState btclightclienttypes.GenesisState

	if appState[btclightclienttypes.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[btclightclienttypes.ModuleName], &genesisState)
	}

	return genesisState
}

// GetEpochingGenesisStateFromAppState returns x/epoching GenesisState given raw application
// genesis state.
func GetEpochingGenesisStateFromAppState(cdc codec.Codec, appState map[string]json.RawMessage) epochingtypes.GenesisState {
	var genesisState epochingtypes.GenesisState

	if appState[epochingtypes.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[epochingtypes.ModuleName], &genesisState)
	}

	return genesisState
}

// GetBtccheckpointGenesisStateFromAppState returns x/btccheckpoint GenesisState given raw application
// genesis state.
func GetBtccheckpointGenesisStateFromAppState(cdc codec.Codec, appState map[string]json.RawMessage) btccheckpointtypes.GenesisState {
	var genesisState btccheckpointtypes.GenesisState

	if appState[btccheckpointtypes.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[btccheckpointtypes.ModuleName], &genesisState)
	}

	return genesisState
}

func (gi *GenesisInfo) GetBaseBTCHeight() uint32 {
	return gi.baseBTCHeight
}

func (gi *GenesisInfo) GetEpochInterval() uint64 {
	return gi.epochInterval
}

func (gi *GenesisInfo) GetBLSKeySet() checkpointingtypes.ValidatorWithBlsKeySet {
	return gi.valSet
}

func (gi *GenesisInfo) GetCheckpointTag() string {
	return gi.checkpointTag
}

func (gi *GenesisInfo) SetBaseBTCHeight(height uint32) {
	gi.baseBTCHeight = height
}
func messageValidatorCreateValidator(msgs []sdk.Msg) error {
	if len(msgs) != 1 {
		return fmt.Errorf("unexpected number of GenTx messages; got: %d, expected: 1", len(msgs))
	}

	_, ok := msgs[0].(*types.MsgCreateValidator)
	if !ok {
		return fmt.Errorf("unexpected GenTx message type; expected: MsgCreateValidator, got: %T", msgs[0])
	}

	return nil
}
