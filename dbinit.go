package main

import "log"

func dbInit() {
	db, err := openDB(".gcalsync.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	var dbVersion int
	err = db.QueryRow("SELECT version FROM db_version WHERE name='gcalsync'").Scan(&dbVersion)
	if err != nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS db_version (
			name TEXT PRIMARY KEY,
			version INTEGER
		)`)
		if err != nil {
			log.Fatalf("Error creating db_version table: %v", err)
		}
		_, err = db.Exec(`INSERT INTO db_version (name, version) VALUES ('gcalsync', 0)`)
		if err != nil {
			log.Fatalf("Error initializing db_version table: %v", err)
		}
		dbVersion = 0
	}

	if dbVersion == 0 {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tokens (
		account_name TEXT PRIMARY KEY,
		token TEXT)`)
		if err != nil {
			log.Fatalf("Error creating tokens table: %v", err)
		}

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS calendars (
		account_name TEXT,
		calendar_id TEXT,
		PRIMARY KEY (account_name, calendar_id))`)

		if err != nil {
			log.Fatalf("Error creating calendars table: %v", err)
		}

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS blocker_events (
			event_id TEXT,
			calendar_id TEXT,
			account_name TEXT,
			origin_event_id TEXT,
			PRIMARY KEY (calendar_id, origin_event_id)
		)`)

		if err != nil {
			log.Fatalf("Error creating blocker_events table: %v", err)
		}

		dbVersion = 1
		_, err = db.Exec(`UPDATE db_version SET version = 1 WHERE name = 'gcalsync'`)
		if err != nil {
			log.Fatalf("Error updating db_version table: %v", err)
		}
	}
}
