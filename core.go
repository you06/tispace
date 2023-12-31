package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/pingcap/tidb/pkg/ddl"
	"github.com/pingcap/tidb/pkg/kv"
	"github.com/pingcap/tidb/pkg/meta/autoid"
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/mysql"
	"github.com/pingcap/tidb/pkg/planner/core"
	"github.com/pingcap/tidb/pkg/store/mockstore"
	"github.com/pingcap/tidb/pkg/table"
	"github.com/pingcap/tidb/pkg/table/tables"
	"github.com/pingcap/tidb/pkg/types"
	"github.com/pingcap/tidb/pkg/util/mock"
)

type Core struct {
	ctx        *mock.Context
	model      *ast.CreateTableStmt
	tbl        table.Table
	colAllocs  []*Allocator
	dropKey    bool
	dropValue  bool
	handleCols core.HandleCols
}

func NewCore(schema string, dropKey, dropValue bool) (*Core, error) {
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
	sessVars := ctx.GetSessionVars()
	sessVars.IDAllocator = autoid.NewAllocatorFromTempTblInfo(tbl)
	sessVars.MemTracker.SetBytesLimit(64 << 30)
	kv.TxnTotalSizeLimit.Store(uint64(64 << 30))
	core := &Core{
		ctx:       ctx,
		model:     createTableNode,
		tbl:       tables.MockTableFromMeta(tbl),
		colAllocs: make([]*Allocator, 0, len(createTableNode.Cols)),
		dropKey:   dropKey,
		dropValue: dropValue,
	}
	for _, col := range createTableNode.Cols {
		core.colAllocs = append(core.colAllocs, NewAllocator(col))
	}
	return core, nil
}

func (c *Core) Context() context.Context {
	ctx := context.Background()
	if c.dropKey {
		ctx = context.WithValue(ctx, "dropKVMemBuffer", "key")
	} else if c.dropValue {
		ctx = context.WithValue(ctx, "dropKVMemBuffer", "value")
	}
	return ctx
}

func (c *Core) InsertRows(n, sample int) (int, error) {
	if err := c.ctx.NewTxn(c.Context()); err != nil {
		return 0, err
	}
	txn, err := c.ctx.Txn(true)
	if err != nil {
		return 0, err
	}
	sampleRows := n / sample
	stage := NewMemStage()
	start := time.Now()
	for i := 0; i < sampleRows; i++ {
		row := c.GetRow()
		opts := []table.AddRecordOption{}
		if _, err := c.tbl.AddRecord(c.ctx, row, opts...); err != nil {
			return 0, err
		}
	}
	fmt.Printf("sample %d lines cost %s, %s per row\n", sampleRows, time.Since(start), time.Since(start)/time.Duration(sampleRows))
	membuf := txn.GetMemBuffer()
	diff := stage.Diff()
	diff.CompareWithMemBuffer("insert", membuf, n, sampleRows)
	runtime.GC()
	time.Sleep(time.Second)
	diff = stage.Diff()
	diff.CompareWithMemBuffer("insert-gc", membuf, n, sampleRows)
	return (membuf.Size() / sampleRows) * n, nil
}

func (c *Core) prepareRows(count int) ([]kv.Handle, [][]types.Datum, error) {
	if err := c.ctx.NewTxn(context.Background()); err != nil {
		return nil, nil, err
	}
	handles := make([]kv.Handle, 0, count)
	rows := make([][]types.Datum, 0, count)
	for i := 0; i < count; i++ {
		row := c.GetRow()
		rows = append(rows, row)
		handle, err := c.tbl.AddRecord(c.ctx, row)
		if err != nil {
			return nil, nil, err
		}
		handles = append(handles, handle)
	}
	if err := c.ctx.CommitTxn(context.Background()); err != nil {
		return nil, nil, err
	}
	return handles, rows, nil
}

func (c *Core) UpdateRows(n, sample int) (int, error) {
	sampleRows := n / sample
	handles, befores, err := c.prepareRows(sampleRows)
	if err != nil {
		return 0, err
	}
	if err := c.ctx.NewTxn(c.Context()); err != nil {
		return 0, err
	}
	txn, err := c.ctx.Txn(true)
	if err != nil {
		return 0, err
	}
	stage := NewMemStage()
	start := time.Now()
	for i := 0; i < sampleRows; i++ {
		after := c.GetRow()
		before := befores[i]
		handle := handles[i]
		touched := make([]bool, len(c.model.Cols))
		for j := 0; j < len(touched); j++ {
			touched[j] = true
		}
		if err := c.tbl.UpdateRecord(context.Background(), c.ctx, handle, before, after, touched); err != nil {
			return 0, err
		}
	}
	fmt.Printf("sample %d lines cost %s, %s per row\n", sampleRows, time.Since(start), time.Since(start)/time.Duration(sampleRows))
	membuf := txn.GetMemBuffer()
	diff := stage.Diff()
	diff.CompareWithMemBuffer("update", membuf, n, sampleRows)
	runtime.GC()
	time.Sleep(time.Second)
	diff = stage.Diff()
	diff.CompareWithMemBuffer("update-gc", membuf, n, sampleRows)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(befores)
	return (membuf.Size() / sampleRows) * n, nil
}

func (c *Core) DeleteRows(n, sample int) (int, error) {
	sampleRows := n / sample
	handles, befores, err := c.prepareRows(sampleRows)
	if err != nil {
		return 0, err
	}
	if err := c.ctx.NewTxn(c.Context()); err != nil {
		return 0, err
	}
	txn, err := c.ctx.Txn(true)
	if err != nil {
		return 0, err
	}
	stage := NewMemStage()
	start := time.Now()
	for i := 0; i < sampleRows; i++ {
		before := befores[i]
		handle := handles[i]
		touched := make([]bool, len(c.model.Cols))
		for j := 0; j < len(touched); j++ {
			touched[j] = true
		}
		if err := c.tbl.RemoveRecord(c.ctx, handle, before); err != nil {
			return 0, err
		}
	}
	fmt.Printf("sample %d lines cost %s, %s per row\n", sampleRows, time.Since(start), time.Since(start)/time.Duration(sampleRows))
	membuf := txn.GetMemBuffer()
	diff := stage.Diff()
	diff.CompareWithMemBuffer("delete", membuf, n, sampleRows)
	runtime.GC()
	time.Sleep(time.Second)
	diff = stage.Diff()
	diff.CompareWithMemBuffer("delete-gc", membuf, n, sampleRows)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(befores)
	return (membuf.Size() / sampleRows) * n, nil
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

type MemStage struct {
	Stats      runtime.MemStats
	ProcessMem uint64
}

func NewMemStage() MemStage {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return MemStage{
		Stats:      m,
		ProcessMem: GetMemByPid(),
	}
}

type MemDiff struct {
	Alloc   uint64
	Sys     uint64
	Process uint64
}

func (m *MemStage) Diff() MemDiff {
	now := NewMemStage()
	return MemDiff{
		Alloc:   now.Stats.Alloc - m.Stats.Alloc,
		Sys:     now.Stats.Sys - m.Stats.Sys,
		Process: now.ProcessMem - m.ProcessMem,
	}
}

func (m *MemDiff) SampleToTotal(n, sampleRows int) MemDiff {
	return MemDiff{
		Alloc:   m.Alloc * uint64(n) / uint64(sampleRows),
		Sys:     m.Sys * uint64(n) / uint64(sampleRows),
		Process: m.Process * uint64(n) / uint64(sampleRows),
	}
}

func (m *MemDiff) CompareWithMemBuffer(tp string, buffer kv.MemBuffer, n, sampleRows int) {
	fmt.Printf("%s %d rows with %d rows sampled\n", tp, n, n/sampleRows)
	bufferSize := buffer.Size()
	fmt.Printf("sampled alloc: %s, sys: %s, process: %s, membuffer: %s\n", readableSize(int(m.Alloc)), readableSize(int(m.Sys)), readableSize(int(m.Process)), readableSize(bufferSize))
	total := m.SampleToTotal(n, sampleRows)
	totalBufferSize := bufferSize * n / sampleRows
	fmt.Printf("total alloc: %s, sys: %s, process: %s, membuffer: %s\n", readableSize(int(total.Alloc)), readableSize(int(total.Sys)), readableSize(int(total.Process)), readableSize(totalBufferSize))
}

var units = []string{"B", "KB", "MB", "GB", "TB", "PB"}

func readableSize(bytesCount int) string {
	floatBytes := float64(bytesCount)
	for _, unit := range units {
		if floatBytes < 1024 {
			return fmt.Sprintf("%.2f%s", floatBytes, unit)
		}
		floatBytes /= 1024
	}
	return fmt.Sprintf("%.2f%s", floatBytes, units[len(units)-1])
}
