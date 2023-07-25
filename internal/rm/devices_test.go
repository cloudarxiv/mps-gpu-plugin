/*
 * Copyright 2023 Nebuly.com.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMPSAnnotatedID__Split(t *testing.T) {
	type splitParts struct {
		first  string
		second int
		third  int
	}

	testCases := []struct {
		name          string
		id            MPSAnnotatedID
		expectedParts splitParts
	}{
		{
			name: "Annotated MPS ID with memory and replicas",
			id:   NewMPSAnnotatedID("id-1", 10, 2, "vcore"),
			expectedParts: splitParts{
				first:  "id-1",
				second: 10,
				third:  2,
			},
		},
		{
			name: "Annotated ID, not MPS: should return whole string as first part",
			id:   MPSAnnotatedID(NewAnnotatedID("id-1", 2).String()),
			expectedParts: splitParts{
				first:  NewAnnotatedID("id-1", 2).String(),
				second: 0,
				third:  0,
			},
		},
		{
			name: "Non-annotated ID, should return whole string as first part",
			id:   "non-annotated",
			expectedParts: splitParts{
				first:  "non-annotated",
				second: 0,
				third:  0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			first, second, third, _ := tc.id.Split()
			require.Equal(t, tc.expectedParts.first, first)
			require.Equal(t, tc.expectedParts.second, second)
			require.Equal(t, tc.expectedParts.third, third)
		})
	}
}
