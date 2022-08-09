// Copyright 2022 The Cluster Monitoring Operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"fmt"
	"strings"
)

type State string

const (
	DegradedState    State = "degraded"
	UnavailableState State = "unavailable"
)

// StateError indicates if an operation performed by the client has
// resulted in an invalid state such as Degraded, Unavailable, Unknown
type StateError struct {
	State   State
	Unknown bool
	Reasons []string
}

var _ error = (*StateError)(nil)

func (se StateError) Error() string {
	unknown := ""
	if se.Unknown {
		unknown = " (unknown)"
	}
	return fmt.Sprintf("%s%s: %s", se.State, unknown, strings.Join(se.Reasons, ", "))
}

func NewDegradedError(reason string) *StateError {
	return &StateError{State: DegradedState, Unknown: false, Reasons: []string{reason}}
}

func NewUnavailableError(reason string) *StateError {
	return &StateError{State: UnavailableState, Unknown: false, Reasons: []string{reason}}
}

func NewUnknownStateError(s State, reason string) *StateError {
	return &StateError{State: s, Unknown: true, Reasons: []string{reason}}
}
