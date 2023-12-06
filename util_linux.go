//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var factors = map[string]uint64{
	"B":  1,
	"K":  1024,
	"KB": 1024,
	"M":  1024 * 1024,
	"MB": 1024 * 1024,
	"G":  1024 * 1024 * 1024,
	"GB": 1024 * 1024 * 1024,
	"T":  1024 * 1024 * 1024 * 1024,
	"TB": 1024 * 1024 * 1024 * 1024,
}

func GetMemByPid() uint64 {
	pid := os.Getpid()
	cmd := fmt.Sprintf("-q %d -o rss=", pid)
	outBytes, err := exec.Command("ps", strings.Split(cmd, " ")...).Output()
	if err != nil {
		fmt.Println(err)
		return 0
	}
	out := string(outBytes)
	out = strings.TrimSpace(strings.ToLower(out))
	if mem, err := strconv.ParseUint(out, 10, 64); err == nil {
		return mem * 1024 // rss is in KB
	}
	for unit, factor := range factors {
		if strings.HasSuffix(out, unit) {
			nums := strings.ReplaceAll(out, unit, "")
			mem, err := strconv.ParseUint(nums, 10, 64)
			if err != nil {
				continue
			}
			return mem * factor
		}
	}
	fmt.Println("parse mem failed:", out)
	return 0
}
