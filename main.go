package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/pingcap/tidb/pkg/planner/core"
	"github.com/pingcap/tidb/pkg/util/logutil"
)

var (
	schema    string
	rows      uint64
	sample    uint64
	mode      string
	dropKey   bool
	dropValue bool
)

func init() {
	flag.StringVar(&schema, "schema", "sbtest", "schema name")
	flag.Uint64Var(&rows, "rows", 1_000_000, "number of rows")
	flag.Uint64Var(&sample, "sample", 10_000, "sample every n rows")
	flag.StringVar(&mode, "mode", "insert", "mode: insert, delete, update")
	flag.BoolVar(&dropKey, "drop-key", false, "drop key to save memory")
	flag.BoolVar(&dropValue, "drop-value", false, "drop value to save memory")
	flag.Parse()
	logutil.SetLevel("error")
}

func main() {
	if schema == "" {
		fmt.Println("schema is empty")
		return
	}
	if rows <= 0 {
		fmt.Println("rows is invalid")
		return
	}
	b, err := os.ReadFile(schema)
	if err != nil {
		fmt.Println(err)
		return
	}
	schemaSQL := string(b)
	core, err := NewCore(schemaSQL, dropKey, dropValue)
	if err != nil {
		fmt.Println(err)
		return
	}
	switch mode {
	case "insert":
		n, err := core.InsertRows(int(rows), int(sample))
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("insert %d rows with memory cost: %s(%d bytes)\n", rows, readableSize(n), n)
	case "update":
		n, err := core.UpdateRows(int(rows), int(sample))
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("update %d rows with memory cost: %s(%d bytes)\n", rows, readableSize(n), n)
	case "delete":
		n, err := core.DeleteRows(int(rows), int(sample))
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("delete %d rows with memory cost: %s(%d bytes)\n", rows, readableSize(n), n)
	default:
		info := fmt.Sprintf("mode %s is not supported", mode)
		fmt.Println(info)
		return
	}
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
