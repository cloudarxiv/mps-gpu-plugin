package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xzaviourr/k8s-device-plugin/internal/rm"
)

func TestMPSDeviceList_Subset(t *testing.T) {
	testCases := []struct {
		description string
		list        MPSDeviceList
		ids         []string
		expected    MPSDeviceList
	}{
		{
			description: "empty list, empty ids",
			list:        MPSDeviceList{},
			ids:         []string{},
			expected:    MPSDeviceList{},
		},
		{
			description: "empty list: subset should be empty",
			list:        MPSDeviceList{},
			ids: []string{
				rm.NewAnnotatedID("id-1", 0).String(),
				rm.NewAnnotatedID("id-2", 0).String(),
			},
			expected: MPSDeviceList{},
		},
		{
			description: "subset should return only MPSDevices with provided IDs",
			list: MPSDeviceList{
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-1", 1, 0, "vcore"),
					Index:       "1",
				},
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-2", 2, 0, "vcore"),
					Index:       "2",
				},
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-3", 3, 0, "vcore"),
					Index:       "3",
				},
			},
			ids: []string{
				rm.NewMPSAnnotatedID("id-1", 1, 0, "vcore").String(),
				rm.NewMPSAnnotatedID("id-2", 2, 0, "vcore").String(),
			},
			expected: MPSDeviceList{
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-1", 1, 0, "vcore"),
					Index:       "1",
				},
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-2", 2, 0, "vcore"),
					Index:       "2",
				},
			},
		},
		{
			description: "subset should consider only annotated IDs",
			list: MPSDeviceList{
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-1", 1, 0, "vcore"),
					Index:       "1",
				},
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-2", 2, 0, "vcore"),
					Index:       "2",
				},
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-3", 3, 0, "vcore"),
					Index:       "3",
				},
			},
			ids: []string{
				rm.NewMPSAnnotatedID("id-1", 1, 0, "vcore").String(),
				"id-2",
			},
			expected: MPSDeviceList{
				{
					AnnotatedID: rm.NewMPSAnnotatedID("id-1", 1, 0, "vcore"),
					Index:       "1",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			res := tc.list.Subset(tc.ids)
			require.ElementsMatch(t, tc.expected, res)
		})
	}
}
