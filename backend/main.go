package main

import (
	"database/sql"
	"fmt"
	"log"
	"monitoring-system/protocol"
	"net"

	_ "github.com/lib/pq"
)

const connStr = "host=localhost user=monitoring password=monitoring123 dbname=monitoring sslmode=require"

func main() {

	//conectar a postgres
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("error conectando a postgres:", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal("postgres no responde:", err)
	}
	fmt.Println("conectado a postgres")

	listener, err := net.Listen("tcp", ":2277")
	if err != nil {
		panic(err)
	}

	store := NewStateStore()

	defer listener.Close()

	fmt.Println("Backend listening on :2277")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}

		fmt.Println("new connection:", conn.RemoteAddr())

		go handleConnection(conn, db, store)
	}
}

func handleConnection(conn net.Conn, db *sql.DB, store *StateStore) {
	defer conn.Close()

	for {
		event, err := protocol.ReadEvent(conn)
		if err != nil {
			fmt.Println("read message error:", err)
			return
		}
		handleEvent(db, event, store)
	}
}

func handleEvent(db *sql.DB, event protocol.Event, store *StateStore) {
	state := store.Get(event.Hostname)
	evaluateRules(event, state)

	switch event.Type {
	case "heartbeat":
		fmt.Printf("[heartbeat] %s\n", event.Hostname)

	case "metrics":
		cpu := event.Data["cpu"]
		ram := event.Data["ram"]
		fmt.Printf("[metrics] %s  cpu=%.1f  ram=%.1f\n", event.Hostname, cpu, ram)
		saveMetrics(db, event)

	case "process":
		procs, ok := event.Data["processes"].([]any)
		if !ok {
			return
		}
		saved := 0
		for _, p := range procs {
			proc, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if !isInterestingProcess(proc) {
				continue
			}
			saveProcess(db, event, proc)
			saved++
		}
		fmt.Printf("[processes] %s  guardados=%d\n", event.Hostname, saved)

	case "connection_snapshot":
		conns, ok := event.Data["connections"].([]any)
		if !ok {
			return
		}
		saved := 0
		for _, c := range conns {
			conn, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if !isInterestingConnection(conn) {
				continue
			}
			saveConnection(db, event, conn)
			saved++
		}
		fmt.Printf("[connections] %s  guardadas=%d\n", event.Hostname, saved)
	}
}
