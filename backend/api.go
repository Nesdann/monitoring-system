package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

func startAPIServer(db *sql.DB) {
	mux := http.NewServeMux()

	mux.HandleFunc("/hosts", handleHosts(db))
	mux.HandleFunc("/alerts", handleAlerts(db))
	mux.HandleFunc("/risk", handleRisk(db))
	mux.HandleFunc("/metrics", handleMetrics(db))

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
