package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

const version = "0.1.0-m1.1"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			fmt.Fprintln(os.Stderr, "court: open sqlite:", err)
			os.Exit(1)
		}
		defer db.Close()
		var v string
		if err := db.QueryRow("select sqlite_version()").Scan(&v); err != nil {
			fmt.Fprintln(os.Stderr, "court: sqlite_version:", err)
			os.Exit(1)
		}
		fmt.Printf("court %s (sqlite %s)\n", version, v)
		return
	}
	fmt.Fprintln(os.Stderr, "usage: court version")
	os.Exit(2)
}
