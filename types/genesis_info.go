package types // nolint:revive

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/babylonlabs-io/babylon/v3/app"
	btccheckpointtypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	btclightclienttypes "github.com/babylonlabs-io/babylon/v3/x/btclightclient/types"
	checkpointingtypes "github.com/babylonlabs-io/babylon/v3/x/checkpointing/types"
	epochingtypes "github.com/babylonlabs-io/babylon/v3/x/epoching/types"
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

	checkpointingGenState, err := getCheckpointGenState(tmpBabylon.AppCodec(), appState)
	if err != nil {
		return nil, fmt.Errorf("invalid checkpointing genesis %w ", err)
	}
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

	epochingParams, err := getEpochingParamsFromAppState(tmpBabylon.AppCodec(), appState)
	if err != nil {
		return nil, fmt.Errorf("invalid epoching genesis %w", err)
	}
	err = epochingParams.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid epoching params %w", err)
	}
	epochInterval = epochingParams.EpochInterval

	btccheckpointParams, err := getBtccheckpointParamsFromAppState(tmpBabylon.AppCodec(), appState)
	if err != nil {
		return nil, fmt.Errorf("invalid btccheckpoint genesis %w", err)
	}
	err = btccheckpointParams.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid btccheckpoint params %w", err)
	}
	checkpointTag = btccheckpointParams.CheckpointTag

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

// getEpochingParamsFromAppState returns x/epoching Params given raw application
// genesis state.
// NOTE: we get only the params for compatibility with changes on the genesis
func getEpochingParamsFromAppState(cdc codec.Codec, appState map[string]json.RawMessage) (epochingtypes.Params, error) {
	var params epochingtypes.Params
	if appState[epochingtypes.ModuleName] != nil {
		// Unmarshal the nested field manually
		var raw map[string]json.RawMessage
		err := json.Unmarshal(appState[epochingtypes.ModuleName], &raw)
		if err != nil {
			return params, err
		}

		// Extract just the "params" field
		paramsRaw, ok := raw["params"]
		if !ok {
			return params, errors.New("params not found")
		}

		err = cdc.UnmarshalJSON(paramsRaw, &params)
		if err != nil {
			return params, err
		}
	}

	return params, nil
}

// getBtccheckpointParamsFromAppState returns x/btccheckpoint Params given raw application
// genesis state.
func getBtccheckpointParamsFromAppState(cdc codec.Codec, appState map[string]json.RawMessage) (btccheckpointtypes.Params, error) {
	var params btccheckpointtypes.Params

	if appState[btccheckpointtypes.ModuleName] != nil {
		// Unmarshal the nested field manually
		var raw map[string]json.RawMessage
		err := json.Unmarshal(appState[btccheckpointtypes.ModuleName], &raw)
		if err != nil {
			return params, err
		}

		// Extract just the "params" field
		paramsRaw, ok := raw["params"]
		if !ok {
			return params, errors.New("params not found")
		}

		err = cdc.UnmarshalJSON(paramsRaw, &params)
		if err != nil {
			return params, err
		}
	}

	return params, nil
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

// getCheckpointGenState gets only the genesis keys of the genesis state.
// NOTE: this is the only field we care about, and we use this function to avoid
// having errors if new fields are added to the genesis (eg. like in v2 changes)
func getCheckpointGenState(cdc codec.Codec, appState map[string]json.RawMessage) (checkpointingtypes.GenesisState, error) {
	var genesisState checkpointingtypes.GenesisState

	if appState[checkpointingtypes.ModuleName] != nil {
		// Unmarshal the nested field manually
		var raw map[string]json.RawMessage
		err := json.Unmarshal(appState[checkpointingtypes.ModuleName], &raw)
		if err != nil {
			return genesisState, err
		}

		// Extract just the "genesis_keys" field
		genKeysRaw, ok := raw["genesis_keys"]
		if !ok {
			return genesisState, errors.New("genesis_keys not found")
		}

		// Decode genKeysRaw as a list of raw messages
		var rawKeys []json.RawMessage
		err = json.Unmarshal(genKeysRaw, &rawKeys)
		if err != nil {
			return genesisState, fmt.Errorf("failed to unmarshal genesis_keys array: %w", err)
		}

		var keys []*checkpointingtypes.GenesisKey
		for _, rk := range rawKeys {
			var key checkpointingtypes.GenesisKey
			if err := cdc.UnmarshalJSON(rk, &key); err != nil {
				return genesisState, fmt.Errorf("failed to decode genesis key: %w", err)
			}
			keys = append(keys, &key)
		}
		genesisState.GenesisKeys = keys
	}

	return genesisState, nil
}
