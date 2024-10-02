// Code generated by MockGen. DO NOT EDIT.
// Source: btcstaking-tracker/stakingeventwatcher/expected_babylon_client.go

// Package stakingeventwatcher is a generated GoMock package.
package stakingeventwatcher

import (
	context "context"
	reflect "reflect"

	types "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	schnorr "github.com/btcsuite/btcd/btcec/v2/schnorr"
	chainhash "github.com/btcsuite/btcd/chaincfg/chainhash"
	gomock "github.com/golang/mock/gomock"
)

// MockBabylonNodeAdapter is a mock of BabylonNodeAdapter interface.
type MockBabylonNodeAdapter struct {
	ctrl     *gomock.Controller
	recorder *MockBabylonNodeAdapterMockRecorder
}

// MockBabylonNodeAdapterMockRecorder is the mock recorder for MockBabylonNodeAdapter.
type MockBabylonNodeAdapterMockRecorder struct {
	mock *MockBabylonNodeAdapter
}

// NewMockBabylonNodeAdapter creates a new mock instance.
func NewMockBabylonNodeAdapter(ctrl *gomock.Controller) *MockBabylonNodeAdapter {
	mock := &MockBabylonNodeAdapter{ctrl: ctrl}
	mock.recorder = &MockBabylonNodeAdapterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBabylonNodeAdapter) EXPECT() *MockBabylonNodeAdapterMockRecorder {
	return m.recorder
}

// ActivateDelegation mocks base method.
func (m *MockBabylonNodeAdapter) ActivateDelegation(ctx context.Context, stakingTxHash chainhash.Hash, proof *types.BTCSpvProof) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ActivateDelegation", ctx, stakingTxHash, proof)
	ret0, _ := ret[0].(error)
	return ret0
}

// ActivateDelegation indicates an expected call of ActivateDelegation.
func (mr *MockBabylonNodeAdapterMockRecorder) ActivateDelegation(ctx, stakingTxHash, proof interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ActivateDelegation", reflect.TypeOf((*MockBabylonNodeAdapter)(nil).ActivateDelegation), ctx, stakingTxHash, proof)
}

// BtcClientTipHeight mocks base method.
func (m *MockBabylonNodeAdapter) BtcClientTipHeight() (uint32, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BtcClientTipHeight")
	ret0, _ := ret[0].(uint32)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BtcClientTipHeight indicates an expected call of BtcClientTipHeight.
func (mr *MockBabylonNodeAdapterMockRecorder) BtcClientTipHeight() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BtcClientTipHeight", reflect.TypeOf((*MockBabylonNodeAdapter)(nil).BtcClientTipHeight))
}

// BtcDelegations mocks base method.
func (m *MockBabylonNodeAdapter) BtcDelegations(offset, limit uint64) ([]Delegation, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BtcDelegations", offset, limit)
	ret0, _ := ret[0].([]Delegation)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BtcDelegations indicates an expected call of BtcDelegations.
func (mr *MockBabylonNodeAdapterMockRecorder) BtcDelegations(offset, limit interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BtcDelegations", reflect.TypeOf((*MockBabylonNodeAdapter)(nil).BtcDelegations), offset, limit)
}

// IsDelegationActive mocks base method.
func (m *MockBabylonNodeAdapter) IsDelegationActive(stakingTxHash chainhash.Hash) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsDelegationActive", stakingTxHash)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// IsDelegationActive indicates an expected call of IsDelegationActive.
func (mr *MockBabylonNodeAdapterMockRecorder) IsDelegationActive(stakingTxHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsDelegationActive", reflect.TypeOf((*MockBabylonNodeAdapter)(nil).IsDelegationActive), stakingTxHash)
}

// IsDelegationVerified mocks base method.
func (m *MockBabylonNodeAdapter) IsDelegationVerified(stakingTxHash chainhash.Hash) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsDelegationVerified", stakingTxHash)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// IsDelegationVerified indicates an expected call of IsDelegationVerified.
func (mr *MockBabylonNodeAdapterMockRecorder) IsDelegationVerified(stakingTxHash interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsDelegationVerified", reflect.TypeOf((*MockBabylonNodeAdapter)(nil).IsDelegationVerified), stakingTxHash)
}

// ReportUnbonding mocks base method.
func (m *MockBabylonNodeAdapter) ReportUnbonding(ctx context.Context, stakingTxHash chainhash.Hash, stakerUnbondingSig *schnorr.Signature) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReportUnbonding", ctx, stakingTxHash, stakerUnbondingSig)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReportUnbonding indicates an expected call of ReportUnbonding.
func (mr *MockBabylonNodeAdapterMockRecorder) ReportUnbonding(ctx, stakingTxHash, stakerUnbondingSig interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportUnbonding", reflect.TypeOf((*MockBabylonNodeAdapter)(nil).ReportUnbonding), ctx, stakingTxHash, stakerUnbondingSig)
}
