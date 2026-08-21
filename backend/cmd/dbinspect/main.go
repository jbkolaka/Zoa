// Command dbinspect is a development utility for looking inside app.db without
// the sqlite3 CLI. It prints the schema, row counts, and optionally the rows of
// a chosen table.
//
//	go run ./cmd/dbinspect                  # schema + row counts
//	go run ./cmd/dbinspect -table users     # dump one table
//	go run ./cmd/dbinspect -sql "SELECT …"  # arbitrary read-only query
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"zoa/backend/internal/db"
)

func main() {
	dbPath := flag.String("db", envOr("DB_PATH", "app.db"), "path to the SQLite file")
	table := flag.String("table", "", "dump all rows of this table")
	query := flag.String("sql", "", "run an arbitrary query and print the result")
	flag.Parse()

	conn, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open %s: %v", *dbPath, err)
	}
	defer conn.Close()

	switch {
	case *query != "":
		mustDump(conn, *query)
	case *table != "":
		mustDump(conn, "SELECT * FROM "+*table)
	default:
		printSchema(conn)
		printCounts(conn)
	}
}

func printSchema(conn *sql.DB) {
	rows, err := conn.Query(`
		SELECT type, name, COALESCE(sql, '')
		FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%'
		ORDER BY CASE type WHEN 'table' THEN 0 ELSE 1 END, name`)
	if err != nil {
		log.Fatalf("read schema: %v", err)
	}
	defer rows.Close()

	fmt.Println("=== SCHEMA ===")
	for rows.Next() {
		var kind, name, ddl string
		if err := rows.Scan(&kind, &name, &ddl); err != nil {
			log.Fatalf("scan schema: %v", err)
		}
		if kind == "index" {
			fmt.Printf("\n-- %s %s\n%s;\n", kind, name, ddl)
			continue
		}
		fmt.Printf("\n%s;\n", ddl)
	}
}

func printCounts(conn *sql.DB) {
	rows, err := conn.Query(`
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		log.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			log.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, t)
	}
	rows.Close()

	fmt.Println("\n=== ROW COUNTS ===")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, t := range tables {
		var n int
		if err := conn.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n); err != nil {
			log.Fatalf("count %s: %v", t, err)
		}
		fmt.Fprintf(w, "%s\t%d\n", t, n)
	}
	w.Flush()
}

// mustDump runs a query and prints the result set as an aligned table.
func mustDump(conn *sql.DB, query string) {
	rows, err := conn.Query(query)
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		log.Fatalf("columns: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(cols, "\t"))
	fmt.Fprintln(w, strings.Repeat("-\t", len(cols)))

	count := 0
	for rows.Next() {
		cells := make([]any, len(cols))
		for i := range cells {
			cells[i] = new(sql.NullString)
		}
		if err := rows.Scan(cells...); err != nil {
			log.Fatalf("scan row: %v", err)
		}

		out := make([]string, len(cols))
		for i, c := range cells {
			if ns := c.(*sql.NullString); ns.Valid {
				out[i] = ns.String
			} else {
				out[i] = "NULL"
			}
		}
		fmt.Fprintln(w, strings.Join(out, "\t"))
		count++
	}
	w.Flush()

	if err := rows.Err(); err != nil {
		log.Fatalf("iterate: %v", err)
	}
	fmt.Printf("\n(%d rows)\n", count)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
