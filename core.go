package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/pingcap/tidb/pkg/ddl"
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	"github.com/pingcap/tidb/pkg/store/mockstore"
	"github.com/pingcap/tidb/pkg/table"
	"github.com/pingcap/tidb/pkg/table/tables"
	"github.com/pingcap/tidb/pkg/types"
	"github.com/pingcap/tidb/pkg/util/mock"
)

type Core struct {
	ctx       *mock.Context
	model     *ast.CreateTableStmt
	tbl       table.Table
	colAllocs []*Allocator
}

func NewCore(schema string) (*Core, error) {
	p := parser.New()
	stmtNodes, _, err := p.Parse(schema, "", "")
	if err != nil {
		return nil, err
	}
	if len(stmtNodes) != 1 {
		return nil, fmt.Errorf("Unexpected count of SQL statements %d", len(stmtNodes))
	}
	createTableNode, ok := stmtNodes[0].(*ast.CreateTableStmt)
	if !ok {
		return nil, errors.New("Only support create table")
	}
	ctx := mock.NewContext()
	store, err := mockstore.NewMockStore()
	if err != nil {
		return nil, err
	}
	ctx.Store = store
	tbl, err := ddl.MockTableInfo(ctx, createTableNode, 1)
	if err != nil {
		return nil, err
	}
	core := &Core{
		ctx:       ctx,
		model:     createTableNode,
		tbl:       tables.MockTableFromMeta(tbl),
		colAllocs: make([]*Allocator, 0, len(createTableNode.Cols)),
	}
	for _, col := range createTableNode.Cols {
		core.colAllocs = append(core.colAllocs, NewAllocator(col))
	}
	return core, nil
}

func (c *Core) InsertRows(n, sample int) (int, error) {
	if err := c.ctx.NewTxn(context.Background()); err != nil {
		return 0, err
	}
	txn, err := c.ctx.Txn(true)
	if err != nil {
		return 0, err
	}
	sampleRows := n / sample
	start := time.Now()
	for i := 0; i < sampleRows; i++ {
		row := c.GetRow()
		opts := []table.AddRecordOption{}
		if _, err := c.tbl.AddRecord(c.ctx, row, opts...); err != nil {
			return 0, err
		}
	}
	fmt.Printf("sample %d lines cost %s\n", sampleRows, time.Since(start))
	membuf := txn.GetMemBuffer()
	return (membuf.Size() / sampleRows) * n, nil
}

func (c *Core) DeleteRows(n, sample int) (int, error) {
	return 0, nil
}

func (c *Core) GetRow() []types.Datum {
	datums := make([]types.Datum, 0, len(c.model.Cols))
	for _, alloc := range c.colAllocs {
		datums = append(datums, alloc.NewDatum())
	}
	return datums
}

type Allocator struct {
	col           *ast.ColumnDef
	uintAlloc     uint64
	intAlloc      int64
	floatAlloc    float64
	bytesAlloc    []byte
	timeAlloc     time.Time
	durationAlloc time.Duration
}

func NewAllocator(col *ast.ColumnDef) *Allocator {
	a := &Allocator{
		col:       col,
		timeAlloc: time.Unix(0, 0),
	}
	ft := col.Tp
	switch ft.GetType() {
	case mysql.TypeString, mysql.TypeVarString, mysql.TypeVarchar, mysql.TypeBlob, mysql.TypeTinyBlob, mysql.TypeMediumBlob, mysql.TypeLongBlob:
		flen := ft.GetFlen()
		if flen > 0 {
			a.bytesAlloc = make([]byte, flen)
		} else {
			a.bytesAlloc = make([]byte, 1)
		}
	}
	return a
}

func (a *Allocator) NewDatum() types.Datum {
	ft := a.col.Tp
	switch ft.GetType() {
	case mysql.TypeTiny, mysql.TypeShort, mysql.TypeInt24, mysql.TypeLong, mysql.TypeLonglong:
		if mysql.HasUnsignedFlag(ft.GetFlag()) {
			a.uintAlloc = a.uintAlloc + 1
			return types.NewUintDatum(a.uintAlloc)
		} else {
			a.intAlloc = a.intAlloc + 1
			minus := a.intAlloc%2 == 0
			if minus {
				return types.NewIntDatum(-a.intAlloc)
			}
			return types.NewIntDatum(a.intAlloc)
		}
	case mysql.TypeFloat, mysql.TypeDouble:
		a.floatAlloc += rand.Float64()
		return types.NewFloat64Datum(a.floatAlloc)
	case mysql.TypeString, mysql.TypeVarString, mysql.TypeVarchar, mysql.TypeBlob, mysql.TypeTinyBlob, mysql.TypeMediumBlob, mysql.TypeLongBlob:
		growBytes(a.bytesAlloc)
		bytes := make([]byte, len(a.bytesAlloc))
		copy(bytes, a.bytesAlloc)
		return types.NewBytesDatum(a.bytesAlloc)
	case mysql.TypeNewDecimal:
		a.floatAlloc += rand.Float64()
		var decimal types.MyDecimal
		MustNil(decimal.FromFloat64(a.floatAlloc))
		return types.NewDecimalDatum(&decimal)
	case mysql.TypeDuration:
		a.durationAlloc += time.Second
		return types.NewDurationDatum(types.Duration{Duration: a.durationAlloc})
	case mysql.TypeDate, mysql.TypeDatetime, mysql.TypeTimestamp:
		a.timeAlloc = a.timeAlloc.Add(time.Second)
		t := types.NewTime(types.FromGoTime(a.timeAlloc), ft.GetType(), 0)
		return types.NewTimeDatum(t)
	default:
		info := fmt.Sprintf("unknown type %v", ft.GetType())
		panic(info)
	}
}

func MustNil(i interface{}) {
	if i != nil {
		info := fmt.Sprintf("%+v not nil", i)
		panic(info)
	}
}

func growBytes(bytes []byte) {
	for i := len(bytes) - 1; i >= 0; i-- {
		if bytes[i] < 255 {
			bytes[i]++
			return
		}
		bytes[i] = 0
	}
}
