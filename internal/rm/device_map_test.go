/**
# Copyright (c) 2022, NVIDIA CORPORATION.  All rights reserved.
# Copyright 2023 Nebuly.ai.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package rm

import (
	"testing"

	spec "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
	"github.com/stretchr/testify/require"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	mockedUUIDOne = "GPU-b1028956-cfa2-0990-bf4a-5da9abb51763"
	mockedUUIDTwo = "GPU-4cf8db2d-06c0-7d70-1a51-e59b25b2c16c"
)

func TestDeviceMapInsert(t *testing.T) {
	device0 := Device{Device: pluginapi.Device{ID: "0"}}
	device0withIndex := Device{Device: pluginapi.Device{ID: "0"}, Index: "index"}
	device1 := Device{Device: pluginapi.Device{ID: "1"}}

	testCases := []struct {
		description       string
		deviceMap         DeviceMap
		key               string
		value             *Device
		expectedDeviceMap DeviceMap
	}{
		{
			description: "insert into empty map",
			deviceMap:   make(DeviceMap),
			key:         "resource",
			value:       &device0,
			expectedDeviceMap: DeviceMap{
				"resource": Devices{
					"0": &device0,
				},
			},
		},
		{
			description: "add to existing resource",
			deviceMap: DeviceMap{
				"resource": Devices{
					"0": &device0,
				},
			},
			key:   "resource",
			value: &device1,
			expectedDeviceMap: DeviceMap{
				"resource": Devices{
					"0": &device0,
					"1": &device1,
				},
			},
		},
		{
			description: "add new resource",
			deviceMap: DeviceMap{
				"resource": Devices{
					"0": &device0,
				},
			},
			key:   "resource1",
			value: &device0,
			expectedDeviceMap: DeviceMap{
				"resource": Devices{
					"0": &device0,
				},
				"resource1": Devices{
					"0": &device0,
				},
			},
		},
		{
			description: "overwrite existing device",
			deviceMap: DeviceMap{
				"resource": Devices{
					"0": &device0,
				},
			},
			key:   "resource",
			value: &device0withIndex,
			expectedDeviceMap: DeviceMap{
				"resource": Devices{
					"0": &device0withIndex,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			tc.deviceMap.insert(spec.ResourceName(tc.key), tc.value)

			require.EqualValues(t, tc.expectedDeviceMap, tc.deviceMap)
		})
	}
}

func TestAddMPSReplicas(t *testing.T) {
	testCases := []struct {
		description       string
		config            *spec.Config
		deviceMap         DeviceMap
		expectedDeviceMap DeviceMap
		expectedErr       bool
	}{
		{
			description:       "DeviceMap is empty",
			config:            &spec.Config{},
			deviceMap:         make(DeviceMap),
			expectedDeviceMap: make(DeviceMap),
			expectedErr:       false,
		},
		{
			description: "Config has no MPS devices, should not add replicas",
			config:      &spec.Config{},
			deviceMap: map[spec.ResourceName]Devices{
				"nvidia.com/gpu": {
					"0": &Device{
						Device: pluginapi.Device{ID: "id-0"},
						Paths:  []string{},
						Index:  "0",
					},
					"1": &Device{
						Device: pluginapi.Device{ID: "id-1"},
						Paths:  []string{},
						Index:  "1",
					},
				},
			},
			expectedDeviceMap: map[spec.ResourceName]Devices{
				"nvidia.com/gpu": {
					"0": &Device{
						Device: pluginapi.Device{ID: "id-0"},
						Paths:  []string{},
						Index:  "0",
					},
					"1": &Device{
						Device: pluginapi.Device{ID: "id-1"},
						Paths:  []string{},
						Index:  "1",
					},
				},
			},
			expectedErr: false,
		},
		{
			description: "For device with MPS replicas, result map should include only annotated device IDs",
			config: &spec.Config{
				Sharing: spec.Sharing{
					MPS: spec.MPS{
						Resources: []spec.MPSResource{
							{
								Name:     "nvidia.com/gpu",
								MemoryGB: 20,
								Replicas: 2,
								Devices:  []spec.ReplicatedDeviceRef{mockedUUIDOne},
							},
						},
					},
				},
			},
			deviceMap: map[spec.ResourceName]Devices{
				"nvidia.com/gpu": {
					mockedUUIDOne: &Device{
						Device: pluginapi.Device{ID: mockedUUIDOne},
						Paths:  []string{},
						Index:  "0",
					},
					mockedUUIDTwo: &Device{
						Device: pluginapi.Device{ID: mockedUUIDTwo},
						Paths:  []string{},
						Index:  "1",
					},
				},
			},
			expectedErr: false,
			expectedDeviceMap: map[spec.ResourceName]Devices{
				"nvidia.com/gpu": {
					NewMPSAnnotatedID(mockedUUIDOne, 20, 0).String(): &Device{
						Device: pluginapi.Device{
							ID:       NewMPSAnnotatedID(mockedUUIDOne, 20, 0).String(),
							Health:   "",
							Topology: nil,
						},
						Paths: []string{},
						Index: "0",
					},
					NewMPSAnnotatedID(mockedUUIDOne, 20, 1).String(): &Device{
						Device: pluginapi.Device{
							ID:       NewMPSAnnotatedID(mockedUUIDOne, 20, 1).String(),
							Health:   "",
							Topology: nil,
						},
						Paths: []string{},
						Index: "0",
					},
					mockedUUIDTwo: &Device{
						Device: pluginapi.Device{
							ID: mockedUUIDTwo,
						},
						Paths: []string{},
						Index: "1",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			res, err := addMPSReplicas(tc.config, tc.deviceMap)
			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, tc.expectedDeviceMap, res)
			}
		})
	}
}
