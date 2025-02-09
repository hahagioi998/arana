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
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

import (
	"github.com/arana-db/arana/pkg/constants/mysql"
	"github.com/arana-db/arana/pkg/mysql/errors"
	"github.com/arana-db/arana/pkg/proto"
)

type ResultSet struct {
	Columns     []proto.Field
	ColumnNames []string
}

type Row struct {
	Content   []byte
	ResultSet *ResultSet
}

type BinaryRow struct {
	Row
}

type TextRow struct {
	Row
}

func (row *Row) Columns() []string {
	if row.ResultSet.ColumnNames != nil {
		return row.ResultSet.ColumnNames
	}

	columns := make([]string, len(row.ResultSet.Columns))
	if row.Content != nil {
		for i := range columns {
			field := row.ResultSet.Columns[i].(*Field)
			if tableName := field.table; len(tableName) > 0 {
				columns[i] = tableName + "." + field.name
			} else {
				columns[i] = field.name
			}
		}
	} else {
		for i := range columns {
			field := row.ResultSet.Columns[i].(*Field)
			columns[i] = field.name
		}
	}

	row.ResultSet.ColumnNames = columns
	return columns
}

func (row *Row) Fields() []proto.Field {
	return row.ResultSet.Columns
}

func (row *Row) Data() []byte {
	return row.Content
}

func (row *Row) Encode(values []*proto.Value, columns []proto.Field, columnNames []string) proto.Row {
	var bf bytes.Buffer
	row.ResultSet = &ResultSet{
		Columns:     columns,
		ColumnNames: columnNames,
	}

	for _, val := range values {
		bf.Write(val.Raw)
	}
	row.Content = bf.Bytes()
	return row
}

func (row *Row) Decode() ([]*proto.Value, error) {
	return nil, nil
}

func (row *Row) GetColumnValue(column string) (interface{}, error) {
	values, err := row.Decode()
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if string(value.Raw) == column {
			return value.Val, nil
		}
	}
	return nil, nil
}

func (rows *TextRow) Decode() ([]*proto.Value, error) {
	dest := make([]*proto.Value, len(rows.ResultSet.Columns))

	// RowSet Packet
	var val []byte
	var isNull bool
	var n int
	var err error
	pos := 0

	for i := 0; i < len(rows.ResultSet.Columns); i++ {
		field := rows.ResultSet.Columns[i].(*Field)

		// Read bytes and convert to string
		val, isNull, n, err = readLengthEncodedString(rows.Content[pos:])
		dest[i] = &proto.Value{
			Typ:   field.fieldType,
			Flags: field.flags,
			Len:   n,
			Val:   val,
			Raw:   val,
		}
		pos += n
		if err == nil {
			if !isNull {
				switch field.fieldType {
				case mysql.FieldTypeTimestamp, mysql.FieldTypeDateTime,
					mysql.FieldTypeDate, mysql.FieldTypeNewDate:
					dest[i].Val, err = parseDateTime(
						val,
						time.Local,
					)
					if err == nil {
						continue
					}
				default:
					continue
				}
			} else {
				dest[i].Val = nil
				continue
			}
		}
		return nil, err // err != nil
	}

	return dest, nil
}

func (rows *BinaryRow) Decode() ([]*proto.Value, error) {
	dest := make([]*proto.Value, len(rows.ResultSet.Columns))

	if rows.Content[0] != mysql.OKPacket {
		return nil, errors.NewSQLError(mysql.CRMalformedPacket, mysql.SSUnknownSQLState, "read binary rows (%v) failed", rows)
	}

	// NULL-bitmap,  [(column-count + 7 + 2) / 8 bytes]
	pos := 1 + (len(dest)+7+2)>>3
	nullMask := rows.Content[1:pos]

	for i := 0; i < len(rows.ResultSet.Columns); i++ {
		// Field is NULL
		// (byte >> bit-pos) % 2 == 1
		if ((nullMask[(i+2)>>3] >> uint((i+2)&7)) & 1) == 1 {
			dest[i] = nil
			continue
		}

		field := rows.ResultSet.Columns[i].(*Field)
		// Convert to byte-coded string
		switch field.fieldType {
		case mysql.FieldTypeNULL:
			dest[i] = &proto.Value{
				Typ:   field.fieldType,
				Flags: field.flags,
				Len:   1,
				Val:   nil,
				Raw:   nil,
			}
			continue

		// Numeric Types
		case mysql.FieldTypeTiny:
			if field.flags&mysql.UnsignedFlag != 0 {
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   1,
					Val:   int64(rows.Content[pos]),
					Raw:   rows.Content[pos : pos+1],
				}
			} else {
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   1,
					Val:   int64(int8(rows.Content[pos])),
					Raw:   rows.Content[pos : pos+1],
				}
			}
			pos++
			continue

		case mysql.FieldTypeShort, mysql.FieldTypeYear:
			if field.flags&mysql.UnsignedFlag != 0 {
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   2,
					Val:   int64(binary.LittleEndian.Uint16(rows.Content[pos : pos+2])),
					Raw:   rows.Content[pos : pos+1],
				}
			} else {
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   2,
					Val:   int64(int16(binary.LittleEndian.Uint16(rows.Content[pos : pos+2]))),
					Raw:   rows.Content[pos : pos+1],
				}
			}
			pos += 2
			continue

		case mysql.FieldTypeInt24, mysql.FieldTypeLong:
			if field.flags&mysql.UnsignedFlag != 0 {
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   4,
					Val:   int64(binary.LittleEndian.Uint32(rows.Content[pos : pos+4])),
					Raw:   rows.Content[pos : pos+4],
				}
			} else {
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   4,
					Val:   int64(int32(binary.LittleEndian.Uint32(rows.Content[pos : pos+4]))),
					Raw:   rows.Content[pos : pos+4],
				}
			}
			pos += 4
			continue

		case mysql.FieldTypeLongLong:
			if field.flags&mysql.UnsignedFlag != 0 {
				val := binary.LittleEndian.Uint64(rows.Content[pos : pos+8])
				if val > math.MaxInt64 {
					dest[i] = &proto.Value{
						Typ:   field.fieldType,
						Flags: field.flags,
						Len:   8,
						Val:   uint64ToString(val),
						Raw:   rows.Content[pos : pos+8],
					}
				} else {
					dest[i] = &proto.Value{
						Typ:   field.fieldType,
						Flags: field.flags,
						Len:   8,
						Val:   int64(val),
						Raw:   rows.Content[pos : pos+8],
					}
				}
			} else {
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   8,
					Val:   int64(binary.LittleEndian.Uint64(rows.Content[pos : pos+8])),
					Raw:   rows.Content[pos : pos+8],
				}
			}
			pos += 8
			continue

		case mysql.FieldTypeFloat:
			dest[i] = &proto.Value{
				Typ:   field.fieldType,
				Flags: field.flags,
				Len:   4,
				Val:   math.Float32frombits(binary.LittleEndian.Uint32(rows.Content[pos : pos+4])),
				Raw:   rows.Content[pos : pos+4],
			}
			pos += 4
			continue

		case mysql.FieldTypeDouble:
			dest[i] = &proto.Value{
				Typ:   field.fieldType,
				Flags: field.flags,
				Len:   8,
				Val:   math.Float64frombits(binary.LittleEndian.Uint64(rows.Content[pos : pos+8])),
				Raw:   rows.Content[pos : pos+8],
			}
			pos += 8
			continue

		// Length coded Binary Strings
		case mysql.FieldTypeDecimal, mysql.FieldTypeNewDecimal, mysql.FieldTypeVarChar,
			mysql.FieldTypeBit, mysql.FieldTypeEnum, mysql.FieldTypeSet, mysql.FieldTypeTinyBLOB,
			mysql.FieldTypeMediumBLOB, mysql.FieldTypeLongBLOB, mysql.FieldTypeBLOB,
			mysql.FieldTypeVarString, mysql.FieldTypeString, mysql.FieldTypeGeometry, mysql.FieldTypeJSON:
			var val interface{}
			var isNull bool
			var n int
			var err error
			val, isNull, n, err = readLengthEncodedString(rows.Content[pos:])
			dest[i] = &proto.Value{
				Typ:   field.fieldType,
				Flags: field.flags,
				Len:   n,
				Val:   val,
				Raw:   rows.Content[pos : pos+n],
			}
			pos += n
			if err == nil {
				if !isNull {
					continue
				} else {
					dest[i].Val = nil
					continue
				}
			}
			return nil, err

		case
			mysql.FieldTypeDate, mysql.FieldTypeNewDate, // Date YYYY-MM-DD
			mysql.FieldTypeTime,                               // Time [-][H]HH:MM:SS[.fractal]
			mysql.FieldTypeTimestamp, mysql.FieldTypeDateTime: // Timestamp YYYY-MM-DD HH:MM:SS[.fractal]

			num, isNull, n := readLengthEncodedInteger(rows.Content[pos:])
			pos += n

			var val interface{}
			var err error
			switch {
			case isNull:
				dest[i] = nil
				continue
			case field.fieldType == mysql.FieldTypeTime:
				// database/sql does not support an equivalent to TIME, return a string
				var dstlen uint8
				switch decimals := field.decimals; decimals {
				case 0x00, 0x1f:
					dstlen = 8
				case 1, 2, 3, 4, 5, 6:
					dstlen = 8 + 1 + decimals
				default:
					return nil, fmt.Errorf(
						"protocol error, illegal decimals architecture.Value %d",
						field.decimals,
					)
				}
				val, err = formatBinaryTime(rows.Content[pos:pos+int(num)], dstlen)
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   n,
					Val:   val,
					Raw:   rows.Content[pos : pos+n],
				}
			default:
				val, err = parseBinaryDateTime(num, rows.Content[pos:], time.Local)
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   n,
					Val:   val,
					Raw:   rows.Content[pos : pos+n],
				}
				if err == nil {
					break
				}

				var dstlen uint8
				if field.fieldType == mysql.FieldTypeDate {
					dstlen = 10
				} else {
					switch decimals := field.decimals; decimals {
					case 0x00, 0x1f:
						dstlen = 19
					case 1, 2, 3, 4, 5, 6:
						dstlen = 19 + 1 + decimals
					default:
						return nil, fmt.Errorf(
							"protocol error, illegal decimals architecture.Value %d",
							field.decimals,
						)
					}
				}
				val, err = formatBinaryDateTime(rows.Content[pos:pos+int(num)], dstlen)
				dest[i] = &proto.Value{
					Typ:   field.fieldType,
					Flags: field.flags,
					Len:   n,
					Val:   val,
					Raw:   rows.Content[pos : pos+n],
				}
			}

			if err == nil {
				pos += int(num)
				continue
			} else {
				return nil, err
			}

		// Please report if this happens!
		default:
			return nil, fmt.Errorf("unknown field type %d", field.fieldType)
		}
	}

	return dest, nil
}
