//go:build !linux

package main

func GetMemByPid() uint64 {
	return 0
}
