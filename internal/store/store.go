// Package store persists dogs, records, and walks in MySQL.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"dogapp-api/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS dogs (
	id VARCHAR(64) PRIMARY KEY,
	ordinal INT NOT NULL,
	name VARCHAR(255) NOT NULL,
	breed VARCHAR(255) NOT NULL,
	color VARCHAR(255) NOT NULL,
	birth_year INT NOT NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS weight_entries (
	dog_id VARCHAR(64) NOT NULL,
	ordinal INT NOT NULL,
	month VARCHAR(32) NOT NULL,
	kg DOUBLE NOT NULL,
	PRIMARY KEY (dog_id, ordinal),
	FOREIGN KEY (dog_id) REFERENCES dogs(id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS health_records (
	id VARCHAR(64) NOT NULL,
	dog_id VARCHAR(64) NOT NULL,
	type VARCHAR(32) NOT NULL,
	label VARCHAR(255) NOT NULL,
	` + "`date`" + ` DATETIME NOT NULL,
	PRIMARY KEY (dog_id, id),
	FOREIGN KEY (dog_id) REFERENCES dogs(id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS walks (
	id VARCHAR(64) PRIMARY KEY,
	dog_id VARCHAR(64) NOT NULL,
	started_at DATETIME NOT NULL,
	duration_seconds INT NOT NULL,
	distance_meters DOUBLE NOT NULL,
	FOREIGN KEY (dog_id) REFERENCES dogs(id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS walk_points (
	walk_id VARCHAR(64) NOT NULL,
	ordinal INT NOT NULL,
	lat DOUBLE NOT NULL,
	lng DOUBLE NOT NULL,
	` + "`timestamp`" + ` DATETIME NOT NULL,
	PRIMARY KEY (walk_id, ordinal),
	FOREIGN KEY (walk_id) REFERENCES walks(id)
) ENGINE=InnoDB;
`

type Store struct {
	db *sql.DB
}

// Open connects to MySQL at dsn (e.g.
// "user:pass@tcp(127.0.0.1:3306)/dogapp?parseTime=true&multiStatements=true&loc=UTC"),
// applies the schema, and seeds sample dogs (matching dogapp_flutter's
// lib/data/mock_data.dart) if the database is empty, so a fresh install
// isn't blank.
//
// The DSN must include parseTime=true (so DATETIME columns scan into
// time.Time) and multiStatements=true (schema is applied as one multi-
// statement Exec).
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to mysql: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	s := &Store{db: db}
	if err := s.seedIfEmpty(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// ListDogs returns every dog, in seed/creation order. dogapp-api has no
// real multi-tenant auth yet, so ownerId (from GET /owners/{ownerId}/dogs)
// is accepted but unused.
func (s *Store) ListDogs(ctx context.Context) ([]model.Dog, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, breed, color, birth_year FROM dogs ORDER BY ordinal`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dogs []model.Dog
	for rows.Next() {
		var d model.Dog
		if err := rows.Scan(&d.ID, &d.Name, &d.Breed, &d.Color, &d.BirthYear); err != nil {
			return nil, err
		}
		dogs = append(dogs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range dogs {
		weights, err := s.weightHistory(ctx, dogs[i].ID)
		if err != nil {
			return nil, err
		}
		dogs[i].WeightHistory = weights

		records, err := s.healthRecords(ctx, dogs[i].ID)
		if err != nil {
			return nil, err
		}
		dogs[i].Records = records
	}
	return dogs, nil
}

func (s *Store) weightHistory(ctx context.Context, dogID string) ([]model.WeightEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT month, kg FROM weight_entries WHERE dog_id = ? ORDER BY ordinal`, dogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []model.WeightEntry{}
	for rows.Next() {
		var e model.WeightEntry
		if err := rows.Scan(&e.Month, &e.Kg); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) healthRecords(ctx context.Context, dogID string) ([]model.HealthRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, type, label, `date` FROM health_records WHERE dog_id = ? ORDER BY `date` DESC", dogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []model.HealthRecord{}
	for rows.Next() {
		var r model.HealthRecord
		if err := rows.Scan(&r.ID, &r.Type, &r.Label, &r.Date); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// DogExists reports whether dogID is a known dog.
func (s *Store) DogExists(ctx context.Context, dogID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dogs WHERE id = ?)`, dogID).Scan(&exists)
	return exists, err
}

// AddRecord inserts a new health record for dogID and returns it with a
// generated id and the current time as its date.
func (s *Store) AddRecord(ctx context.Context, dogID string, recordType model.RecordType, label string) (model.HealthRecord, error) {
	record := model.HealthRecord{
		ID:    uuid.NewString(),
		Type:  recordType,
		Label: label,
		Date:  time.Now().UTC(),
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO health_records (id, dog_id, type, label, `date`) VALUES (?, ?, ?, ?, ?)",
		record.ID, dogID, record.Type, record.Label, record.Date)
	if err != nil {
		return model.HealthRecord{}, err
	}
	return record, nil
}

// ListWalks returns dogID's walks, most recent first.
func (s *Store) ListWalks(ctx context.Context, dogID string) ([]model.WalkRoute, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, dog_id, started_at, duration_seconds, distance_meters
		 FROM walks WHERE dog_id = ? ORDER BY started_at DESC`, dogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	walks := []model.WalkRoute{}
	for rows.Next() {
		var w model.WalkRoute
		if err := rows.Scan(&w.ID, &w.DogID, &w.StartedAt, &w.DurationSeconds, &w.DistanceMeters); err != nil {
			return nil, err
		}
		walks = append(walks, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range walks {
		points, err := s.walkPoints(ctx, walks[i].ID)
		if err != nil {
			return nil, err
		}
		walks[i].Points = points
	}
	return walks, nil
}

func (s *Store) walkPoints(ctx context.Context, walkID string) ([]model.GeoPoint, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT lat, lng, `timestamp` FROM walk_points WHERE walk_id = ? ORDER BY ordinal", walkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []model.GeoPoint{}
	for rows.Next() {
		var p model.GeoPoint
		if err := rows.Scan(&p.Lat, &p.Lng, &p.Timestamp); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// CreateWalk inserts a new walk (with a generated id) for dogID and returns it.
func (s *Store) CreateWalk(ctx context.Context, dogID string, startedAt time.Time, durationSeconds int, distanceMeters float64, points []model.GeoPoint) (model.WalkRoute, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.WalkRoute{}, err
	}
	defer tx.Rollback()

	walk := model.WalkRoute{
		ID:              uuid.NewString(),
		DogID:           dogID,
		StartedAt:       startedAt,
		DurationSeconds: durationSeconds,
		DistanceMeters:  distanceMeters,
		Points:          points,
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO walks (id, dog_id, started_at, duration_seconds, distance_meters) VALUES (?, ?, ?, ?, ?)`,
		walk.ID, walk.DogID, walk.StartedAt, walk.DurationSeconds, walk.DistanceMeters)
	if err != nil {
		return model.WalkRoute{}, err
	}
	for i, p := range points {
		_, err = tx.ExecContext(ctx,
			"INSERT INTO walk_points (walk_id, ordinal, lat, lng, `timestamp`) VALUES (?, ?, ?, ?, ?)",
			walk.ID, i, p.Lat, p.Lng, p.Timestamp)
		if err != nil {
			return model.WalkRoute{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.WalkRoute{}, err
	}
	return walk, nil
}

func (s *Store) seedIfEmpty(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dogs`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for dogOrdinal, d := range seedDogs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO dogs (id, ordinal, name, breed, color, birth_year) VALUES (?, ?, ?, ?, ?, ?)`,
			d.ID, dogOrdinal, d.Name, d.Breed, d.Color, d.BirthYear); err != nil {
			return err
		}
		for i, w := range d.WeightHistory {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO weight_entries (dog_id, ordinal, month, kg) VALUES (?, ?, ?, ?)`,
				d.ID, i, w.Month, w.Kg); err != nil {
				return err
			}
		}
		for _, r := range d.Records {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO health_records (id, dog_id, type, label, `date`) VALUES (?, ?, ?, ?, ?)",
				r.ID, d.ID, r.Type, r.Label, r.Date); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
