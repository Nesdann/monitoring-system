package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

func startAPIServer(db *sql.DB) {
	mux := http.NewServeMux()

	mux.HandleFunc("/hosts", requireRole(db, "viewer", handleHosts(db)))
	mux.HandleFunc("/alerts", requireRole(db, "viewer", handleAlerts(db)))
	mux.HandleFunc("/risk", requireRole(db, "viewer", handleRisk(db)))
	mux.HandleFunc("/metrics", requireRole(db, "viewer", handleMetrics(db)))
	mux.HandleFunc("/processes", requireRole(db, "admin", handleProcesses(db)))
	mux.HandleFunc("/connections", requireRole(db, "admin", handleConnections(db)))
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./dashboard.html")
	})

	println("API listening on :8081")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		println("API server error:", err.Error())
	}
}

// GET /hosts -> list of distinct hostnames seen
func handleHosts(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT DISTINCT hostname FROM metrics")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var hosts []string
		for rows.Next() {
			var h string
			rows.Scan(&h)
			hosts = append(hosts, h)
		}
		writeJSON(w, hosts)
	}
}

// GET /alerts?hostname=&category=&severity=&since=&limit=&offset=
func handleAlerts(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		hostname := q.Get("hostname")
		category := q.Get("category")
		severity := q.Get("severity")
		since := parseIntDefault(q.Get("since"), 0)  // unix timestamp
		limit := parseIntDefault(q.Get("limit"), 50) // default page size
		offset := parseIntDefault(q.Get("offset"), 0)

		if limit > 200 {
			limit = 200 // hard cap so nobody can request the whole table at once
		}

		query := `SELECT id, timestamp, hostname, detector, severity, message, category, occurrence_count, last_seen
                  FROM alerts WHERE 1=1`
		args := []any{}
		argN := 1

		if hostname != "" {
			query += " AND hostname = $" + strconv.Itoa(argN)
			args = append(args, hostname)
			argN++
		}
		if category != "" {
			query += " AND category = $" + strconv.Itoa(argN)
			args = append(args, category)
			argN++
		}
		if severity != "" {
			query += " AND severity = $" + strconv.Itoa(argN)
			args = append(args, severity)
			argN++
		}
		if since > 0 {
			query += " AND last_seen >= $" + strconv.Itoa(argN)
			args = append(args, since)
			argN++
		}

		query += " ORDER BY last_seen DESC LIMIT $" + strconv.Itoa(argN) + " OFFSET $" + strconv.Itoa(argN+1)
		args = append(args, limit, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Alert struct {
			ID              int    `json:"id"`
			Timestamp       int64  `json:"timestamp"`
			Hostname        string `json:"hostname"`
			Detector        string `json:"detector"`
			Severity        string `json:"severity"`
			Message         string `json:"message"`
			Category        string `json:"category"`
			OccurrenceCount int    `json:"occurrence_count"`
			LastSeen        int64  `json:"last_seen"`
		}

		var results []Alert
		for rows.Next() {
			var a Alert
			if err := rows.Scan(&a.ID, &a.Timestamp, &a.Hostname, &a.Detector, &a.Severity, &a.Message, &a.Category, &a.OccurrenceCount, &a.LastSeen); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			results = append(results, a)
		}
		writeJSON(w, results)
	}
}

// GET /risk?hostname=
func handleRisk(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostname := r.URL.Query().Get("hostname")

		if hostname != "" {
			row := db.QueryRow("SELECT hostname, score, updated_at FROM host_risk WHERE hostname = $1", hostname)
			var h string
			var score float64
			var updated int64
			if err := row.Scan(&h, &score, &updated); err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			writeJSON(w, map[string]any{"hostname": h, "score": score, "updated_at": updated})
			return
		}

		rows, err := db.Query("SELECT hostname, score, updated_at FROM host_risk ORDER BY score DESC")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var h string
			var score float64
			var updated int64
			rows.Scan(&h, &score, &updated)
			results = append(results, map[string]any{"hostname": h, "score": score, "updated_at": updated})
		}
		writeJSON(w, results)
	}
}

// GET /metrics?hostname=&since=&limit=
func handleMetrics(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		hostname := q.Get("hostname")
		since := parseIntDefault(q.Get("since"), 0)
		limit := parseIntDefault(q.Get("limit"), 100)
		if limit > 500 {
			limit = 500
		}

		if hostname == "" {
			http.Error(w, "hostname is required", http.StatusBadRequest)
			return
		}

		rows, err := db.Query(
			"SELECT timestamp, cpu, ram FROM metrics WHERE hostname = $1 AND timestamp >= $2 ORDER BY timestamp DESC LIMIT $3",
			hostname, since, limit,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Metric struct {
			Timestamp int64   `json:"timestamp"`
			CPU       float64 `json:"cpu"`
			RAM       float64 `json:"ram"`
		}

		var results []Metric
		for rows.Next() {
			var m Metric
			rows.Scan(&m.Timestamp, &m.CPU, &m.RAM)
			results = append(results, m)
		}
		writeJSON(w, results)
	}
}

// GET /processes?hostname=&name=&since=&limit=&offset=
func handleProcesses(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		hostname := q.Get("hostname")
		name := q.Get("name")
		since := parseIntDefault(q.Get("since"), 0)
		limit := parseIntDefault(q.Get("limit"), 50)
		offset := parseIntDefault(q.Get("offset"), 0)
		if limit > 200 {
			limit = 200
		}

		query := `SELECT timestamp, hostname, pid, name, username, cpu, mem, ppid, exe, create_time, num_fds
                  FROM processes WHERE 1=1`
		args := []any{}
		argN := 1

		if hostname != "" {
			query += " AND hostname = $" + strconv.Itoa(argN)
			args = append(args, hostname)
			argN++
		}
		if name != "" {
			query += " AND name = $" + strconv.Itoa(argN)
			args = append(args, name)
			argN++
		}
		if since > 0 {
			query += " AND timestamp >= $" + strconv.Itoa(argN)
			args = append(args, since)
			argN++
		}

		query += " ORDER BY timestamp DESC LIMIT $" + strconv.Itoa(argN) + " OFFSET $" + strconv.Itoa(argN+1)
		args = append(args, limit, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Process struct {
			Timestamp  int64   `json:"timestamp"`
			Hostname   string  `json:"hostname"`
			PID        int     `json:"pid"`
			Name       string  `json:"name"`
			Username   string  `json:"username"`
			CPU        float64 `json:"cpu"`
			Mem        float64 `json:"mem"`
			PPID       int     `json:"ppid"`
			Exe        string  `json:"exe"`
			CreateTime int64   `json:"create_time"`
			NumFDs     int     `json:"num_fds"`
		}

		var results []Process
		for rows.Next() {
			var p Process
			if err := rows.Scan(&p.Timestamp, &p.Hostname, &p.PID, &p.Name, &p.Username, &p.CPU, &p.Mem, &p.PPID, &p.Exe, &p.CreateTime, &p.NumFDs); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			results = append(results, p)
		}
		writeJSON(w, results)
	}
}

// GET /connections?hostname=&status=&dst_ip=&since=&limit=&offset=
func handleConnections(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		hostname := q.Get("hostname")
		status := q.Get("status")
		dstIP := q.Get("dst_ip")
		since := parseIntDefault(q.Get("since"), 0)
		limit := parseIntDefault(q.Get("limit"), 50)
		offset := parseIntDefault(q.Get("offset"), 0)
		if limit > 200 {
			limit = 200
		}

		query := `SELECT timestamp, hostname, pid, process_name, src_ip, src_port, dst_ip, dst_port, protocol, status
                  FROM connections WHERE 1=1`
		args := []any{}
		argN := 1

		if hostname != "" {
			query += " AND hostname = $" + strconv.Itoa(argN)
			args = append(args, hostname)
			argN++
		}
		if status != "" {
			query += " AND status = $" + strconv.Itoa(argN)
			args = append(args, status)
			argN++
		}
		if dstIP != "" {
			query += " AND dst_ip = $" + strconv.Itoa(argN)
			args = append(args, dstIP)
			argN++
		}
		if since > 0 {
			query += " AND timestamp >= $" + strconv.Itoa(argN)
			args = append(args, since)
			argN++
		}

		query += " ORDER BY timestamp DESC LIMIT $" + strconv.Itoa(argN) + " OFFSET $" + strconv.Itoa(argN+1)
		args = append(args, limit, offset)

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type Connection struct {
			Timestamp   int64  `json:"timestamp"`
			Hostname    string `json:"hostname"`
			PID         int    `json:"pid"`
			ProcessName string `json:"process_name"`
			SrcIP       string `json:"src_ip"`
			SrcPort     int    `json:"src_port"`
			DstIP       string `json:"dst_ip"`
			DstPort     int    `json:"dst_port"`
			Protocol    int    `json:"protocol"`
			Status      string `json:"status"`
		}

		var results []Connection
		for rows.Next() {
			var c Connection
			if err := rows.Scan(&c.Timestamp, &c.Hostname, &c.PID, &c.ProcessName, &c.SrcIP, &c.SrcPort, &c.DstIP, &c.DstPort, &c.Protocol, &c.Status); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			results = append(results, c)
		}
		writeJSON(w, results)
	}
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func parseIntDefault(s string, def int64) int64 {
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return n
}
