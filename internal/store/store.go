// Package store persists dogs, records, and walks in Postgres.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"dogapp-api/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id VARCHAR(64) PRIMARY KEY,
	email VARCHAR(255) NOT NULL UNIQUE,
	password_hash VARCHAR(255) NOT NULL,
	created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS dogs (
	id VARCHAR(64) PRIMARY KEY,
	ordinal INT NOT NULL,
	name VARCHAR(255) NOT NULL,
	breed VARCHAR(255) NOT NULL,
	color VARCHAR(255) NOT NULL,
	birth_year INT NOT NULL
);

CREATE TABLE IF NOT EXISTS weight_entries (
	dog_id VARCHAR(64) NOT NULL,
	ordinal INT NOT NULL,
	month VARCHAR(32) NOT NULL,
	kg DOUBLE PRECISION NOT NULL,
	PRIMARY KEY (dog_id, ordinal),
	FOREIGN KEY (dog_id) REFERENCES dogs(id)
);

CREATE TABLE IF NOT EXISTS health_records (
	id VARCHAR(64) NOT NULL,
	dog_id VARCHAR(64) NOT NULL,
	type VARCHAR(255) NOT NULL,
	label VARCHAR(255) NOT NULL,
	"date" TIMESTAMP NOT NULL,
	cost DOUBLE PRECISION NULL,
	PRIMARY KEY (dog_id, id),
	FOREIGN KEY (dog_id) REFERENCES dogs(id)
);

CREATE TABLE IF NOT EXISTS walks (
	id VARCHAR(64) PRIMARY KEY,
	dog_id VARCHAR(64) NOT NULL,
	started_at TIMESTAMP NOT NULL,
	duration_seconds INT NOT NULL,
	distance_meters DOUBLE PRECISION NOT NULL,
	FOREIGN KEY (dog_id) REFERENCES dogs(id)
);

CREATE TABLE IF NOT EXISTS walk_points (
	walk_id VARCHAR(64) NOT NULL,
	ordinal INT NOT NULL,
	lat DOUBLE PRECISION NOT NULL,
	lng DOUBLE PRECISION NOT NULL,
	"timestamp" TIMESTAMP NOT NULL,
	PRIMARY KEY (walk_id, ordinal),
	FOREIGN KEY (walk_id) REFERENCES walks(id)
);
`

type Store struct {
	db *sql.DB
}

// Open connects to Postgres at dsn (e.g.
// "postgres://user:pass@127.0.0.1:5432/dogapp?sslmode=disable"), applies the
// schema, and seeds sample dogs (matching dogapp_flutter's
// lib/data/mock_data.dart) if the database is empty, so a fresh install
// isn't blank.
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	// health_records.type used to be a closed set of enum values (VARCHAR(32))
	// but is now free text entered by the user, so widen it to match label's
	// width. CREATE TABLE IF NOT EXISTS above doesn't alter an already-existing
	// column, so this runs unconditionally; widening a column is a no-op if
	// it's already this size or larger.
	if _, err := db.ExecContext(ctx, `ALTER TABLE health_records ALTER COLUMN type TYPE VARCHAR(255)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("widen health_records.type: %w", err)
	}
	// dogs predate login; owner_id is nullable so pre-auth seed/legacy rows
	// keep working (they just won't show up to any logged-in user's list),
	// while every dog created through the API from now on gets one.
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE dogs ADD COLUMN IF NOT EXISTS owner_id VARCHAR(64) NULL REFERENCES users(id)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("add dogs.owner_id: %w", err)
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

// ListDogs returns ownerID's dogs, in creation order.
func (s *Store) ListDogs(ctx context.Context, ownerID string) ([]model.Dog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, breed, color, birth_year FROM dogs WHERE owner_id = $1 ORDER BY ordinal`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Must start as a non-nil empty slice, not `var dogs []model.Dog` - a nil
	// slice encodes to JSON `null`, which the Flutter client can't cast to
	// List<dynamic> (every new signup starts with zero dogs, so this isn't
	// an edge case).
	dogs := []model.Dog{}
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
		`SELECT month, kg FROM weight_entries WHERE dog_id = $1 ORDER BY ordinal`, dogID)
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
		`SELECT id, type, label, "date", cost FROM health_records WHERE dog_id = $1 ORDER BY "date" DESC`, dogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := []model.HealthRecord{}
	for rows.Next() {
		var r model.HealthRecord
		var cost sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.Type, &r.Label, &r.Date, &cost); err != nil {
			return nil, err
		}
		if cost.Valid {
			r.Cost = &cost.Float64
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// CreateDog inserts a new dog owned by ownerID and returns it.
func (s *Store) CreateDog(ctx context.Context, ownerID, name, breed, color string, birthYear int) (model.Dog, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Dog{}, err
	}
	defer tx.Rollback()

	var nextOrdinal int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(ordinal), -1) + 1 FROM dogs WHERE owner_id = $1`, ownerID).Scan(&nextOrdinal); err != nil {
		return model.Dog{}, err
	}

	dog := model.Dog{
		ID:            uuid.NewString(),
		Name:          name,
		Breed:         breed,
		Color:         color,
		BirthYear:     birthYear,
		WeightHistory: []model.WeightEntry{},
		Records:       []model.HealthRecord{},
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO dogs (id, ordinal, name, breed, color, birth_year, owner_id) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		dog.ID, nextOrdinal, dog.Name, dog.Breed, dog.Color, dog.BirthYear, ownerID); err != nil {
		return model.Dog{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.Dog{}, err
	}
	return dog, nil
}

// UpdateDog updates dogID's editable profile fields and returns the updated
// dog (including its weight history and records, unaffected by this call).
// Returns sql.ErrNoRows if dogID doesn't exist or isn't owned by ownerID.
func (s *Store) UpdateDog(ctx context.Context, dogID, ownerID, name, breed, color string, birthYear int) (model.Dog, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE dogs SET name = $1, breed = $2, color = $3, birth_year = $4 WHERE id = $5 AND owner_id = $6`,
		name, breed, color, birthYear, dogID, ownerID)
	if err != nil {
		return model.Dog{}, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return model.Dog{}, err
	}
	if rows == 0 {
		return model.Dog{}, sql.ErrNoRows
	}

	dog := model.Dog{ID: dogID, Name: name, Breed: breed, Color: color, BirthYear: birthYear}
	weights, err := s.weightHistory(ctx, dogID)
	if err != nil {
		return model.Dog{}, err
	}
	dog.WeightHistory = weights
	records, err := s.healthRecords(ctx, dogID)
	if err != nil {
		return model.Dog{}, err
	}
	dog.Records = records
	return dog, nil
}

// DogOwnedBy reports whether dogID exists and is owned by ownerID. Handlers
// use this to authorize access to a specific dog's records/walks.
func (s *Store) DogOwnedBy(ctx context.Context, dogID, ownerID string) (bool, error) {
	var owned bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM dogs WHERE id = $1 AND owner_id = $2)`, dogID, ownerID).Scan(&owned)
	return owned, err
}

// AddRecord inserts a new health record for dogID and returns it with a
// generated id and the current time as its date. cost is optional (nil for
// records with no associated expense, e.g. an AI check result).
func (s *Store) AddRecord(ctx context.Context, dogID string, recordType model.RecordType, label string, cost *float64) (model.HealthRecord, error) {
	record := model.HealthRecord{
		ID:    uuid.NewString(),
		Type:  recordType,
		Label: label,
		Date:  time.Now().UTC(),
		Cost:  cost,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO health_records (id, dog_id, type, label, "date", cost) VALUES ($1, $2, $3, $4, $5, $6)`,
		record.ID, dogID, record.Type, record.Label, record.Date, nullableFloat(cost))
	if err != nil {
		return model.HealthRecord{}, err
	}
	return record, nil
}

func nullableFloat(v *float64) sql.NullFloat64 {
	if v == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *v, Valid: true}
}

// ListWalks returns dogID's walks, most recent first.
func (s *Store) ListWalks(ctx context.Context, dogID string) ([]model.WalkRoute, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, dog_id, started_at, duration_seconds, distance_meters
		 FROM walks WHERE dog_id = $1 ORDER BY started_at DESC`, dogID)
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
		`SELECT lat, lng, "timestamp" FROM walk_points WHERE walk_id = $1 ORDER BY ordinal`, walkID)
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
		`INSERT INTO walks (id, dog_id, started_at, duration_seconds, distance_meters) VALUES ($1, $2, $3, $4, $5)`,
		walk.ID, walk.DogID, walk.StartedAt, walk.DurationSeconds, walk.DistanceMeters)
	if err != nil {
		return model.WalkRoute{}, err
	}
	for i, p := range points {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO walk_points (walk_id, ordinal, lat, lng, "timestamp") VALUES ($1, $2, $3, $4, $5)`,
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
			`INSERT INTO dogs (id, ordinal, name, breed, color, birth_year) VALUES ($1, $2, $3, $4, $5, $6)`,
			d.ID, dogOrdinal, d.Name, d.Breed, d.Color, d.BirthYear); err != nil {
			return err
		}
		for i, w := range d.WeightHistory {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO weight_entries (dog_id, ordinal, month, kg) VALUES ($1, $2, $3, $4)`,
				d.ID, i, w.Month, w.Kg); err != nil {
				return err
			}
		}
		for _, r := range d.Records {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO health_records (id, dog_id, type, label, "date", cost) VALUES ($1, $2, $3, $4, $5, $6)`,
				r.ID, d.ID, r.Type, r.Label, r.Date, nullableFloat(r.Cost)); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ErrEmailTaken is returned by CreateUser when the email is already registered.
var ErrEmailTaken = errors.New("email already registered")

// CreateUser inserts a new user with an already-hashed password and returns
// it (without the hash - that never leaves the store layer as a value type).
func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (model.User, error) {
	user := model.User{ID: uuid.NewString(), Email: email}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, created_at) VALUES ($1, $2, $3, $4)`,
		user.ID, email, passwordHash, time.Now().UTC())
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return model.User{}, ErrEmailTaken
		}
		return model.User{}, err
	}
	return user, nil
}

// FindUserByEmail returns the user and their password hash for login.
// Returns sql.ErrNoRows if no user has this email.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (model.User, string, error) {
	var user model.User
	var passwordHash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash FROM users WHERE email = $1`, email).
		Scan(&user.ID, &user.Email, &passwordHash)
	if err != nil {
		return model.User{}, "", err
	}
	return user, passwordHash, nil
}
