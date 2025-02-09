/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package plan

import (
	"context"
)

import (
	"github.com/arana-db/arana/pkg/mysql"
	"github.com/arana-db/arana/pkg/proto"
)

var _ proto.Plan = (*AlwaysEmptyExecPlan)(nil)

var _emptyResult mysql.Result

// AlwaysEmptyExecPlan represents an exec plan which affects nothing.
type AlwaysEmptyExecPlan struct {
}

func (a AlwaysEmptyExecPlan) Type() proto.PlanType {
	return proto.PlanTypeExec
}

func (a AlwaysEmptyExecPlan) ExecIn(ctx context.Context, conn proto.VConn) (proto.Result, error) {
	return &_emptyResult, nil
}
