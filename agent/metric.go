package main

import (
	"log"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"monitoring-system/protocol"
)

func collectMetrics(events chan<- protocol.Event) {
	for{
		cpuPercent, err := cpu.Percent(time.Second, false)
		if err != nil {
			log.Println("metrics error:", err)
			continue
		}

		vm, err := mem.VirtualMemory()
		if err != nil {
			log.Println("metrics error:", err)
			continue
		}

		events <- protocol.Event{
			Type:      "metrics",
			Hostname:  "agent-1",
			Timestamp: time.Now().Unix(),
			Data:      map[string]any{"cpu": cpuPercent[0], "ram": vm.UsedPercent},
		}

		time.Sleep(5 * time.Second)
	}
}

func heartbeat(events chan<- protocol.Event) {
	for {
		events <- protocol.Event{
			Type:      "heartbeat",
			Hostname:  "agent-1",
			Timestamp: time.Now().Unix(),
			Data:      map[string]any{},
		}

		time.Sleep(10 * time.Second)
	}
}