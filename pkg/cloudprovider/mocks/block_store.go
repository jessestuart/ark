/*
Copyright 2018 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by mockery v1.0.0. DO NOT EDIT.
package mocks

import mock "github.com/stretchr/testify/mock"
import runtime "k8s.io/apimachinery/pkg/runtime"

// BlockStore is an autogenerated mock type for the BlockStore type
type BlockStore struct {
	mock.Mock
}

// CreateSnapshot provides a mock function with given fields: volumeID, volumeAZ, tags
func (_m *BlockStore) CreateSnapshot(volumeID string, volumeAZ string, tags map[string]string) (string, error) {
	ret := _m.Called(volumeID, volumeAZ, tags)

	var r0 string
	if rf, ok := ret.Get(0).(func(string, string, map[string]string) string); ok {
		r0 = rf(volumeID, volumeAZ, tags)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, map[string]string) error); ok {
		r1 = rf(volumeID, volumeAZ, tags)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// CreateVolumeFromSnapshot provides a mock function with given fields: snapshotID, volumeType, volumeAZ, iops
func (_m *BlockStore) CreateVolumeFromSnapshot(snapshotID string, volumeType string, volumeAZ string, iops *int64) (string, error) {
	ret := _m.Called(snapshotID, volumeType, volumeAZ, iops)

	var r0 string
	if rf, ok := ret.Get(0).(func(string, string, string, *int64) string); ok {
		r0 = rf(snapshotID, volumeType, volumeAZ, iops)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, string, *int64) error); ok {
		r1 = rf(snapshotID, volumeType, volumeAZ, iops)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteSnapshot provides a mock function with given fields: snapshotID
func (_m *BlockStore) DeleteSnapshot(snapshotID string) error {
	ret := _m.Called(snapshotID)

	var r0 error
	if rf, ok := ret.Get(0).(func(string) error); ok {
		r0 = rf(snapshotID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetVolumeID provides a mock function with given fields: pv
func (_m *BlockStore) GetVolumeID(pv runtime.Unstructured) (string, error) {
	ret := _m.Called(pv)

	var r0 string
	if rf, ok := ret.Get(0).(func(runtime.Unstructured) string); ok {
		r0 = rf(pv)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(runtime.Unstructured) error); ok {
		r1 = rf(pv)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetVolumeInfo provides a mock function with given fields: volumeID, volumeAZ
func (_m *BlockStore) GetVolumeInfo(volumeID string, volumeAZ string) (string, *int64, error) {
	ret := _m.Called(volumeID, volumeAZ)

	var r0 string
	if rf, ok := ret.Get(0).(func(string, string) string); ok {
		r0 = rf(volumeID, volumeAZ)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 *int64
	if rf, ok := ret.Get(1).(func(string, string) *int64); ok {
		r1 = rf(volumeID, volumeAZ)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*int64)
		}
	}

	var r2 error
	if rf, ok := ret.Get(2).(func(string, string) error); ok {
		r2 = rf(volumeID, volumeAZ)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// Init provides a mock function with given fields: config
func (_m *BlockStore) Init(config map[string]string) error {
	ret := _m.Called(config)

	var r0 error
	if rf, ok := ret.Get(0).(func(map[string]string) error); ok {
		r0 = rf(config)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetVolumeID provides a mock function with given fields: pv, volumeID
func (_m *BlockStore) SetVolumeID(pv runtime.Unstructured, volumeID string) (runtime.Unstructured, error) {
	ret := _m.Called(pv, volumeID)

	var r0 runtime.Unstructured
	if rf, ok := ret.Get(0).(func(runtime.Unstructured, string) runtime.Unstructured); ok {
		r0 = rf(pv, volumeID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(runtime.Unstructured)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(runtime.Unstructured, string) error); ok {
		r1 = rf(pv, volumeID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
