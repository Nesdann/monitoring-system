package main

import (
	"database/sql"
	"fmt"
	"monitoring-system/protocol"
)

func saveConnection(db *sql.DB, event protocol.Event, conn map[string]any) {
	pid := int(toFloat(conn["pid"]))
	srcIP, _ := conn["src_ip"].(string)
	srcPort := int(toFloat(conn["src_port"]))
	dstIP, _ := conn["dst_ip"].(string)
	dstPort := int(toFloat(conn["dst_port"]))
	proto := int(toFloat(conn["protocol"]))
	status, _ := conn["status"].(string)

	_, err := db.Exec(`
        INSERT INTO connections (timestamp, hostname, pid, src_ip, src_port, dst_ip, dst_port, protocol, status)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.Timestamp, event.Hostname, pid, srcIP, srcPort, dstIP, dstPort, proto, status,
	)
	if err != nil {
		fmt.Println("error guardando conexion:", err)
	}
}

func saveProcess(db *sql.DB, event protocol.Event, proc map[string]any) {
	pid := int(toFloat(proc["pid"]))
	name, _ := proc["name"].(string)
	username, _ := proc["user"].(string)
	cpu := toFloat(proc["cpu"])
	mem := toFloat(proc["mem"])

	_, err := db.Exec(`
        INSERT INTO processes (timestamp, hostname, pid, name, username, cpu, mem)
        VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.Timestamp, event.Hostname, pid, name, username, cpu, mem,
	)
	if err != nil {
		fmt.Println("error guardando proceso:", err)
	}
}

func toFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}
func saveMetrics(db *sql.DB, event protocol.Event) {
	cpu := toFloat(event.Data["cpu"])
	ram := toFloat(event.Data["ram"])

	_, err := db.Exec(`
        INSERT INTO metrics (timestamp, hostname, cpu, ram)
        VALUES ($1, $2, $3, $4)`,
		event.Timestamp, event.Hostname, cpu, ram,
	)
	if err != nil {
		fmt.Println("error guardando metrics:", err)
	}
}
