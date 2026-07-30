package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"monitoring-system/protocol"
	"net"

	_ "github.com/lib/pq"
)

const connStr = "host=localhost user=monitoring password=monitoring123 dbname=monitoring sslmode=require"

func main() {
	genOwner := flag.String("generate-key", "", "generate an API key for this owner name, then exit")
	genRole := flag.String("role", "viewer", "role for the generated key: viewer or admin")
	adminKey := flag.String("admin-key", "", "an existing admin key, required to generate new keys after the first")
	flag.Parse()

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
	// crea las tablas si no existen, para que el usuario no tenga que hacerlo a mano
	if err := runMigrations(db); err != nil {
		log.Fatal("error running migrations:", err)
	}
	go startAPIServer(db)
	fmt.Println("migrations OK")

	if *genOwner != "" {
		key, err := generateAPIKey(db, *genOwner, *genRole, *adminKey)
		if err != nil {
			log.Fatal("error generating key:", err)
		}
		fmt.Println("API key for", *genOwner, "(role:", *genRole, "):", key)
		fmt.Println("Save this now — it will not be shown again.")
		return
	}

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
		cpu := toFloat(event.Data["cpu"])
		ram := toFloat(event.Data["ram"])
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

// runMigrations creates every table this system needs, if it doesn't exist yet.
// Safe to run on every startup — CREATE TABLE IF NOT EXISTS is a no-op when the table is already there.
func runMigrations(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS metrics (
			id SERIAL PRIMARY KEY,
			timestamp BIGINT NOT NULL,
			hostname TEXT NOT NULL,
			cpu DOUBLE PRECISION NOT NULL,
			ram DOUBLE PRECISION NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS host_risk (
			hostname TEXT PRIMARY KEY,
			score DOUBLE PRECISION NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS processes (
			id SERIAL PRIMARY KEY,
			timestamp BIGINT NOT NULL,
			hostname TEXT NOT NULL,
			pid INTEGER NOT NULL,
			name TEXT NOT NULL,
			username TEXT NOT NULL,
			cpu DOUBLE PRECISION NOT NULL,
			mem DOUBLE PRECISION NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS connections (
			id SERIAL PRIMARY KEY,
			timestamp BIGINT NOT NULL,
			hostname TEXT NOT NULL,
			pid INTEGER NOT NULL,
			src_ip TEXT NOT NULL,
			src_port INTEGER NOT NULL,
			dst_ip TEXT NOT NULL,
			dst_port INTEGER NOT NULL,
			protocol INTEGER NOT NULL,
			status TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id SERIAL PRIMARY KEY,
			key_hash TEXT NOT NULL UNIQUE,
			owner TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			created_at BIGINT NOT NULL,
			active BOOLEAN NOT NULL DEFAULT true
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id SERIAL PRIMARY KEY,
			timestamp BIGINT NOT NULL,
			hostname TEXT NOT NULL,
			detector TEXT NOT NULL,
			severity TEXT NOT NULL,
			message TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT 'uncategorized'
		)`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'uncategorized'`,
		`ALTER TABLE processes ADD COLUMN IF NOT EXISTS ppid INTEGER NOT NULL DEFAULT -1`,
		`ALTER TABLE processes ADD COLUMN IF NOT EXISTS exe TEXT NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE processes ADD COLUMN IF NOT EXISTS create_time BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE processes ADD COLUMN IF NOT EXISTS num_fds INTEGER NOT NULL DEFAULT -1`,
		`ALTER TABLE connections ADD COLUMN IF NOT EXISTS process_name TEXT NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS occurrence_count INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE alerts ADD COLUMN IF NOT EXISTS last_seen BIGINT`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}
