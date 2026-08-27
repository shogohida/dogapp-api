package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"dogapp-api/internal/model"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenSeedsSampleDogs(t *testing.T) {
	s := openTestStore(t)

	dogs, err := s.ListDogs(context.Background())
	if err != nil {
		t.Fatalf("ListDogs: %v", err)
	}
	if len(dogs) != 2 {
		t.Fatalf("expected 2 seeded dogs, got %d", len(dogs))
	}
	if dogs[0].ID != "leo" || dogs[1].ID != "noa" {
		t.Fatalf("unexpected dog ids: %v", []string{dogs[0].ID, dogs[1].ID})
	}
	if len(dogs[0].WeightHistory) != 6 {
		t.Fatalf("expected 6 weight entries for leo, got %d", len(dogs[0].WeightHistory))
	}
	if len(dogs[0].Records) != 3 {
		t.Fatalf("expected 3 records for leo, got %d", len(dogs[0].Records))
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	// Re-opening an existing (non-empty) database must not duplicate seed data.
	path := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()

	s1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (1st): %v", err)
	}
	s1.Close()

	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (2nd): %v", err)
	}
	defer s2.Close()

	dogs, err := s2.ListDogs(ctx)
	if err != nil {
		t.Fatalf("ListDogs: %v", err)
	}
	if len(dogs) != 2 {
		t.Fatalf("expected 2 dogs after reopen, got %d", len(dogs))
	}
}

func TestAddRecord(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	record, err := s.AddRecord(ctx, "leo", model.RecordVet, "定期健診(追加)")
	if err != nil {
		t.Fatalf("AddRecord: %v", err)
	}
	if record.ID == "" {
		t.Fatal("expected a generated id")
	}
	if record.Label != "定期健診(追加)" {
		t.Fatalf("unexpected label: %s", record.Label)
	}

	dogs, err := s.ListDogs(ctx)
	if err != nil {
		t.Fatalf("ListDogs: %v", err)
	}
	if len(dogs[0].Records) != 4 {
		t.Fatalf("expected 4 records for leo after adding one, got %d", len(dogs[0].Records))
	}
}

func TestDogExists(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	exists, err := s.DogExists(ctx, "leo")
	if err != nil || !exists {
		t.Fatalf("expected leo to exist, err=%v exists=%v", err, exists)
	}

	exists, err = s.DogExists(ctx, "nonexistent")
	if err != nil || exists {
		t.Fatalf("expected nonexistent dog to not exist, err=%v exists=%v", err, exists)
	}
}

func TestCreateAndListWalks(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	start := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	points := []model.GeoPoint{
		{Lat: 35.0, Lng: 139.0, Timestamp: start},
		{Lat: 35.001, Lng: 139.001, Timestamp: start.Add(time.Minute)},
	}
	created, err := s.CreateWalk(ctx, "leo", start, 600, 850.5, points)
	if err != nil {
		t.Fatalf("CreateWalk: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected a generated walk id")
	}

	walks, err := s.ListWalks(ctx, "leo")
	if err != nil {
		t.Fatalf("ListWalks: %v", err)
	}
	if len(walks) != 1 {
		t.Fatalf("expected 1 walk, got %d", len(walks))
	}
	if walks[0].DistanceMeters != 850.5 {
		t.Fatalf("unexpected distance: %v", walks[0].DistanceMeters)
	}
	if len(walks[0].Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(walks[0].Points))
	}
	if !walks[0].Points[0].Timestamp.Equal(start) {
		t.Fatalf("unexpected first point timestamp: %v", walks[0].Points[0].Timestamp)
	}

	// A different dog's walk list must stay empty.
	noaWalks, err := s.ListWalks(ctx, "noa")
	if err != nil {
		t.Fatalf("ListWalks(noa): %v", err)
	}
	if len(noaWalks) != 0 {
		t.Fatalf("expected 0 walks for noa, got %d", len(noaWalks))
	}
}
