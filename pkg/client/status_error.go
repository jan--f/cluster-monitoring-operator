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
)

type StatusError struct {
	status    string
	reason    string
	unkown    bool
	prevError *StatusError
}

func (se StatusError) Error() string {
	return fmt.Sprintf("Status: %s: %s", se.status, se.reason)
}

func (se StatusError) Unwrap() error {
	return se.prevError
}

func (se *StatusError) Append(err StatusError) StatusError {
	if se.prevError == nil {
		se.prevError = &err
		return *se
	}
	return se.prevError.Append(err)
}
