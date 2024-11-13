package types

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/babylonlabs-io/babylon/app"
	btccheckpointtypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclightclienttypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	checkpointingtypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	epochingtypes "github.com/babylonlabs-io/babylon/x/epoching/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
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
	err = checkpointingGenState.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid checkpointing genesis %w", err)
	}
	genutilGenState := genutiltypes.GetGenesisStateFromAppState(tmpBabylon.AppCodec(), appState)
	gentxs := genutilGenState.GenTxs
	gks := checkpointingGenState.GetGenesisKeys()

	valSet.ValSet = make([]*checkpointingtypes.ValidatorWithBlsKey, 0)
	for _, gk := range gks {
		for _, tx := range gentxs {
			tx, err := genutiltypes.ValidateAndGetGenTx(tx, tmpBabylon.TxConfig().TxJSONDecoder(), gentxModule.GenTxValidator)
			if err != nil {
				return nil, fmt.Errorf("invalid genesis tx %w", err)
			}
			msgs := tx.GetMsgs()
			if len(msgs) == 0 {
				return nil, errors.New("invalid genesis transaction")
			}
			msgCreateValidator, ok := msgs[0].(*stakingtypes.MsgCreateValidator)
			if !ok {
				return nil, fmt.Errorf("unexpected message type: %T", msgs[0])
			}

			if gk.ValidatorAddress == msgCreateValidator.ValidatorAddress {
				power := sdk.TokensToConsensusPower(msgCreateValidator.Value.Amount, sdk.DefaultPowerReduction)
				if power < 0 {
					return nil, fmt.Errorf("consensus power calculation returned a negative value")
				}
				keyWithPower := &checkpointingtypes.ValidatorWithBlsKey{
					ValidatorAddress: msgCreateValidator.ValidatorAddress,
					BlsPubKey:        *gk.BlsKey.Pubkey,
					VotingPower:      uint64(power),
				}
				valSet.ValSet = append(valSet.ValSet, keyWithPower)
			}
		}
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
