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
	ABI: "[{\"inputs\":[{\"internalType\":\"uint120\",\"name\":\"_blockHeight\",\"type\":\"uint120\"},{\"internalType\":\"bytes32\",\"name\":\"_blockHash\",\"type\":\"bytes32\"},{\"internalType\":\"uint120\",\"name\":\"_blockTime\",\"type\":\"uint120\"},{\"internalType\":\"uint256\",\"name\":\"_expectedTarget\",\"type\":\"uint256\"},{\"internalType\":\"bool\",\"name\":\"_isTestnet\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"BadParent\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"DifficultyRetargetLT25\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"HashAboveTarget\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientChainLength\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"InsufficientTotalDifficulty\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NoBlocksSubmitted\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"NoParent\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"OldDifficultyPeriod\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"TooDeepReorg\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WrongDifficultyBits\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"WrongHeaderLength\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockHeight\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockTime\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"blockHash\",\"type\":\"bytes32\"}],\"name\":\"NewTip\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"blockHeight\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"totalDifficulty\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint32\",\"name\":\"newDifficultyBits\",\"type\":\"uint32\"}],\"name\":\"NewTotalDifficultySinceRetarget\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"count\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"oldTip\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"newTip\",\"type\":\"bytes32\"}],\"name\":\"Reorg\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"blockHashes\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"blockNum\",\"type\":\"uint256\"}],\"name\":\"getBlockHash\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getLatestBlockHeight\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"getLatestBlockTime\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"bits\",\"type\":\"bytes32\"}],\"name\":\"getTarget\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"pure\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"isTestnet\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"name\":\"periodToTarget\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"blockHeight\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"blockHeaders\",\"type\":\"bytes\"}],\"name\":\"submit\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60a060405234801561000f575f5ffd5b50604051611a70380380611a7083398181016040528101906100319190610221565b6107e0856effffffffffffffffffffffffffffff161161004f575f5ffd5b8360016107d0876effffffffffffffffffffffffffffff1661007191906102c5565b6107d08110610083576100826102f5565b5b0181905550845f600f6101000a8154816effffffffffffffffffffffffffffff02191690836effffffffffffffffffffffffffffff160217905550825f5f6101000a8154816effffffffffffffffffffffffffffff02191690836effffffffffffffffffffffffffffff160217905550816107d15f6107e0886101069190610322565b6effffffffffffffffffffffffffffff1681526020019081526020015f20819055508015156080811515815250505050505050610352565b5f5ffd5b5f6effffffffffffffffffffffffffffff82169050919050565b61016581610142565b811461016f575f5ffd5b50565b5f815190506101808161015c565b92915050565b5f819050919050565b61019881610186565b81146101a2575f5ffd5b50565b5f815190506101b38161018f565b92915050565b5f819050919050565b6101cb816101b9565b81146101d5575f5ffd5b50565b5f815190506101e6816101c2565b92915050565b5f8115159050919050565b610200816101ec565b811461020a575f5ffd5b50565b5f8151905061021b816101f7565b92915050565b5f5f5f5f5f60a0868803121561023a5761023961013e565b5b5f61024788828901610172565b9550506020610258888289016101a5565b945050604061026988828901610172565b935050606061027a888289016101d8565b925050608061028b8882890161020d565b9150509295509295909350565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52601260045260245ffd5b5f6102cf826101b9565b91506102da836101b9565b9250826102ea576102e9610298565b5b828206905092915050565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52603260045260245ffd5b5f61032c82610142565b915061033783610142565b92508261034757610346610298565b5b828204905092915050565b6080516116f86103785f395f818161022501528181610d310152610dee01526116f85ff3fe608060405234801561000f575f5ffd5b5060043610610086575f3560e01c8063a9c2d7ab11610059578063a9c2d7ab14610126578063d1a2eab214610156578063e875aa5d14610172578063ee82ac5e1461019057610086565b80630d2b28601461008a57806332c25832146100ba57806334cdf78d146100d857806392108c8614610108575b5f5ffd5b6100a4600480360381019061009f9190610fdb565b6101c0565b6040516100b19190611015565b60405180910390f35b6100c26101d6565b6040516100cf9190611015565b60405180910390f35b6100f260048036038101906100ed9190610fdb565b610209565b6040516100ff9190611046565b60405180910390f35b610110610223565b60405161011d9190611079565b60405180910390f35b610140600480360381019061013b91906110bc565b610247565b60405161014d9190611015565b60405180910390f35b610170600480360381019061016b9190611148565b6102ff565b005b61017a610918565b6040516101879190611015565b60405180910390f35b6101aa60048036038101906101a59190610fdb565b61094c565b6040516101b79190611046565b60405180910390f35b6107d1602052805f5260405f205f915090505481565b5f5f5f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff16905090565b6001816107d08110610219575f80fd5b015f915090505481565b7f000000000000000000000000000000000000000000000000000000000000000081565b5f5f8260036020811061025d5761025c6111a5565b5b1a60f81b60f81c60ff1690505f8360026020811061027e5761027d6111a5565b5b1a60f81b60f81c60ff1690508360016020811061029e5761029d6111a5565b5b1a60f81b60f81c60ff16600882901b179050835f602081106102c3576102c26111a5565b5b1a60f81b60f81c60ff16600882901b1790505f6003836102e391906111ff565b60086102ef9190611232565b82901b9050809350505050919050565b6107e0831161030c575f5ffd5b5f6050838390508161032157610320611273565b5b049050828290506050820214610363576040517f8789b42d00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f810361039c576040517f83d7903300000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b6103e85f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff16038411610406576040517fe9aa60d000000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f6107e05f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff168161044657610445611273565b5b046effffffffffffffffffffffffffffff1690505f60018387010390505f6107e0828161047657610475611273565b5b049050828110156104b3576040517fbd4ee89800000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f6107e060018903816104c9576104c8611273565b5b0490505f5f905081831115610538576001820183146104e6575f5ffd5b84830361052b57610524855f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff16610a35565b9050610537565b818514610536575f5ffd5b5b5b5f61056f5f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff1661094c565b90505f8a5f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff160390505f5f90505b888110156105e7575f818d0190506105db818d8d856050029060018701605002926105d6939291906112a8565b610a9c565b508060010190506105a9565b508385111561073c57365f8b8b60018c0360500290809261060a939291906112a8565b915091505f6106398383604890604c92610626939291906112a8565b906106319190611323565b60e01c610e68565b90505f610646898b610a35565b9050868111610681576040517f83d81a4600000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f60018b0190505b5f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff1681116106f7575f5f1b60016107d083816106d4576106d3611273565b5b066107d081106106e7576106e66111a5565b5b0181905550806001019050610689565b507f452ea9c9e4985cb8cc7199131fd292d600dae30f407e749b6ca7e7f4c7690cf78a828460405161072b9392919061139f565b60405180910390a1505050506107b9565b868514610747575f5ffd5b838514610752575f5ffd5b5f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff1686116107b8576040517fc903dd4600000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5b855f600f6101000a8154816effffffffffffffffffffffffffffff02191690836effffffffffffffffffffffffffffff1602179055505f600c8b8b90500390505f8b8b8390600485019261080f939291906112a8565b9061081a9190611323565b60e01c905061082881610e68565b63ffffffff165f5f6101000a8154816effffffffffffffffffffffffffffff02191690836effffffffffffffffffffffffffffff1602179055505f61086c8961094c565b90507f11cedc7f83a9a6f5228b8fcd3c3c0ae686a51e2e04ea46923e3491a32a4bbaa8895f5f9054906101000a90046effffffffffffffffffffffffffffff16836040516108bc93929190611427565b60405180910390a15f841115610908577fea952b3dfe43a8e680ea58302c72429cd78ef0537abeaf75797ef9bb0496cefc8486836040516108ff9392919061145c565b60405180910390a15b5050505050505050505050505050565b5f5f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff16905090565b5f5f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff168211156109bd576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004016109b4906114eb565b60405180910390fd5b6103e85f600f9054906101000a90046effffffffffffffffffffffffffffff166effffffffffffffffffffffffffffff166109f891906111ff565b821015610a09575f5f1b9050610a30565b60016107d083610a199190611509565b6107d08110610a2b57610a2a6111a5565b5b015490505b919050565b5f5f6107d15f8581526020019081526020015f205490505f6001808301831981610a6257610a61611273565b5b040190505f60016107e08702860301905060018110158015610a8657506107e08111155b610a8e575f5ffd5b818102935050505092915050565b6107e08311610aa9575f5ffd5b60508282905014610ab8575f5ffd5b5f610b7e6002808585604051610acf929190611575565b602060405180830381855afa158015610aea573d5f5f3e3d5ffd5b5050506040513d601f19601f82011682018060405250810190610b0d91906115a1565b604051602001610b1d91906115ec565b604051602081830303815290604052604051610b39919061164e565b602060405180830381855afa158015610b54573d5f5f3e3d5ffd5b5050506040513d601f19601f82011682018060405250810190610b7791906115a1565b5f1c610eae565b9050805f1b60016107d08681610b9757610b96611273565b5b066107d08110610baa57610ba96111a5565b5b01819055505f610bd98484600490602492610bc7939291906112a8565b90610bd29190611664565b5f1c610eae565b5f1b905060016107d060016107d088010381610bf857610bf7611273565b5b066107d08110610c0b57610c0a6111a5565b5b01548114610c45576040517f190ad08b00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f5f1b8103610c80576040517f5d6e92cc00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f8484604890604c92610c95939291906112a8565b90610ca09190611664565b90505f610cac82610247565b9050808410610ce7576040517ff3fd212800000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5f6107e08881610cfa57610cf9611273565b5b0490505f6107e08981610d1057610d0f611273565b5b0603610dec575f6107d15f6001840381526020019081526020015f205490507f0000000000000000000000000000000000000000000000000000000000000000610dcf5780600284901c10610d91576040517f772fd2aa00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b80600284901b11610dce576040517f772fd2aa00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5b826107d15f8481526020019081526020015f208190555050610e5e565b7f0000000000000000000000000000000000000000000000000000000000000000610e5d576107d15f8281526020019081526020015f20548214610e5c576040517fd0e7266900000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b5b5b5050505050505050565b5f819050600862ff00ff821663ffffffff16901b600863ff00ff00831663ffffffff16901c17905060108163ffffffff16901b60108263ffffffff16901c179050919050565b5f8190505f7fff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff00ff009050600881198316901b6008828416901c1791505f7fffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff0000ffff00009050601081198416901b6010828516901c1792505f7fffffffff00000000ffffffff00000000ffffffff00000000ffffffff000000009050602081198516901b6020828616901c1793505f7fffffffffffffffff0000000000000000ffffffffffffffff00000000000000009050604081198616901b6040828716901c179450608085901b608086901c17945050505050919050565b5f5ffd5b5f5ffd5b5f819050919050565b610fba81610fa8565b8114610fc4575f5ffd5b50565b5f81359050610fd581610fb1565b92915050565b5f60208284031215610ff057610fef610fa0565b5b5f610ffd84828501610fc7565b91505092915050565b61100f81610fa8565b82525050565b5f6020820190506110285f830184611006565b92915050565b5f819050919050565b6110408161102e565b82525050565b5f6020820190506110595f830184611037565b92915050565b5f8115159050919050565b6110738161105f565b82525050565b5f60208201905061108c5f83018461106a565b92915050565b61109b8161102e565b81146110a5575f5ffd5b50565b5f813590506110b681611092565b92915050565b5f602082840312156110d1576110d0610fa0565b5b5f6110de848285016110a8565b91505092915050565b5f5ffd5b5f5ffd5b5f5ffd5b5f5f83601f840112611108576111076110e7565b5b8235905067ffffffffffffffff811115611125576111246110eb565b5b602083019150836001820283011115611141576111406110ef565b5b9250929050565b5f5f5f6040848603121561115f5761115e610fa0565b5b5f61116c86828701610fc7565b935050602084013567ffffffffffffffff81111561118d5761118c610fa4565b5b611199868287016110f3565b92509250509250925092565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52603260045260245ffd5b7f4e487b71000000000000000000000000000000000000000000000000000000005f52601160045260245ffd5b5f61120982610fa8565b915061121483610fa8565b925082820390508181111561122c5761122b6111d2565b5b92915050565b5f61123c82610fa8565b915061124783610fa8565b925082820261125581610fa8565b9150828204841483151761126c5761126b6111d2565b5b5092915050565b7f4e487b71000000000000000000000000000000000000000000000000000000005f52601260045260245ffd5b5f5ffd5b5f5ffd5b5f5f858511156112bb576112ba6112a0565b5b838611156112cc576112cb6112a4565b5b6001850283019150848603905094509492505050565b5f82905092915050565b5f7fffffffff0000000000000000000000000000000000000000000000000000000082169050919050565b5f82821b905092915050565b5f61132e83836112e2565b8261133981356112ec565b92506004821015611379576113747fffffffff0000000000000000000000000000000000000000000000000000000083600403600802611317565b831692505b505092915050565b5f63ffffffff82169050919050565b61139981611381565b82525050565b5f6060820190506113b25f830186611006565b6113bf6020830185611006565b6113cc6040830184611390565b949350505050565b5f6effffffffffffffffffffffffffffff82169050919050565b5f819050919050565b5f61141161140c611407846113d4565b6113ee565b610fa8565b9050919050565b611421816113f7565b82525050565b5f60608201905061143a5f830186611006565b6114476020830185611418565b6114546040830184611037565b949350505050565b5f60608201905061146f5f830186611006565b61147c6020830185611037565b6114896040830184611037565b949350505050565b5f82825260208201905092915050565b7f426c6f636b206e6f7420796574207375626d69747465640000000000000000005f82015250565b5f6114d5601783611491565b91506114e0826114a1565b602082019050919050565b5f6020820190508181035f830152611502816114c9565b9050919050565b5f61151382610fa8565b915061151e83610fa8565b92508261152e5761152d611273565b5b828206905092915050565b5f81905092915050565b828183375f83830152505050565b5f61155c8385611539565b9350611569838584611543565b82840190509392505050565b5f611581828486611551565b91508190509392505050565b5f8151905061159b81611092565b92915050565b5f602082840312156115b6576115b5610fa0565b5b5f6115c38482850161158d565b91505092915050565b5f819050919050565b6115e66115e18261102e565b6115cc565b82525050565b5f6115f782846115d5565b60208201915081905092915050565b5f81519050919050565b8281835e5f83830152505050565b5f61162882611606565b6116328185611539565b9350611642818560208601611610565b80840191505092915050565b5f611659828461161e565b915081905092915050565b5f61166f83836112e2565b8261167a813561102e565b925060208210156116ba576116b57fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff83602003600802611317565b831692505b50509291505056fea26469706673582212207f5a58757c136ae8540a6c9e413f9cc7c3af1c71318345a7df8a27296cb2702f64736f6c634300081e0033",
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
