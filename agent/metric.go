package main

import (
	"log"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
	"monitoring-system/protocol"
	"github.com/shirou/gopsutil/v4/net"
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
				log.Println("process name error:", err)
				name = "unknown"
			}

			pid := proc.Pid

			user, err := proc.Username()
			if err != nil {
				log.Println("process user error:", err)
				user = "unknown"
			}

			cpuPercent, err := proc.CPUPercent()
			if err != nil {
				log.Println("process cpu error:", err)
				cpuPercent = 0.0
			}

			memPercent, err := proc.MemoryPercent()
			if err != nil {
				log.Println("process mem error:", err)
				memPercent = 0.0
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
func collectConnections(events chan<- protocol.Event) {
    for {
        conns, err := net.Connections("inet")
        if err != nil {
            log.Println("connections error:", err)
            time.Sleep(20 * time.Second)
            continue
        }

        data := make([]map[string]any, 0, len(conns))
		for _, conn := range conns {
			if conn.Raddr.IP == "" {
            continue // skip sockets en LISTEN sin destino
            }
			pid := conn.Pid
			if pid == 0 {
				pid = -1
			}
			srcIP := conn.Laddr.IP
			if srcIP == "" {
				srcIP = "unknown"
			}
			srcPort := conn.Laddr.Port
			dstIP := conn.Raddr.IP
			if dstIP == "" {
				dstIP = "nuncadeberiaocurrir"
			}
			dstPort := conn.Raddr.Port
			protoType := conn.Type
			status := conn.Status
			if status == "" {
				status = "unknown"
			}
			data = append(data, map[string]any{
				"pid":       pid,
				"src_ip":    srcIP,
				"src_port":  srcPort,
				"dst_ip":    dstIP,
				"dst_port":  dstPort,
				"protocol":  protoType,
				"status":    status,
			})
		}

        events <- protocol.Event{
            Type:      "connection_snapshot",
            Hostname:  "agent-1",
            Timestamp: time.Now().Unix(),
            Data:      map[string]any{"connections": data},
        }

        time.Sleep(20 * time.Second)
    }
}
