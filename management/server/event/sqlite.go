package event

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"path/filepath"
	"time"
)

const (
	SQLiteEventSinkDB = "events.db"
	createTableQuery  = "CREATE TABLE IF NOT EXISTS events " +
		"(id INTEGER PRIMARY KEY AUTOINCREMENT, account TEXT NOT NULL, " +
		"operation INTEGER, " +
		"type TEXT, " +
		"timestamp DATETIME, " +
		"modifier TEXT," +
		" target TEXT);"
)

// SQLiteStore is the implementation of the event.Store interface backed by SQLite
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLiteStore with an event table if not exists.
func NewSQLiteStore(dataDir string) (*SQLiteStore, error) {
	dbFile := filepath.Join(dataDir, SQLiteEventSinkDB)
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(createTableQuery)
	if err != nil {
		return nil, err
	}

	return &SQLiteStore{db: db}, nil
}

func processResult(result *sql.Rows) ([]Event, error) {
	events := make([]Event, 0)
	for result.Next() {
		var id int64
		var operation string
		var timestamp time.Time
		var modifier string
		var target string
		var account string
		var typ Type
		err := result.Scan(&id, &operation, &timestamp, &modifier, &target, &account, &typ)
		if err != nil {
			return nil, err
		}

		events = append(events, Event{
			Timestamp:  timestamp,
			Operation:  operation,
			ID:         uint64(id),
			Type:       typ,
			ModifierID: modifier,
			TargetID:   target,
			AccountID:  account,
		})
	}

	return events, nil
}

// Get returns "limit" number of events from index ordered descending or ascending by a timestamp
func (store *SQLiteStore) Get(accountID string, offset, limit int, descending bool) ([]Event, error) {
	order := "DESC"
	if !descending {
		order = "ASC"
	}
	stmt, err := store.db.Prepare(fmt.Sprintf("SELECT id, operation, timestamp, modifier, target, account, type"+
		" FROM events WHERE account = ? ORDER BY timestamp %s LIMIT ? OFFSET ?;", order))
	if err != nil {
		return nil, err
	}

	result, err := stmt.Query(accountID, limit, offset)
	if err != nil {
		return nil, err
	}

	defer result.Close() //nolint
	return processResult(result)
}

// Save an event in the SQLite events table
func (store *SQLiteStore) Save(event Event) (*Event, error) {

	stmt, err := store.db.Prepare("INSERT INTO events(operation, timestamp, modifier, target, account, type) VALUES(?, ?, ?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}

	result, err := stmt.Exec(event.OperationCode, event.Timestamp, event.ModifierID, event.TargetID, event.AccountID, event.Type)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	eventCopy := event.Copy()
	eventCopy.ID = uint64(id)
	return eventCopy, nil
}

// Close the SQLiteStore
func (store *SQLiteStore) Close() error {
	if store.db != nil {
		return store.db.Close()
	}
	return nil
}
