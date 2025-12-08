// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package btcprism

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// BtcprismMetaData contains all meta data concerning the Btcprism contract.
var BtcprismMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"uint120\",\"name\":\"_blockHeight\",\"type\":\"uint120\"},{\"internalType\":\"bytes32\",\"name\":\"_blockHash\",\"type\":\"bytes32\"},{\"internalType\":\"uint120\",\"name\":\"_blockTime\",\"type\":\"uint120\"},{\"internalType\":\"uint256\",\"name\":\"_expectedTarget\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"_isTestnet\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"BadParent\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"DifficultyRetargetLT25\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"HashAboveTarget\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientChainLength\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientTotalDifficulty\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NoBlocksSubmitted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NoParent\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"OldDifficultyPeriod\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TooDeepReorg\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WrongDifficultyBits\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WrongHeaderLength\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockHeight\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockTime\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"blockHash\",\"type\":\"bytes32\"}],\"name\":\"NewTip\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockHeight\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"totalDifficulty\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"newDifficultyBits\",\"type\":\"uint32\"}],\"name\":\"NewTotalDifficultySinceRetarget\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"count\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"oldTip\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"newTip\",\"type\":\"bytes32\"}],\"name\":\"Reorg\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"blockHashes\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"blockNum\",\"type\":\"uint256\"}],\"name\":\"getBlockHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"blockHash\",\"type\":\"bytes32\"}],\"name\":\"getBlockNumber\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"blockNumber\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getLatestBlockHeight\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getLatestBlockTime\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"bits\",\"type\":\"bytes32\"}],\"name\":\"getTarget\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"pure\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"hashToHeight\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"isTestnet\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"periodToTarget\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"blockHeight\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"blockHeaders\",\"type\":\"bytes\"}],\"name\":\"submit\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60a060405234801561000f575f5ffd5b50604051610fe8380380610fe883398101604081905261002e91610109565b6107e0856001600160781b031611610044575f5ffd5b83600161005c6107d06001600160781b038916610178565b6107d0811061006d5761006d61018b565b01555f8481526107d1602052604081206001600160781b038781169182905582546001600160f01b031916600160781b9092026001600160781b0319169190911790851617815582906107d2906100c66107e08961019f565b6001600160781b0316815260208101919091526040015f20551515608052506101cd92505050565b80516001600160781b0381168114610104575f5ffd5b919050565b5f5f5f5f5f60a0868803121561011d575f5ffd5b610126866100ee565b94506020860151935061013b604087016100ee565b92506060860151915060808601518015158114610156575f5ffd5b809150509295509295909350565b634e487b7160e01b5f52601260045260245ffd5b5f8261018657610186610164565b500690565b634e487b7160e01b5f52603260045260245ffd5b5f6001600160781b038316806101b7576101b7610164565b6001600160781b03929092169190910492915050565b608051610df56101f35f395f818161012d01528181610b140152610b980152610df55ff3fe608060405234801561000f575f5ffd5b506004361061009b575f3560e01c806392108c861161006357806392108c8614610128578063a9c2d7ab1461015f578063d1a2eab214610172578063e875aa5d14610187578063ee82ac5e1461019e575f5ffd5b80630d2b28601461009f57806332c25832146100d257806334cdf78d146100e257806347378145146100f55780634e64bb6e14610108575b5f5ffd5b6100bf6100ad366004610bf4565b6107d26020525f908152604090205481565b6040519081526020015b60405180910390f35b5f546001600160781b03166100bf565b6100bf6100f0366004610bf4565b6101b1565b6100bf610103366004610bf4565b6101c8565b6100bf610116366004610bf4565b6107d16020525f908152604090205481565b61014f7f000000000000000000000000000000000000000000000000000000000000000081565b60405190151581526020016100c9565b6100bf61016d366004610bf4565b610218565b610185610180366004610c0b565b610259565b005b5f54600160781b90046001600160781b03166100bf565b6100bf6101ac366004610bf4565b6106d8565b6001816107d081106101c1575f80fd5b0154905081565b5f8181526107d16020526040812054908190036101e657505f919050565b5f54610205906103e890600160781b90046001600160781b0316610c96565b81101561021357505f919050565b919050565b5f600382811a90600284901a600890811b600186901a17901b84841a179083906102429084610c96565b61024d906008610cbd565b9190911b949350505050565b6107e08311610266575f5ffd5b6050808204908102821461028d57604051638789b42d60e01b815260040160405180910390fd5b805f036102ad576040516383d7903360e01b815260040160405180910390fd5b5f54600160781b90046001600160781b03166103e7190184116102e357604051630e9aa60d60e41b815260040160405180910390fd5b5f546107e0600160781b9091046001600160781b0390811682900416908286015f19019081048281101561032a576040516317a9dd1360e31b815260040160405180910390fd5b6107e05f198801045f8183111561037f57816001018314610349575f5ffd5b848303610374575f5461036d908690600160781b90046001600160781b0316610792565b905061037f565b81851461037f575f5ffd5b5f805461039b90600160781b90046001600160781b03166106d8565b5f8054919250600160781b9091046001600160781b03168b9003905b888110156103f2575f818d0190506103e9818d8d856050029086600101605002926103e493929190610ce8565b6107ee565b506001016103b7565b508385111561054757365f61040f8b60505f198d0102818f610ce8565b90925090505f610455610426604c60488587610ce8565b61042f91610d0f565b60d881901c63ff00ff001662ff00ff60e89290921c9190911617601081811b91901c1790565b90505f610462898b610792565b9050868111610484576040516341ec0d2360e11b815260040160405180910390fd5b60018a015b5f54600160781b90046001600160781b031681116104f8575f60016107d083066107d081106104ba576104ba610ca9565b0154905080156104d4575f8181526107d160205260408120555b5f60016107d084066107d081106104ed576104ed610ca9565b015550600101610489565b50604080518b81526020810183905263ffffffff84168183015290517f452ea9c9e4985cb8cc7199131fd292d600dae30f407e749b6ca7e7f4c7690cf79181900360600190a15050505061058e565b868514610552575f5ffd5b83851461055d575f5ffd5b5f54600160781b90046001600160781b0316861161058e57604051636481eea360e11b815260040160405180910390fd5b5f80546effffffffffffffffffffffffffffff60781b1916600160781b6001600160781b03891602178155600b198a01906105cf6007198c01838d8f610ce8565b6105d891610d0f565b60e01c905061060581600881811c62ff00ff1663ff00ff009290911b9190911617601081811c91901b1790565b5f80546effffffffffffffffffffffffffffff191663ffffffff92909216919091178155610632896106d8565b5f54604080518c81526001600160781b03909216602083015281018290529091507f11cedc7f83a9a6f5228b8fcd3c3c0ae686a51e2e04ea46923e3491a32a4bbaa89060600160405180910390a183156106c85760408051858152602081018790529081018290527fea952b3dfe43a8e680ea58302c72429cd78ef0537abeaf75797ef9bb0496cefc9060600160405180910390a15b5050505050505050505050505050565b5f8054600160781b90046001600160781b031682111561073e5760405162461bcd60e51b815260206004820152601760248201527f426c6f636b206e6f7420796574207375626d6974746564000000000000000000604482015260640160405180910390fd5b5f5461075d906103e890600160781b90046001600160781b0316610c96565b82101561076b57505f919050565b60016107796107d084610d47565b6107d0811061078a5761078a610ca9565b015492915050565b5f8281526107d2602052604081205481600182018219816107b5576107b5610cd4565b0460010190505f856107e00285036001019050600181101580156107db57506107e08111155b6107e3575f5ffd5b029150505b92915050565b6107e083116107fb575f5ffd5b60508114610807575f5ffd5b5f610a04600280858560405161081e929190610d66565b602060405180830381855afa158015610839573d5f5f3e3d5ffd5b5050506040513d601f19601f8201168201806040525081019061085c9190610d75565b60405160200161086e91815260200190565b60408051601f198184030181529082905261088891610d8c565b602060405180830381855afa1580156108a3573d5f5f3e3d5ffd5b5050506040513d601f19601f820116820180604052508101906108c69190610d75565b7bffffffff000000000000000000000000ffffffff00000000000000007eff000000ff000000ff000000ff000000ff000000ff000000ff000000ff0000600883811c9182167fff000000ff000000ff000000ff000000ff000000ff000000ff000000ff0000009490911b93841617601090811c7cff000000ff000000ff000000ff000000ff000000ff000000ff000000ff9092167dff000000ff000000ff000000ff000000ff000000ff000000ff000000ff009094169390931790921b91909117602081811c9283167fffffffff000000000000000000000000ffffffff0000000000000000000000009290911b91821617604090811c73ffffffff000000000000000000000000ffffffff90931677ffffffff000000000000000000000000ffffffff0000000090921691909117901b17608081811c91901b1790565b9050808060016107d087066107d08110610a2057610a20610ca9565b01555f8181526107d160205260408120869055610a4d610a44602460048789610ce8565b6108c691610da2565b905060016107d06107cf8801066107d08110610a6b57610a6b610ca9565b01548114610a8c5760405163190ad08b60e01b815260040160405180910390fd5b80610aaa5760405163175ba4b360e21b815260040160405180910390fd5b5f610ab9604c60488789610ce8565b610ac291610da2565b90505f610ace82610218565b9050808510610af057604051631e7fa42560e31b815260040160405180910390fd5b6107e08089049089065f03610b96575f1981015f9081526107d260205260409020547f0000000000000000000000000000000000000000000000000000000000000000610b7f5780600284901c10610b5b57604051633b97e95560e11b815260040160405180910390fd5b80600284901b11610b7f57604051633b97e95560e11b815260040160405180910390fd5b505f8181526107d260205260409020829055610be9565b7f0000000000000000000000000000000000000000000000000000000000000000610be9575f8181526107d260205260409020548214610be95760405163d0e7266960e01b815260040160405180910390fd5b505050505050505050565b5f60208284031215610c04575f5ffd5b5035919050565b5f5f5f60408486031215610c1d575f5ffd5b83359250602084013567ffffffffffffffff811115610c3a575f5ffd5b8401601f81018613610c4a575f5ffd5b803567ffffffffffffffff811115610c60575f5ffd5b866020828401011115610c71575f5ffd5b939660209190910195509293505050565b634e487b7160e01b5f52601160045260245ffd5b818103818111156107e8576107e8610c82565b634e487b7160e01b5f52603260045260245ffd5b80820281158282048414176107e8576107e8610c82565b634e487b7160e01b5f52601260045260245ffd5b5f5f85851115610cf6575f5ffd5b83861115610d02575f5ffd5b5050820193919092039150565b80356001600160e01b03198116906004841015610d40576001600160e01b0319600485900360031b81901b82161691505b5092915050565b5f82610d6157634e487b7160e01b5f52601260045260245ffd5b500690565b818382375f9101908152919050565b5f60208284031215610d85575f5ffd5b5051919050565b5f82518060208501845e5f920191825250919050565b803560208310156107e8575f19602084900360031b1b169291505056fea2646970667358221220b891d84a1a298be2ada8746a4e38c5a2c63d00af3949a23e6cfac59b265d5f3b64736f6c634300081c0033",
}

// BtcprismABI is the input ABI used to generate the binding from.
// Deprecated: Use BtcprismMetaData.ABI instead.
var BtcprismABI = BtcprismMetaData.ABI

// BtcprismBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use BtcprismMetaData.Bin instead.
var BtcprismBin = BtcprismMetaData.Bin

// DeployBtcprism deploys a new Ethereum contract, binding an instance of Btcprism to it.
func DeployBtcprism(auth *bind.TransactOpts, backend bind.ContractBackend, _blockHeight *big.Int, _blockHash [32]byte, _blockTime *big.Int, _expectedTarget *big.Int, _isTestnet bool) (common.Address, *types.Transaction, *Btcprism, error) {
	parsed, err := BtcprismMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(BtcprismBin), backend, _blockHeight, _blockHash, _blockTime, _expectedTarget, _isTestnet)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &Btcprism{BtcprismCaller: BtcprismCaller{contract: contract}, BtcprismTransactor: BtcprismTransactor{contract: contract}, BtcprismFilterer: BtcprismFilterer{contract: contract}}, nil
}

// Btcprism is an auto generated Go binding around an Ethereum contract.
type Btcprism struct {
	BtcprismCaller     // Read-only binding to the contract
	BtcprismTransactor // Write-only binding to the contract
	BtcprismFilterer   // Log filterer for contract events
}

// BtcprismCaller is an auto generated read-only Go binding around an Ethereum contract.
type BtcprismCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BtcprismTransactor is an auto generated write-only Go binding around an Ethereum contract.
type BtcprismTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BtcprismFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type BtcprismFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BtcprismSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type BtcprismSession struct {
	Contract     *Btcprism         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// BtcprismCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type BtcprismCallerSession struct {
	Contract *BtcprismCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// BtcprismTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type BtcprismTransactorSession struct {
	Contract     *BtcprismTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// BtcprismRaw is an auto generated low-level Go binding around an Ethereum contract.
type BtcprismRaw struct {
	Contract *Btcprism // Generic contract binding to access the raw methods on
}

// BtcprismCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type BtcprismCallerRaw struct {
	Contract *BtcprismCaller // Generic read-only contract binding to access the raw methods on
}

// BtcprismTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type BtcprismTransactorRaw struct {
	Contract *BtcprismTransactor // Generic write-only contract binding to access the raw methods on
}

// NewBtcprism creates a new instance of Btcprism, bound to a specific deployed contract.
func NewBtcprism(address common.Address, backend bind.ContractBackend) (*Btcprism, error) {
	contract, err := bindBtcprism(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Btcprism{BtcprismCaller: BtcprismCaller{contract: contract}, BtcprismTransactor: BtcprismTransactor{contract: contract}, BtcprismFilterer: BtcprismFilterer{contract: contract}}, nil
}

// NewBtcprismCaller creates a new read-only instance of Btcprism, bound to a specific deployed contract.
func NewBtcprismCaller(address common.Address, caller bind.ContractCaller) (*BtcprismCaller, error) {
	contract, err := bindBtcprism(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &BtcprismCaller{contract: contract}, nil
}

// NewBtcprismTransactor creates a new write-only instance of Btcprism, bound to a specific deployed contract.
func NewBtcprismTransactor(address common.Address, transactor bind.ContractTransactor) (*BtcprismTransactor, error) {
	contract, err := bindBtcprism(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &BtcprismTransactor{contract: contract}, nil
}

// NewBtcprismFilterer creates a new log filterer instance of Btcprism, bound to a specific deployed contract.
func NewBtcprismFilterer(address common.Address, filterer bind.ContractFilterer) (*BtcprismFilterer, error) {
	contract, err := bindBtcprism(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &BtcprismFilterer{contract: contract}, nil
}

// bindBtcprism binds a generic wrapper to an already deployed contract.
func bindBtcprism(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := BtcprismMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Btcprism *BtcprismRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Btcprism.Contract.BtcprismCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Btcprism *BtcprismRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Btcprism.Contract.BtcprismTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Btcprism *BtcprismRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Btcprism.Contract.BtcprismTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Btcprism *BtcprismCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Btcprism.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Btcprism *BtcprismTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Btcprism.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Btcprism *BtcprismTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Btcprism.Contract.contract.Transact(opts, method, params...)
}

// BlockHashes is a free data retrieval call binding the contract method 0x34cdf78d.
//
// Solidity: function blockHashes(uint256 ) view returns(bytes32)
func (_Btcprism *BtcprismCaller) BlockHashes(opts *bind.CallOpts, arg0 *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "blockHashes", arg0)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// BlockHashes is a free data retrieval call binding the contract method 0x34cdf78d.
//
// Solidity: function blockHashes(uint256 ) view returns(bytes32)
func (_Btcprism *BtcprismSession) BlockHashes(arg0 *big.Int) ([32]byte, error) {
	return _Btcprism.Contract.BlockHashes(&_Btcprism.CallOpts, arg0)
}

// BlockHashes is a free data retrieval call binding the contract method 0x34cdf78d.
//
// Solidity: function blockHashes(uint256 ) view returns(bytes32)
func (_Btcprism *BtcprismCallerSession) BlockHashes(arg0 *big.Int) ([32]byte, error) {
	return _Btcprism.Contract.BlockHashes(&_Btcprism.CallOpts, arg0)
}

// GetBlockHash is a free data retrieval call binding the contract method 0xee82ac5e.
//
// Solidity: function getBlockHash(uint256 blockNum) view returns(bytes32)
func (_Btcprism *BtcprismCaller) GetBlockHash(opts *bind.CallOpts, blockNum *big.Int) ([32]byte, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "getBlockHash", blockNum)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetBlockHash is a free data retrieval call binding the contract method 0xee82ac5e.
//
// Solidity: function getBlockHash(uint256 blockNum) view returns(bytes32)
func (_Btcprism *BtcprismSession) GetBlockHash(blockNum *big.Int) ([32]byte, error) {
	return _Btcprism.Contract.GetBlockHash(&_Btcprism.CallOpts, blockNum)
}

// GetBlockHash is a free data retrieval call binding the contract method 0xee82ac5e.
//
// Solidity: function getBlockHash(uint256 blockNum) view returns(bytes32)
func (_Btcprism *BtcprismCallerSession) GetBlockHash(blockNum *big.Int) ([32]byte, error) {
	return _Btcprism.Contract.GetBlockHash(&_Btcprism.CallOpts, blockNum)
}

// GetBlockNumber is a free data retrieval call binding the contract method 0x47378145.
//
// Solidity: function getBlockNumber(bytes32 blockHash) view returns(uint256 blockNumber)
func (_Btcprism *BtcprismCaller) GetBlockNumber(opts *bind.CallOpts, blockHash [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "getBlockNumber", blockHash)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetBlockNumber is a free data retrieval call binding the contract method 0x47378145.
//
// Solidity: function getBlockNumber(bytes32 blockHash) view returns(uint256 blockNumber)
func (_Btcprism *BtcprismSession) GetBlockNumber(blockHash [32]byte) (*big.Int, error) {
	return _Btcprism.Contract.GetBlockNumber(&_Btcprism.CallOpts, blockHash)
}

// GetBlockNumber is a free data retrieval call binding the contract method 0x47378145.
//
// Solidity: function getBlockNumber(bytes32 blockHash) view returns(uint256 blockNumber)
func (_Btcprism *BtcprismCallerSession) GetBlockNumber(blockHash [32]byte) (*big.Int, error) {
	return _Btcprism.Contract.GetBlockNumber(&_Btcprism.CallOpts, blockHash)
}

// GetLatestBlockHeight is a free data retrieval call binding the contract method 0xe875aa5d.
//
// Solidity: function getLatestBlockHeight() view returns(uint256)
func (_Btcprism *BtcprismCaller) GetLatestBlockHeight(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "getLatestBlockHeight")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetLatestBlockHeight is a free data retrieval call binding the contract method 0xe875aa5d.
//
// Solidity: function getLatestBlockHeight() view returns(uint256)
func (_Btcprism *BtcprismSession) GetLatestBlockHeight() (*big.Int, error) {
	return _Btcprism.Contract.GetLatestBlockHeight(&_Btcprism.CallOpts)
}

// GetLatestBlockHeight is a free data retrieval call binding the contract method 0xe875aa5d.
//
// Solidity: function getLatestBlockHeight() view returns(uint256)
func (_Btcprism *BtcprismCallerSession) GetLatestBlockHeight() (*big.Int, error) {
	return _Btcprism.Contract.GetLatestBlockHeight(&_Btcprism.CallOpts)
}

// GetLatestBlockTime is a free data retrieval call binding the contract method 0x32c25832.
//
// Solidity: function getLatestBlockTime() view returns(uint256)
func (_Btcprism *BtcprismCaller) GetLatestBlockTime(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "getLatestBlockTime")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetLatestBlockTime is a free data retrieval call binding the contract method 0x32c25832.
//
// Solidity: function getLatestBlockTime() view returns(uint256)
func (_Btcprism *BtcprismSession) GetLatestBlockTime() (*big.Int, error) {
	return _Btcprism.Contract.GetLatestBlockTime(&_Btcprism.CallOpts)
}

// GetLatestBlockTime is a free data retrieval call binding the contract method 0x32c25832.
//
// Solidity: function getLatestBlockTime() view returns(uint256)
func (_Btcprism *BtcprismCallerSession) GetLatestBlockTime() (*big.Int, error) {
	return _Btcprism.Contract.GetLatestBlockTime(&_Btcprism.CallOpts)
}

// GetTarget is a free data retrieval call binding the contract method 0xa9c2d7ab.
//
// Solidity: function getTarget(bytes32 bits) pure returns(uint256)
func (_Btcprism *BtcprismCaller) GetTarget(opts *bind.CallOpts, bits [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "getTarget", bits)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetTarget is a free data retrieval call binding the contract method 0xa9c2d7ab.
//
// Solidity: function getTarget(bytes32 bits) pure returns(uint256)
func (_Btcprism *BtcprismSession) GetTarget(bits [32]byte) (*big.Int, error) {
	return _Btcprism.Contract.GetTarget(&_Btcprism.CallOpts, bits)
}

// GetTarget is a free data retrieval call binding the contract method 0xa9c2d7ab.
//
// Solidity: function getTarget(bytes32 bits) pure returns(uint256)
func (_Btcprism *BtcprismCallerSession) GetTarget(bits [32]byte) (*big.Int, error) {
	return _Btcprism.Contract.GetTarget(&_Btcprism.CallOpts, bits)
}

// HashToHeight is a free data retrieval call binding the contract method 0x4e64bb6e.
//
// Solidity: function hashToHeight(bytes32 ) view returns(uint256)
func (_Btcprism *BtcprismCaller) HashToHeight(opts *bind.CallOpts, arg0 [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "hashToHeight", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// HashToHeight is a free data retrieval call binding the contract method 0x4e64bb6e.
//
// Solidity: function hashToHeight(bytes32 ) view returns(uint256)
func (_Btcprism *BtcprismSession) HashToHeight(arg0 [32]byte) (*big.Int, error) {
	return _Btcprism.Contract.HashToHeight(&_Btcprism.CallOpts, arg0)
}

// HashToHeight is a free data retrieval call binding the contract method 0x4e64bb6e.
//
// Solidity: function hashToHeight(bytes32 ) view returns(uint256)
func (_Btcprism *BtcprismCallerSession) HashToHeight(arg0 [32]byte) (*big.Int, error) {
	return _Btcprism.Contract.HashToHeight(&_Btcprism.CallOpts, arg0)
}

// IsTestnet is a free data retrieval call binding the contract method 0x92108c86.
//
// Solidity: function isTestnet() view returns(bool)
func (_Btcprism *BtcprismCaller) IsTestnet(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "isTestnet")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsTestnet is a free data retrieval call binding the contract method 0x92108c86.
//
// Solidity: function isTestnet() view returns(bool)
func (_Btcprism *BtcprismSession) IsTestnet() (bool, error) {
	return _Btcprism.Contract.IsTestnet(&_Btcprism.CallOpts)
}

// IsTestnet is a free data retrieval call binding the contract method 0x92108c86.
//
// Solidity: function isTestnet() view returns(bool)
func (_Btcprism *BtcprismCallerSession) IsTestnet() (bool, error) {
	return _Btcprism.Contract.IsTestnet(&_Btcprism.CallOpts)
}

// PeriodToTarget is a free data retrieval call binding the contract method 0x0d2b2860.
//
// Solidity: function periodToTarget(uint256 ) view returns(uint256)
func (_Btcprism *BtcprismCaller) PeriodToTarget(opts *bind.CallOpts, arg0 *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _Btcprism.contract.Call(opts, &out, "periodToTarget", arg0)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// PeriodToTarget is a free data retrieval call binding the contract method 0x0d2b2860.
//
// Solidity: function periodToTarget(uint256 ) view returns(uint256)
func (_Btcprism *BtcprismSession) PeriodToTarget(arg0 *big.Int) (*big.Int, error) {
	return _Btcprism.Contract.PeriodToTarget(&_Btcprism.CallOpts, arg0)
}

// PeriodToTarget is a free data retrieval call binding the contract method 0x0d2b2860.
//
// Solidity: function periodToTarget(uint256 ) view returns(uint256)
func (_Btcprism *BtcprismCallerSession) PeriodToTarget(arg0 *big.Int) (*big.Int, error) {
	return _Btcprism.Contract.PeriodToTarget(&_Btcprism.CallOpts, arg0)
}

// Submit is a paid mutator transaction binding the contract method 0xd1a2eab2.
//
// Solidity: function submit(uint256 blockHeight, bytes blockHeaders) returns()
func (_Btcprism *BtcprismTransactor) Submit(opts *bind.TransactOpts, blockHeight *big.Int, blockHeaders []byte) (*types.Transaction, error) {
	return _Btcprism.contract.Transact(opts, "submit", blockHeight, blockHeaders)
}

// Submit is a paid mutator transaction binding the contract method 0xd1a2eab2.
//
// Solidity: function submit(uint256 blockHeight, bytes blockHeaders) returns()
func (_Btcprism *BtcprismSession) Submit(blockHeight *big.Int, blockHeaders []byte) (*types.Transaction, error) {
	return _Btcprism.Contract.Submit(&_Btcprism.TransactOpts, blockHeight, blockHeaders)
}

// Submit is a paid mutator transaction binding the contract method 0xd1a2eab2.
//
// Solidity: function submit(uint256 blockHeight, bytes blockHeaders) returns()
func (_Btcprism *BtcprismTransactorSession) Submit(blockHeight *big.Int, blockHeaders []byte) (*types.Transaction, error) {
	return _Btcprism.Contract.Submit(&_Btcprism.TransactOpts, blockHeight, blockHeaders)
}

// BtcprismNewTipIterator is returned from FilterNewTip and is used to iterate over the raw logs and unpacked data for NewTip events raised by the Btcprism contract.
type BtcprismNewTipIterator struct {
	Event *BtcprismNewTip // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BtcprismNewTipIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BtcprismNewTip)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BtcprismNewTip)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BtcprismNewTipIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BtcprismNewTipIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BtcprismNewTip represents a NewTip event raised by the Btcprism contract.
type BtcprismNewTip struct {
	BlockHeight *big.Int
	BlockTime   *big.Int
	BlockHash   [32]byte
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterNewTip is a free log retrieval operation binding the contract event 0x11cedc7f83a9a6f5228b8fcd3c3c0ae686a51e2e04ea46923e3491a32a4bbaa8.
//
// Solidity: event NewTip(uint256 blockHeight, uint256 blockTime, bytes32 blockHash)
func (_Btcprism *BtcprismFilterer) FilterNewTip(opts *bind.FilterOpts) (*BtcprismNewTipIterator, error) {

	logs, sub, err := _Btcprism.contract.FilterLogs(opts, "NewTip")
	if err != nil {
		return nil, err
	}
	return &BtcprismNewTipIterator{contract: _Btcprism.contract, event: "NewTip", logs: logs, sub: sub}, nil
}

// WatchNewTip is a free log subscription operation binding the contract event 0x11cedc7f83a9a6f5228b8fcd3c3c0ae686a51e2e04ea46923e3491a32a4bbaa8.
//
// Solidity: event NewTip(uint256 blockHeight, uint256 blockTime, bytes32 blockHash)
func (_Btcprism *BtcprismFilterer) WatchNewTip(opts *bind.WatchOpts, sink chan<- *BtcprismNewTip) (event.Subscription, error) {

	logs, sub, err := _Btcprism.contract.WatchLogs(opts, "NewTip")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BtcprismNewTip)
				if err := _Btcprism.contract.UnpackLog(event, "NewTip", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseNewTip is a log parse operation binding the contract event 0x11cedc7f83a9a6f5228b8fcd3c3c0ae686a51e2e04ea46923e3491a32a4bbaa8.
//
// Solidity: event NewTip(uint256 blockHeight, uint256 blockTime, bytes32 blockHash)
func (_Btcprism *BtcprismFilterer) ParseNewTip(log types.Log) (*BtcprismNewTip, error) {
	event := new(BtcprismNewTip)
	if err := _Btcprism.contract.UnpackLog(event, "NewTip", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BtcprismNewTotalDifficultySinceRetargetIterator is returned from FilterNewTotalDifficultySinceRetarget and is used to iterate over the raw logs and unpacked data for NewTotalDifficultySinceRetarget events raised by the Btcprism contract.
type BtcprismNewTotalDifficultySinceRetargetIterator struct {
	Event *BtcprismNewTotalDifficultySinceRetarget // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BtcprismNewTotalDifficultySinceRetargetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BtcprismNewTotalDifficultySinceRetarget)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BtcprismNewTotalDifficultySinceRetarget)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BtcprismNewTotalDifficultySinceRetargetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BtcprismNewTotalDifficultySinceRetargetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BtcprismNewTotalDifficultySinceRetarget represents a NewTotalDifficultySinceRetarget event raised by the Btcprism contract.
type BtcprismNewTotalDifficultySinceRetarget struct {
	BlockHeight       *big.Int
	TotalDifficulty   *big.Int
	NewDifficultyBits uint32
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterNewTotalDifficultySinceRetarget is a free log retrieval operation binding the contract event 0x452ea9c9e4985cb8cc7199131fd292d600dae30f407e749b6ca7e7f4c7690cf7.
//
// Solidity: event NewTotalDifficultySinceRetarget(uint256 blockHeight, uint256 totalDifficulty, uint32 newDifficultyBits)
func (_Btcprism *BtcprismFilterer) FilterNewTotalDifficultySinceRetarget(opts *bind.FilterOpts) (*BtcprismNewTotalDifficultySinceRetargetIterator, error) {

	logs, sub, err := _Btcprism.contract.FilterLogs(opts, "NewTotalDifficultySinceRetarget")
	if err != nil {
		return nil, err
	}
	return &BtcprismNewTotalDifficultySinceRetargetIterator{contract: _Btcprism.contract, event: "NewTotalDifficultySinceRetarget", logs: logs, sub: sub}, nil
}

// WatchNewTotalDifficultySinceRetarget is a free log subscription operation binding the contract event 0x452ea9c9e4985cb8cc7199131fd292d600dae30f407e749b6ca7e7f4c7690cf7.
//
// Solidity: event NewTotalDifficultySinceRetarget(uint256 blockHeight, uint256 totalDifficulty, uint32 newDifficultyBits)
func (_Btcprism *BtcprismFilterer) WatchNewTotalDifficultySinceRetarget(opts *bind.WatchOpts, sink chan<- *BtcprismNewTotalDifficultySinceRetarget) (event.Subscription, error) {

	logs, sub, err := _Btcprism.contract.WatchLogs(opts, "NewTotalDifficultySinceRetarget")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BtcprismNewTotalDifficultySinceRetarget)
				if err := _Btcprism.contract.UnpackLog(event, "NewTotalDifficultySinceRetarget", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseNewTotalDifficultySinceRetarget is a log parse operation binding the contract event 0x452ea9c9e4985cb8cc7199131fd292d600dae30f407e749b6ca7e7f4c7690cf7.
//
// Solidity: event NewTotalDifficultySinceRetarget(uint256 blockHeight, uint256 totalDifficulty, uint32 newDifficultyBits)
func (_Btcprism *BtcprismFilterer) ParseNewTotalDifficultySinceRetarget(log types.Log) (*BtcprismNewTotalDifficultySinceRetarget, error) {
	event := new(BtcprismNewTotalDifficultySinceRetarget)
	if err := _Btcprism.contract.UnpackLog(event, "NewTotalDifficultySinceRetarget", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// BtcprismReorgIterator is returned from FilterReorg and is used to iterate over the raw logs and unpacked data for Reorg events raised by the Btcprism contract.
type BtcprismReorgIterator struct {
	Event *BtcprismReorg // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *BtcprismReorgIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(BtcprismReorg)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(BtcprismReorg)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *BtcprismReorgIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *BtcprismReorgIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// BtcprismReorg represents a Reorg event raised by the Btcprism contract.
type BtcprismReorg struct {
	Count  *big.Int
	OldTip [32]byte
	NewTip [32]byte
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterReorg is a free log retrieval operation binding the contract event 0xea952b3dfe43a8e680ea58302c72429cd78ef0537abeaf75797ef9bb0496cefc.
//
// Solidity: event Reorg(uint256 count, bytes32 oldTip, bytes32 newTip)
func (_Btcprism *BtcprismFilterer) FilterReorg(opts *bind.FilterOpts) (*BtcprismReorgIterator, error) {

	logs, sub, err := _Btcprism.contract.FilterLogs(opts, "Reorg")
	if err != nil {
		return nil, err
	}
	return &BtcprismReorgIterator{contract: _Btcprism.contract, event: "Reorg", logs: logs, sub: sub}, nil
}

// WatchReorg is a free log subscription operation binding the contract event 0xea952b3dfe43a8e680ea58302c72429cd78ef0537abeaf75797ef9bb0496cefc.
//
// Solidity: event Reorg(uint256 count, bytes32 oldTip, bytes32 newTip)
func (_Btcprism *BtcprismFilterer) WatchReorg(opts *bind.WatchOpts, sink chan<- *BtcprismReorg) (event.Subscription, error) {

	logs, sub, err := _Btcprism.contract.WatchLogs(opts, "Reorg")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(BtcprismReorg)
				if err := _Btcprism.contract.UnpackLog(event, "Reorg", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseReorg is a log parse operation binding the contract event 0xea952b3dfe43a8e680ea58302c72429cd78ef0537abeaf75797ef9bb0496cefc.
//
// Solidity: event Reorg(uint256 count, bytes32 oldTip, bytes32 newTip)
func (_Btcprism *BtcprismFilterer) ParseReorg(log types.Log) (*BtcprismReorg, error) {
	event := new(BtcprismReorg)
	if err := _Btcprism.contract.UnpackLog(event, "Reorg", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
