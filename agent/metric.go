package main

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

func collectMetrics() (float64, float64, error) {

	cpuPercent, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, 0, err
	}

	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}

	return cpuPercent[0], vm.UsedPercent, nil
}