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

package mysql

import (
	"testing"
)

import (
	"github.com/stretchr/testify/assert"
)

import (
	"github.com/arana-db/arana/pkg/constants/mysql"
	"github.com/arana-db/arana/pkg/mysql/errors"
)

func TestDSNParse(t *testing.T) {
	dsn := "admin:123456@tcp"
	_, err := ParseDSN(dsn)
	assert.Error(t, err)
	dsnNoAddr := "admin:123456@127.0.0.1:3306/pass"
	_, err = ParseDSN(dsnNoAddr)
	assert.Error(t, err)
}

func TestTcpDSNParse(t *testing.T) {
	dsn := "admin:123456@tcp/pass"
	cfg, err := ParseDSN(dsn)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:3306", cfg.Addr)
	assert.Equal(t, "admin", cfg.User)
	assert.Equal(t, "123456", cfg.Passwd)
	assert.Equal(t, "pass", cfg.DBName)
}

func TestUnixDSNParse(t *testing.T) {
	dsn := "admin:123456@unix/pass"
	cfg, err := ParseDSN(dsn)
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/mysql.sock", cfg.Addr)
	assert.Equal(t, "admin", cfg.User)
	assert.Equal(t, "123456", cfg.Passwd)
	assert.Equal(t, "pass", cfg.DBName)
}

func TestSimpleDSNParse(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass"
	cfg, err := ParseDSN(dsn)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:3306", cfg.Addr)
	assert.Equal(t, "admin", cfg.User)
	assert.Equal(t, "123456", cfg.Passwd)
	assert.Equal(t, "pass", cfg.DBName)

	dsnNoPort := "admin:123456@tcp(127.0.0.1)/pass"
	cfg, err = ParseDSN(dsnNoPort)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:3306", cfg.Addr)
	assert.Equal(t, "admin", cfg.User)
	assert.Equal(t, "123456", cfg.Passwd)
	assert.Equal(t, "pass", cfg.DBName)
}

func TestDSNWithParam(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, err := ParseDSN(dsn)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:3306", cfg.Addr)
	assert.Equal(t, "admin", cfg.User)
	assert.Equal(t, "123456", cfg.Passwd)
	assert.Equal(t, "pass", cfg.DBName)

	assert.True(t, cfg.AllowAllFiles)
	assert.True(t, cfg.AllowCleartextPasswords)

	clone := cfg.Clone()
	assert.Equal(t, "127.0.0.1:3306", clone.Addr)
	assert.Equal(t, "admin", clone.User)
	assert.Equal(t, "123456", clone.Passwd)
	assert.Equal(t, "pass", clone.DBName)
	assert.True(t, clone.AllowAllFiles)
	assert.True(t, clone.AllowCleartextPasswords)
}

func TestParseInitialHandshakePacket(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	data := make([]byte, 4)
	data[0] = mysql.ErrPacket
	data[1] = 400 & 0xff
	data[2] = 400 >> 8
	data[3] = 65
	_, _, _, err := conn.parseInitialHandshakePacket(data)
	assert.Error(t, err)
	assert.Equal(t, "immediate error from server errorCode=400 errorMsg=A", err.(*errors.SQLError).Message)

	data[0] = mysql.ProtocolVersion - 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Error(t, err)

	data = make([]byte, 2)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no server version", err.(*errors.SQLError).Message)

	data = make([]byte, 6)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no connection id", err.(*errors.SQLError).Message)

	data = make([]byte, 14)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no auth-plugin-Content-part-1", err.(*errors.SQLError).Message)

	data = make([]byte, 15)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no filler", err.(*errors.SQLError).Message)

	data = make([]byte, 16)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no capability flags (lower 2 bytes)", err.(*errors.SQLError).Message)

	data = make([]byte, 18)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	length, _, _, _ := conn.parseInitialHandshakePacket(data)
	assert.Equal(t, uint32(0), length)

	data = make([]byte, 19)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no status flags", err.(*errors.SQLError).Message)

	data = make([]byte, 21)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no capability flags (upper 2 bytes)", err.(*errors.SQLError).Message)

	data = make([]byte, 23)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, _, err = conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "parseInitialHandshakePacket: packet has no length of auth-plugin-Content filler", err.(*errors.SQLError).Message)

	data = make([]byte, 24)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	_, _, password, _ := conn.parseInitialHandshakePacket(data)
	assert.Equal(t, mysql.MysqlNativePassword, password)

	data = make([]byte, 37)
	data[0] = mysql.ProtocolVersion
	data[1] = 1
	data[17] = 255
	data[21] = 255
	data[23] = 9
	data[35] = 65
	_, authData, authName, _ := conn.parseInitialHandshakePacket(data)
	assert.Equal(t, "A", authName)
	assert.Equal(t, "\x00\x00\x00\x00\x00\x00\x00\x00", string(authData))
}

func TestSimpleConnAuth(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	data := make([]byte, 1)
	data[0] = 65
	plugin := "caching_sha2_password"
	authResp, err := conn.auth(data, plugin)
	assert.NoError(t, err)
	assert.NotNil(t, authResp)
}

func TestWriteHandshakeResponse41(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	data := make([]byte, 1)
	data[0] = 65
	plugin := "caching_sha2_password"
	authResp, _ := conn.auth(data, plugin)
	assert.NotNil(t, authResp)
	err := conn.writeHandshakeResponse41(0, authResp, plugin)
	assert.NoError(t, err)
	written := conn.c.conn.(*mockConn).written
	length := int(written[0]) + (int(written[1]) << 8) + (int(written[2]) << 16)
	assert.Equal(t, len(written)-4, length)
	flag := 2859525
	flagInWritten := int(written[4]) + (int(written[5]) << 8) + (int(written[6]) << 16) + (int(written[7]) << 24)
	assert.Equal(t, flag, flagInWritten)
	assert.Equal(t, cfg.User, string(written[36:41]))
	assert.Equal(t, authResp, written[43:43+len(authResp)])
	assert.Equal(t, plugin, string(written[43+len(authResp):43+len(authResp)+len(plugin)]))
}

func TestWriteComInitDB(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	err := conn.WriteComInitDB("demo")
	assert.NoError(t, err)
	written := conn.c.conn.(*mockConn).written
	assert.Equal(t, uint8(len("demo")+1), written[0])
	assert.Equal(t, mysql.ComInitDB, int(written[4]))
	assert.Equal(t, "demo", string(written[5:5+len("demo")]))
}

func TestWriteComQuery(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	query := "SELECT 1"
	err := conn.WriteComQuery(query)
	assert.NoError(t, err)
	written := conn.c.conn.(*mockConn).written
	assert.Equal(t, uint8(len(query)+1), written[0])
	assert.Equal(t, mysql.ComQuery, int(written[4]))
	assert.Equal(t, query, string(written[5:5+len(query)]))
}

func TestWriteComSetOption(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	err := conn.WriteComSetOption(1)
	assert.NoError(t, err)
	written := conn.c.conn.(*mockConn).written
	assert.Equal(t, uint8(17), written[0])
	assert.Equal(t, mysql.ComSetOption, int(written[4]))
}

func TestWriteComFieldList(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	table := "demo"
	column := "date"
	err := conn.WriteComFieldList(table, column)
	assert.NoError(t, err)
	written := conn.c.conn.(*mockConn).written
	assert.Equal(t, uint8(1+len(table)+len(column)+2), written[0])
	assert.Equal(t, mysql.ComFieldList, int(written[4]))
	assert.Equal(t, table, string(written[5:5+len(table)]))
	assert.Equal(t, column, string(written[5+len(table)+1:5+len(table)+1+len(column)]))
}

func TestPrepare(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	buf := make([]byte, 13)
	buf[0] = 9
	buf[3] = 1
	buf[4] = mysql.OKPacket
	conn.c.conn.(*mockConn).data = buf
	stmt, err := conn.prepare("SELECT 1")
	assert.NoError(t, err)
	assert.Equal(t, 0, stmt.paramCount)
}

func TestReadComQueryResponse(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	buf := make([]byte, 13)
	buf[0] = 9
	buf[4] = mysql.OKPacket
	buf[5] = 1
	buf[6] = 1
	conn.c.conn.(*mockConn).data = buf
	affectedRows, lastInsertID, _, _, _, err := conn.ReadComQueryResponse()
	assert.NoError(t, err)
	assert.Equal(t, uint64(0x1), affectedRows)
	assert.Equal(t, uint64(0x1), lastInsertID)
}

func TestReadColumnDefinition(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	buf := make([]byte, 100)
	buf[0] = 96
	buf[4] = 3
	buf[5] = 'd'
	buf[6] = 'e'
	buf[7] = 'f'
	buf[8] = 8
	buf[9] = 't'
	buf[10] = 'e'
	buf[11] = 's'
	buf[12] = 't'
	buf[13] = 'b'
	buf[14] = 'a'
	buf[15] = 's'
	buf[16] = 'e'
	buf[17] = 9
	buf[18] = 't'
	buf[19] = 'e'
	buf[20] = 's'
	buf[21] = 't'
	buf[22] = 't'
	buf[23] = 'a'
	buf[24] = 'b'
	buf[25] = 'l'
	buf[26] = 'e'
	buf[28] = 4
	buf[29] = 'n'
	buf[30] = 'a'
	buf[31] = 'm'
	buf[32] = 'e'
	buf[37] = 255
	buf[41] = 15
	buf[45] = 4
	buf[53] = 'u'
	buf[54] = 's'
	buf[55] = 'e'
	buf[56] = 'r'
	conn.c.conn.(*mockConn).data = buf
	field := &Field{}
	err := conn.ReadColumnDefinition(field, 0)
	assert.NoError(t, err)
	assert.Equal(t, "testtable", field.table)
	assert.Equal(t, "testbase", field.database)
	assert.Equal(t, "name", field.name)
	assert.Equal(t, mysql.FieldTypeVarChar, field.fieldType)
	assert.Equal(t, uint64(0x4), field.defaultValueLength)
	assert.Equal(t, "user", string(field.defaultValue))
	assert.Equal(t, uint32(255), field.columnLength)
}

func TestReadColumnDefinitionType(t *testing.T) {
	dsn := "admin:123456@tcp(127.0.0.1:3306)/pass?allowAllFiles=true&allowCleartextPasswords=true"
	cfg, _ := ParseDSN(dsn)
	conn := &BackendConnection{conf: cfg}
	conn.c = newConn(new(mockConn))
	buf := make([]byte, 100)
	buf[0] = 96
	buf[4] = 3
	buf[5] = 'd'
	buf[6] = 'e'
	buf[7] = 'f'
	buf[8] = 8
	buf[9] = 't'
	buf[10] = 'e'
	buf[11] = 's'
	buf[12] = 't'
	buf[13] = 'b'
	buf[14] = 'a'
	buf[15] = 's'
	buf[16] = 'e'
	buf[17] = 9
	buf[18] = 't'
	buf[19] = 'e'
	buf[20] = 's'
	buf[21] = 't'
	buf[22] = 't'
	buf[23] = 'a'
	buf[24] = 'b'
	buf[25] = 'l'
	buf[26] = 'e'
	buf[28] = 4
	buf[29] = 'n'
	buf[30] = 'a'
	buf[31] = 'm'
	buf[32] = 'e'
	buf[37] = 255
	buf[41] = 15
	conn.c.conn.(*mockConn).data = buf
	field := &Field{}
	err := conn.ReadColumnDefinitionType(field, 0)
	assert.NoError(t, err)
	assert.Equal(t, mysql.FieldTypeVarChar, field.fieldType)
}
