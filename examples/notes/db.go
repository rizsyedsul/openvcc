package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const schema = `
CREATE TABLE IF NOT EXISTS notes (
    id     UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    text   STRING  NOT NULL,
    cloud  STRING,
    created TIMESTAMP NOT NULL DEFAULT now()
);`

type Note struct {
	ID      string    `json:"id"`
	Text    string    `json:"text"`
	Cloud   string    `json:"cloud,omitempty"`
	Created time.Time `json:"created"`
}

type Store struct{ db *sql.DB }

func openStore(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Health(ctx context.Context) error {
	if s == nil {
		return errors.New("store not initialized")
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.db.PingContext(c)
}

func (s *Store) Insert(ctx context.Context, text, cloud string) (Note, error) {
	var n Note
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO notes (text, cloud) VALUES ($1, $2) RETURNING id, text, cloud, created`,
		text, cloud)
	if err := row.Scan(&n.ID, &n.Text, &n.Cloud, &n.Created); err != nil {
		return Note{}, err
	}
	return n, nil
}

func (s *Store) List(ctx context.Context, limit int) ([]Note, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, text, cloud, created FROM notes ORDER BY created DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Note, 0, limit)
	for rows.Next() {
		var n Note
		var cloud sql.NullString
		if err := rows.Scan(&n.ID, &n.Text, &cloud, &n.Created); err != nil {
			return nil, err
		}
		if cloud.Valid {
			n.Cloud = cloud.String
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
