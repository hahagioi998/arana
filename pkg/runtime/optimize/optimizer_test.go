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

package optimize

import (
	"context"
	"strings"
	"testing"
)

import (
	"github.com/arana-db/parser"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/assert"
)

import (
	"github.com/arana-db/arana/pkg/mysql"
	"github.com/arana-db/arana/pkg/proto"
	rcontext "github.com/arana-db/arana/pkg/runtime/context"
	"github.com/arana-db/arana/testdata"
)

func TestOptimizer_OptimizeSelect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := testdata.NewMockVConn(ctrl)

	conn.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, db string, sql string, args ...interface{}) (proto.Result, error) {
			t.Logf("fake query: db=%s, sql=%s, args=%v\n", db, sql, args)
			return nil, nil
		}).
		AnyTimes()

	var (
		sql  = "select id, uid from student where uid in (?,?,?)"
		ctx  = context.Background()
		rule = makeFakeRule(ctrl, 8)
		opt  optimizer
	)

	p := parser.New()
	stmt, _ := p.ParseOneStmt(sql, "", "")

	plan, err := opt.Optimize(rcontext.WithRule(ctx, rule), conn, stmt, 1, 2, 3)
	assert.NoError(t, err)

	_, _ = plan.ExecIn(ctx, conn)
}

func TestOptimizer_OptimizeInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	conn := testdata.NewMockVConn(ctrl)

	var fakeId uint64

	conn.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, db string, sql string, args ...interface{}) (proto.Result, error) {
			t.Logf("fake exec: db='%s', sql=\"%s\", args=%v\n", db, sql, args)
			fakeId++

			return &mysql.Result{
				AffectedRows: uint64(strings.Count(sql, "?")),
				InsertId:     fakeId,
			}, nil
		}).
		AnyTimes()

	var (
		ctx  = context.Background()
		rule = makeFakeRule(ctrl, 8)
		opt  optimizer
	)

	t.Run("sharding", func(t *testing.T) {
		sql := "insert into student(name,uid,age) values('foo',?,18),('bar',?,19),('qux',?,17)"

		p := parser.New()
		stmt, _ := p.ParseOneStmt(sql, "", "")

		plan, err := opt.Optimize(rcontext.WithRule(ctx, rule), conn, stmt, 8, 9, 16) // 8,16 -> fake_db_0000, 9 -> fake_db_0001
		assert.NoError(t, err)

		res, err := plan.ExecIn(ctx, conn)
		assert.NoError(t, err)

		affected, _ := res.RowsAffected()
		assert.Equal(t, uint64(3), affected)
		lastInsertId, _ := res.LastInsertId()
		assert.Equal(t, fakeId, lastInsertId)
	})

	t.Run("non-sharding", func(t *testing.T) {
		sql := "insert into abc set name='foo',uid=?,age=18"

		p := parser.New()
		stmt, _ := p.ParseOneStmt(sql, "", "")

		plan, err := opt.Optimize(rcontext.WithRule(ctx, rule), conn, stmt, 1)
		assert.NoError(t, err)

		res, err := plan.ExecIn(ctx, conn)
		assert.NoError(t, err)

		affected, _ := res.RowsAffected()
		assert.Equal(t, uint64(1), affected)
		lastInsertId, _ := res.LastInsertId()
		assert.Equal(t, fakeId, lastInsertId)
	})

}
