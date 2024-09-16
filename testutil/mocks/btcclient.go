// Code generated by MockGen. DO NOT EDIT.
// Source: btcclient/interface.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	config "github.com/babylonlabs-io/vigilante/config"
	types "github.com/babylonlabs-io/vigilante/types"
	btcjson "github.com/btcsuite/btcd/btcjson"
	btcutil "github.com/btcsuite/btcd/btcutil"
	chaincfg "github.com/btcsuite/btcd/chaincfg"
	chainhash "github.com/btcsuite/btcd/chaincfg/chainhash"
	wire "github.com/btcsuite/btcd/wire"
	gomock "github.com/golang/mock/gomock"
)

// MockBTCClient is a mock of BTCClient interface.
type MockBTCClient struct {
	ctrl     *gomock.Controller
	recorder *MockBTCClientMockRecorder
}

// MockBTCClientMockRecorder is the mock recorder for MockBTCClient.
type MockBTCClientMockRecorder struct {
	mock *MockBTCClient
}

// NewMockBTCClient creates a new mock instance.
func NewMockBTCClient(ctrl *gomock.Controller) *MockBTCClient {
	mock := &MockBTCClient{ctrl: ctrl}
	mock.recorder = &MockBTCClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBTCClient) EXPECT() *MockBTCClientMockRecorder {
	return m.recorder
}

// FindTailBlocksByHeight mocks base method.
func (m *MockBTCClient) FindTailBlocksByHeight(height uint64) ([]*types.IndexedBlock, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindTailBlocksByHeight", height)
	ret0, _ := ret[0].([]*types.IndexedBlock)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindTailBlocksByHeight indicates an expected call of FindTailBlocksByHeight.
func (mr *MockBTCClientMockRecorder) FindTailBlocksByHeight(height interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindTailBlocksByHeight", reflect.TypeOf((*MockBTCClient)(nil).FindTailBlocksByHeight), height)
}

// GetBestBlock mocks base method.
func (m *MockBTCClient) GetBestBlock() (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBestBlock")
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetBestBlock indicates an expected call of GetBestBlock.
func (mr *MockBTCClientMockRecorder) GetBestBlock() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBestBlock", reflect.TypeOf((*MockBTCClient)(nil).GetBestBlock))
}

// GetBlockByHash mocks base method.
func (m *MockBTCClient) GetBlockByHash(blockHash *chainhash.Hash) (*types.IndexedBlock, *wire.MsgBlock, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBlockByHash", blockHash)
	ret0, _ := ret[0].(*types.IndexedBlock)
	ret1, _ := ret[1].(*wire.MsgBlock)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetBlockByHash indicates an expected call of GetBlockByHash.
func (mr *MockBTCClientMockRecorder) GetBlockByHash(blockHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBlockByHash", reflect.TypeOf((*MockBTCClient)(nil).GetBlockByHash), blockHash)
}

// GetBlockByHeight mocks base method.
func (m *MockBTCClient) GetBlockByHeight(height uint64) (*types.IndexedBlock, *wire.MsgBlock, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBlockByHeight", height)
	ret0, _ := ret[0].(*types.IndexedBlock)
	ret1, _ := ret[1].(*wire.MsgBlock)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetBlockByHeight indicates an expected call of GetBlockByHeight.
func (mr *MockBTCClientMockRecorder) GetBlockByHeight(height interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBlockByHeight", reflect.TypeOf((*MockBTCClient)(nil).GetBlockByHeight), height)
}

// GetRawTransaction mocks base method.
func (m *MockBTCClient) GetRawTransaction(txHash *chainhash.Hash) (*btcutil.Tx, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRawTransaction", txHash)
	ret0, _ := ret[0].(*btcutil.Tx)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRawTransaction indicates an expected call of GetRawTransaction.
func (mr *MockBTCClientMockRecorder) GetRawTransaction(txHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRawTransaction", reflect.TypeOf((*MockBTCClient)(nil).GetRawTransaction), txHash)
}

// GetTransaction mocks base method.
func (m *MockBTCClient) GetTransaction(txHash *chainhash.Hash) (*btcjson.GetTransactionResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTransaction", txHash)
	ret0, _ := ret[0].(*btcjson.GetTransactionResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTransaction indicates an expected call of GetTransaction.
func (mr *MockBTCClientMockRecorder) GetTransaction(txHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTransaction", reflect.TypeOf((*MockBTCClient)(nil).GetTransaction), txHash)
}

// GetTxOut mocks base method.
func (m *MockBTCClient) GetTxOut(txHash *chainhash.Hash, index uint32, mempool bool) (*btcjson.GetTxOutResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetTxOut", txHash, index, mempool)
	ret0, _ := ret[0].(*btcjson.GetTxOutResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetTxOut indicates an expected call of GetTxOut.
func (mr *MockBTCClientMockRecorder) GetTxOut(txHash, index, mempool interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetTxOut", reflect.TypeOf((*MockBTCClient)(nil).GetTxOut), txHash, index, mempool)
}

// SendRawTransaction mocks base method.
func (m *MockBTCClient) SendRawTransaction(tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendRawTransaction", tx, allowHighFees)
	ret0, _ := ret[0].(*chainhash.Hash)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SendRawTransaction indicates an expected call of SendRawTransaction.
func (mr *MockBTCClientMockRecorder) SendRawTransaction(tx, allowHighFees interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendRawTransaction", reflect.TypeOf((*MockBTCClient)(nil).SendRawTransaction), tx, allowHighFees)
}

// Stop mocks base method.
func (m *MockBTCClient) Stop() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Stop")
}

// Stop indicates an expected call of Stop.
func (mr *MockBTCClientMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockBTCClient)(nil).Stop))
}

// WaitForShutdown mocks base method.
func (m *MockBTCClient) WaitForShutdown() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "WaitForShutdown")
}

// WaitForShutdown indicates an expected call of WaitForShutdown.
func (mr *MockBTCClientMockRecorder) WaitForShutdown() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WaitForShutdown", reflect.TypeOf((*MockBTCClient)(nil).WaitForShutdown))
}

// MockBTCWallet is a mock of BTCWallet interface.
type MockBTCWallet struct {
	ctrl     *gomock.Controller
	recorder *MockBTCWalletMockRecorder
}

// MockBTCWalletMockRecorder is the mock recorder for MockBTCWallet.
type MockBTCWalletMockRecorder struct {
	mock *MockBTCWallet
}

// NewMockBTCWallet creates a new mock instance.
func NewMockBTCWallet(ctrl *gomock.Controller) *MockBTCWallet {
	mock := &MockBTCWallet{ctrl: ctrl}
	mock.recorder = &MockBTCWalletMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBTCWallet) EXPECT() *MockBTCWalletMockRecorder {
	return m.recorder
}

// FundRawTransaction mocks base method.
func (m *MockBTCWallet) FundRawTransaction(tx *wire.MsgTx, opts btcjson.FundRawTransactionOpts, isWitness *bool) (*btcjson.FundRawTransactionResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FundRawTransaction", tx, opts, isWitness)
	ret0, _ := ret[0].(*btcjson.FundRawTransactionResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FundRawTransaction indicates an expected call of FundRawTransaction.
func (mr *MockBTCWalletMockRecorder) FundRawTransaction(tx, opts, isWitness interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FundRawTransaction", reflect.TypeOf((*MockBTCWallet)(nil).FundRawTransaction), tx, opts, isWitness)
}

// GetBTCConfig mocks base method.
func (m *MockBTCWallet) GetBTCConfig() *config.BTCConfig {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetBTCConfig")
	ret0, _ := ret[0].(*config.BTCConfig)
	return ret0
}

// GetBTCConfig indicates an expected call of GetBTCConfig.
func (mr *MockBTCWalletMockRecorder) GetBTCConfig() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetBTCConfig", reflect.TypeOf((*MockBTCWallet)(nil).GetBTCConfig))
}

// GetHighUTXOAndSum mocks base method.
func (m *MockBTCWallet) GetHighUTXOAndSum() (*btcjson.ListUnspentResult, float64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetHighUTXOAndSum")
	ret0, _ := ret[0].(*btcjson.ListUnspentResult)
	ret1, _ := ret[1].(float64)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetHighUTXOAndSum indicates an expected call of GetHighUTXOAndSum.
func (mr *MockBTCWalletMockRecorder) GetHighUTXOAndSum() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetHighUTXOAndSum", reflect.TypeOf((*MockBTCWallet)(nil).GetHighUTXOAndSum))
}

// GetNetParams mocks base method.
func (m *MockBTCWallet) GetNetParams() *chaincfg.Params {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetNetParams")
	ret0, _ := ret[0].(*chaincfg.Params)
	return ret0
}

// GetNetParams indicates an expected call of GetNetParams.
func (mr *MockBTCWalletMockRecorder) GetNetParams() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetNetParams", reflect.TypeOf((*MockBTCWallet)(nil).GetNetParams))
}

// GetRawChangeAddress mocks base method.
func (m *MockBTCWallet) GetRawChangeAddress(account string) (btcutil.Address, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRawChangeAddress", account)
	ret0, _ := ret[0].(btcutil.Address)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRawChangeAddress indicates an expected call of GetRawChangeAddress.
func (mr *MockBTCWalletMockRecorder) GetRawChangeAddress(account interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRawChangeAddress", reflect.TypeOf((*MockBTCWallet)(nil).GetRawChangeAddress), account)
}

// GetRawTransaction mocks base method.
func (m *MockBTCWallet) GetRawTransaction(txHash *chainhash.Hash) (*btcutil.Tx, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRawTransaction", txHash)
	ret0, _ := ret[0].(*btcutil.Tx)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetRawTransaction indicates an expected call of GetRawTransaction.
func (mr *MockBTCWalletMockRecorder) GetRawTransaction(txHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetRawTransaction", reflect.TypeOf((*MockBTCWallet)(nil).GetRawTransaction), txHash)
}

// GetWalletLockTime mocks base method.
func (m *MockBTCWallet) GetWalletLockTime() int64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetWalletLockTime")
	ret0, _ := ret[0].(int64)
	return ret0
}

// GetWalletLockTime indicates an expected call of GetWalletLockTime.
func (mr *MockBTCWalletMockRecorder) GetWalletLockTime() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetWalletLockTime", reflect.TypeOf((*MockBTCWallet)(nil).GetWalletLockTime))
}

// GetWalletPass mocks base method.
func (m *MockBTCWallet) GetWalletPass() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetWalletPass")
	ret0, _ := ret[0].(string)
	return ret0
}

// GetWalletPass indicates an expected call of GetWalletPass.
func (mr *MockBTCWalletMockRecorder) GetWalletPass() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetWalletPass", reflect.TypeOf((*MockBTCWallet)(nil).GetWalletPass))
}

// ListReceivedByAddress mocks base method.
func (m *MockBTCWallet) ListReceivedByAddress() ([]btcjson.ListReceivedByAddressResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListReceivedByAddress")
	ret0, _ := ret[0].([]btcjson.ListReceivedByAddressResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListReceivedByAddress indicates an expected call of ListReceivedByAddress.
func (mr *MockBTCWalletMockRecorder) ListReceivedByAddress() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListReceivedByAddress", reflect.TypeOf((*MockBTCWallet)(nil).ListReceivedByAddress))
}

// ListUnspent mocks base method.
func (m *MockBTCWallet) ListUnspent() ([]btcjson.ListUnspentResult, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListUnspent")
	ret0, _ := ret[0].([]btcjson.ListUnspentResult)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListUnspent indicates an expected call of ListUnspent.
func (mr *MockBTCWalletMockRecorder) ListUnspent() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListUnspent", reflect.TypeOf((*MockBTCWallet)(nil).ListUnspent))
}

// SendRawTransaction mocks base method.
func (m *MockBTCWallet) SendRawTransaction(tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SendRawTransaction", tx, allowHighFees)
	ret0, _ := ret[0].(*chainhash.Hash)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// SendRawTransaction indicates an expected call of SendRawTransaction.
func (mr *MockBTCWalletMockRecorder) SendRawTransaction(tx, allowHighFees interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendRawTransaction", reflect.TypeOf((*MockBTCWallet)(nil).SendRawTransaction), tx, allowHighFees)
}

// SignRawTransactionWithWallet mocks base method.
func (m *MockBTCWallet) SignRawTransactionWithWallet(tx *wire.MsgTx) (*wire.MsgTx, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SignRawTransactionWithWallet", tx)
	ret0, _ := ret[0].(*wire.MsgTx)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// SignRawTransactionWithWallet indicates an expected call of SignRawTransactionWithWallet.
func (mr *MockBTCWalletMockRecorder) SignRawTransactionWithWallet(tx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SignRawTransactionWithWallet", reflect.TypeOf((*MockBTCWallet)(nil).SignRawTransactionWithWallet), tx)
}

// Stop mocks base method.
func (m *MockBTCWallet) Stop() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Stop")
}

// Stop indicates an expected call of Stop.
func (mr *MockBTCWalletMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockBTCWallet)(nil).Stop))
}

// WalletPassphrase mocks base method.
func (m *MockBTCWallet) WalletPassphrase(passphrase string, timeoutSecs int64) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WalletPassphrase", passphrase, timeoutSecs)
	ret0, _ := ret[0].(error)
	return ret0
}

// WalletPassphrase indicates an expected call of WalletPassphrase.
func (mr *MockBTCWalletMockRecorder) WalletPassphrase(passphrase, timeoutSecs interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WalletPassphrase", reflect.TypeOf((*MockBTCWallet)(nil).WalletPassphrase), passphrase, timeoutSecs)
}
