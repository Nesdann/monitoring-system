package main

import (
	"log"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"monitoring-system/protocol"
)

func collectMetrics(events chan<- protocol.Event) {
	for {
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

func collectProcess(events chan<- protocol.Event) {
	for {
		procs, err := process.Processes()
		if err != nil {
			log.Println("process error:", err)
			continue
		}

		data := make([]map[string]any, 0, len(procs))
		for _, proc := range procs {
			name, err := proc.Name()
			if err != nil {
				log.Println("process error:", err)
				continue
			}

			pid := proc.Pid

			user, err := proc.Username()
			if err != nil {
				user = "unknown"
			}

			cpuPercent, err := proc.CPUPercent()
			if err != nil {
				log.Println("process error:", err)
				continue
			}

			memPercent, err := proc.MemoryPercent()
			if err != nil {
				log.Println("process error:", err)
				continue
			}

			pinfo := map[string]any{
				"name": name,
				"pid":  pid,
				"user": user,
				"cpu":  cpuPercent,
				"mem":  memPercent,
			}

			data = append(data, pinfo)
		}

		events <- protocol.Event{
			Type:      "process",
			Hostname:  "agent-1",
			Timestamp: time.Now().Unix(),
			Data:      map[string]any{"processes": data},
		}

		time.Sleep(15 * time.Second)
	}

}
