package main

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/lib/pq" // Postgres driver
)

// connectDB connects to Postgres (retrying while the DB container starts)
// and creates the claims table. Note: there is deliberately NO foreign key to
// the items table, because that table lives in a different service/database.
func connectDB(dsn string) *sql.DB {
	var db *sql.DB
	var err error

	for attempt := 1; attempt <= 30; attempt++ {
		db, err = sql.Open("postgres", dsn)
		if err == nil {
			if err = db.Ping(); err == nil {
				break
			}
		}
		log.Printf("waiting for database (attempt %d/30): %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("could not connect to database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS claims (
			id             BIGSERIAL PRIMARY KEY,
			item_id        BIGINT NOT NULL,
			claimant_name  TEXT NOT NULL,
			claimant_email TEXT NOT NULL,
			status         TEXT NOT NULL DEFAULT 'PENDING',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		log.Fatalf("could not create claims table: %v", err)
	}
	log.Println("connected to claim database")
	return db
}
