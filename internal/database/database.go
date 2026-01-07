package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

type Database struct {
	Conn *sql.DB
}

type FileMetadata struct {
	ID        int
	Name      string
	Size      int64
	Hash      string
	CreatedAt time.Time
}

type ChunkMetadata struct {
	ID        int
	FileID    int
	MessageID string
	PartNum   int
}

func Initialize(path string) (*Database, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable Foreign Keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return &Database{Conn: db}, nil
}

func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			size INTEGER NOT NULL,
			hash TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_id INTEGER NOT NULL,
			message_id TEXT NOT NULL,
			part_num INTEGER NOT NULL,
			FOREIGN KEY(file_id) REFERENCES files(id) ON DELETE CASCADE
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}

func (db *Database) SaveFile(name string, size int64, hash string) (int, error) {
	query := `INSERT INTO files (name, size, hash) VALUES (?, ?, ?) RETURNING id`
	var id int
	err := db.Conn.QueryRow(query, name, size, hash).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (db *Database) SaveChunk(fileID int, messageID string, partNum int) error {
	query := `INSERT INTO chunks (file_id, message_id, part_num) VALUES (?, ?, ?)`
	_, err := db.Conn.Exec(query, fileID, messageID, partNum)
	return err
}

func (db *Database) ListFiles() ([]FileMetadata, error) {
	query := `SELECT id, name, size, hash, created_at FROM files ORDER BY created_at DESC`
	rows, err := db.Conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []FileMetadata
	for rows.Next() {
		var f FileMetadata
		if err := rows.Scan(&f.ID, &f.Name, &f.Size, &f.Hash, &f.CreatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, nil
}

func (db *Database) GetFile(id int) (*FileMetadata, error) {
	query := `SELECT id, name, size, hash, created_at FROM files WHERE id = ?`
	var f FileMetadata
	err := db.Conn.QueryRow(query, id).Scan(&f.ID, &f.Name, &f.Size, &f.Hash, &f.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (db *Database) DeleteFile(id int) error {
	// Cascading delete should handle chunks due to foreign key constraint,
	// but we must ensure foreign keys are enabled or delete manually.
	// SQLite supports FKs but they might need to be enabled.
	// Safest is to rely on ON DELETE CASCADE if enabled, or just delete chunks first?
	// The table definition HAS "ON DELETE CASCADE".
	// We just need to ensure PRAGMA foreign_keys = ON; is set.
	// I'll add that to Initialize or just trust manual deletion for now to be safe.

	_, err := db.Conn.Exec("DELETE FROM files WHERE id = ?", id)
	return err
}

func (db *Database) GetChunks(fileID int) ([]ChunkMetadata, error) {
	query := `SELECT id, file_id, message_id, part_num FROM chunks WHERE file_id = ? ORDER BY part_num ASC`
	rows, err := db.Conn.Query(query, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []ChunkMetadata
	for rows.Next() {
		var c ChunkMetadata
		if err := rows.Scan(&c.ID, &c.FileID, &c.MessageID, &c.PartNum); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}
