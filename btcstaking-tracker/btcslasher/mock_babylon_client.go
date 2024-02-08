// Code generated by MockGen. DO NOT EDIT.
// Source: btcstaking-tracker/btcslasher/expected_babylon_client.go

// Package btcslasher is a generated GoMock package.
package btcslasher

import (
	reflect "reflect"

	types "github.com/babylonchain/babylon/x/btccheckpoint/types"
	types0 "github.com/babylonchain/babylon/x/btcstaking/types"
	types1 "github.com/babylonchain/babylon/x/finality/types"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	query "github.com/cosmos/cosmos-sdk/types/query"
	gomock "github.com/golang/mock/gomock"
)

// MockBabylonQueryClient is a mock of BabylonQueryClient interface.
type MockBabylonQueryClient struct {
	ctrl     *gomock.Controller
	recorder *MockBabylonQueryClientMockRecorder
}

// MockBabylonQueryClientMockRecorder is the mock recorder for MockBabylonQueryClient.
type MockBabylonQueryClientMockRecorder struct {
	mock *MockBabylonQueryClient
}

// NewMockBabylonQueryClient creates a new mock instance.
func NewMockBabylonQueryClient(ctrl *gomock.Controller) *MockBabylonQueryClient {
	mock := &MockBabylonQueryClient{ctrl: ctrl}
	mock.recorder = &MockBabylonQueryClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockBabylonQueryClient) EXPECT() *MockBabylonQueryClientMockRecorder {
	return m.recorder
}

// BTCCheckpointParams mocks base method.
func (m *MockBabylonQueryClient) BTCCheckpointParams() (*types.QueryParamsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BTCCheckpointParams")
	ret0, _ := ret[0].(*types.QueryParamsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BTCCheckpointParams indicates an expected call of BTCCheckpointParams.
func (mr *MockBabylonQueryClientMockRecorder) BTCCheckpointParams() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BTCCheckpointParams", reflect.TypeOf((*MockBabylonQueryClient)(nil).BTCCheckpointParams))
}

// BTCStakingParams mocks base method.
func (m *MockBabylonQueryClient) BTCStakingParams() (*types0.QueryParamsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BTCStakingParams")
	ret0, _ := ret[0].(*types0.QueryParamsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BTCStakingParams indicates an expected call of BTCStakingParams.
func (mr *MockBabylonQueryClientMockRecorder) BTCStakingParams() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BTCStakingParams", reflect.TypeOf((*MockBabylonQueryClient)(nil).BTCStakingParams))
}

// FinalityProviderDelegations mocks base method.
func (m *MockBabylonQueryClient) FinalityProviderDelegations(fpBTCPKHex string, pagination *query.PageRequest) (*types0.QueryFinalityProviderDelegationsResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FinalityProviderDelegations", fpBTCPKHex, pagination)
	ret0, _ := ret[0].(*types0.QueryFinalityProviderDelegationsResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FinalityProviderDelegations indicates an expected call of FinalityProviderDelegations.
func (mr *MockBabylonQueryClientMockRecorder) FinalityProviderDelegations(fpBTCPKHex, pagination interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FinalityProviderDelegations", reflect.TypeOf((*MockBabylonQueryClient)(nil).FinalityProviderDelegations), fpBTCPKHex, pagination)
}

// IsRunning mocks base method.
func (m *MockBabylonQueryClient) IsRunning() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsRunning")
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsRunning indicates an expected call of IsRunning.
func (mr *MockBabylonQueryClientMockRecorder) IsRunning() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsRunning", reflect.TypeOf((*MockBabylonQueryClient)(nil).IsRunning))
}

// ListEvidences mocks base method.
func (m *MockBabylonQueryClient) ListEvidences(startHeight uint64, pagination *query.PageRequest) (*types1.QueryListEvidencesResponse, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListEvidences", startHeight, pagination)
	ret0, _ := ret[0].(*types1.QueryListEvidencesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListEvidences indicates an expected call of ListEvidences.
func (mr *MockBabylonQueryClientMockRecorder) ListEvidences(startHeight, pagination interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListEvidences", reflect.TypeOf((*MockBabylonQueryClient)(nil).ListEvidences), startHeight, pagination)
}

// Subscribe mocks base method.
func (m *MockBabylonQueryClient) Subscribe(subscriber, query string, outCapacity ...int) (<-chan coretypes.ResultEvent, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{subscriber, query}
	for _, a := range outCapacity {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Subscribe", varargs...)
	ret0, _ := ret[0].(<-chan coretypes.ResultEvent)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Subscribe indicates an expected call of Subscribe.
func (mr *MockBabylonQueryClientMockRecorder) Subscribe(subscriber, query interface{}, outCapacity ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{subscriber, query}, outCapacity...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Subscribe", reflect.TypeOf((*MockBabylonQueryClient)(nil).Subscribe), varargs...)
}

// UnsubscribeAll mocks base method.
func (m *MockBabylonQueryClient) UnsubscribeAll(subscriber string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UnsubscribeAll", subscriber)
	ret0, _ := ret[0].(error)
	return ret0
}

// UnsubscribeAll indicates an expected call of UnsubscribeAll.
func (mr *MockBabylonQueryClientMockRecorder) UnsubscribeAll(subscriber interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UnsubscribeAll", reflect.TypeOf((*MockBabylonQueryClient)(nil).UnsubscribeAll), subscriber)
}