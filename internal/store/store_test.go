package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"dogapp-api/internal/model"
	"dogapp-api/internal/store/storetest"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), storetest.NewDSN(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// countDogs is a white-box helper: seeded dogs have no owner (they predate
// login), so they're invisible to the now owner-scoped ListDogs. Verifying
// they landed in the table is done with a direct query instead.
func countDogs(t *testing.T, s *Store) int {
	t.Helper()
	var count int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM dogs`).Scan(&count); err != nil {
		t.Fatalf("count dogs: %v", err)
	}
	return count
}

func TestOpenSeedsSampleDogs(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if count := countDogs(t, s); count != 2 {
		t.Fatalf("expected 2 seeded dogs, got %d", count)
	}

	weights, err := s.weightHistory(ctx, "leo")
	if err != nil {
		t.Fatalf("weightHistory: %v", err)
	}
	if len(weights) != 6 {
		t.Fatalf("expected 6 weight entries for leo, got %d", len(weights))
	}

	records, err := s.healthRecords(ctx, "leo")
	if err != nil {
		t.Fatalf("healthRecords: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records for leo, got %d", len(records))
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	// Re-opening an existing (non-empty) database must not duplicate seed data.
	dsn := storetest.NewDSN(t)
	ctx := context.Background()

	s1, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open (1st): %v", err)
	}
	s1.Close()

	s2, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open (2nd): %v", err)
	}
	defer s2.Close()

	if count := countDogs(t, s2); count != 2 {
		t.Fatalf("expected 2 dogs after reopen, got %d", count)
	}
}

func TestCreateUserAndFindByEmail(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	user, err := s.CreateUser(ctx, "owner@example.com", "hashed-password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID == "" {
		t.Fatal("expected a generated id")
	}
	if user.Email != "owner@example.com" {
		t.Fatalf("unexpected email: %s", user.Email)
	}

	found, hash, err := s.FindUserByEmail(ctx, "owner@example.com")
	if err != nil {
		t.Fatalf("FindUserByEmail: %v", err)
	}
	if found.ID != user.ID {
		t.Fatalf("expected same user id, got %s vs %s", found.ID, user.ID)
	}
	if hash != "hashed-password" {
		t.Fatalf("unexpected password hash: %s", hash)
	}
}

func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateUser(ctx, "dup@example.com", "hash1"); err != nil {
		t.Fatalf("CreateUser (1st): %v", err)
	}
	_, err := s.CreateUser(ctx, "dup@example.com", "hash2")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestFindUserByEmailNotFound(t *testing.T) {
	s := openTestStore(t)

	_, _, err := s.FindUserByEmail(context.Background(), "nobody@example.com")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// A nil slice would marshal to JSON `null`, which the Flutter client can't
// cast to List<dynamic> - every fresh signup hits this since they start
// with zero dogs, so it must come back as `[]`.
func TestListDogsReturnsEmptySliceNotNilForNewUser(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	user, err := s.CreateUser(ctx, "nodogs@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	dogs, err := s.ListDogs(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListDogs: %v", err)
	}
	if dogs == nil {
		t.Fatal("expected a non-nil empty slice, got nil")
	}
	if len(dogs) != 0 {
		t.Fatalf("expected 0 dogs, got %d", len(dogs))
	}
}

func TestCreateDogIsOwnedByCreator(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	user, err := s.CreateUser(ctx, "owner2@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	dog, err := s.CreateDog(ctx, user.ID, "Mochi", "Shiba", "Cream", 2023)
	if err != nil {
		t.Fatalf("CreateDog: %v", err)
	}
	if dog.ID == "" {
		t.Fatal("expected a generated dog id")
	}

	dogs, err := s.ListDogs(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListDogs: %v", err)
	}
	if len(dogs) != 1 || dogs[0].ID != dog.ID {
		t.Fatalf("expected only the created dog in owner's list, got %+v", dogs)
	}

	// A different (or nonexistent) owner must not see it.
	otherDogs, err := s.ListDogs(ctx, "someone-else")
	if err != nil {
		t.Fatalf("ListDogs(other): %v", err)
	}
	if len(otherDogs) != 0 {
		t.Fatalf("expected 0 dogs for a different owner, got %d", len(otherDogs))
	}
}

func TestUpdateDogRequiresOwnership(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	user, err := s.CreateUser(ctx, "owner3@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dog, err := s.CreateDog(ctx, user.ID, "Coco", "Poodle", "Black", 2020)
	if err != nil {
		t.Fatalf("CreateDog: %v", err)
	}

	if _, err := s.UpdateDog(ctx, dog.ID, "someone-else", "x", "x", "x", 2020); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for a non-owner update, got %v", err)
	}

	updated, err := s.UpdateDog(ctx, dog.ID, user.ID, "Coco2", "Poodle", "White", 2020)
	if err != nil {
		t.Fatalf("UpdateDog: %v", err)
	}
	if updated.Name != "Coco2" || updated.Color != "White" {
		t.Fatalf("unexpected updated dog: %+v", updated)
	}
}

func TestDogOwnedBy(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	user, err := s.CreateUser(ctx, "owner4@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dog, err := s.CreateDog(ctx, user.ID, "Leo", "Poodle", "Apricot", 2021)
	if err != nil {
		t.Fatalf("CreateDog: %v", err)
	}

	if owned, err := s.DogOwnedBy(ctx, dog.ID, user.ID); err != nil || !owned {
		t.Fatalf("expected dog to be owned by its creator, err=%v owned=%v", err, owned)
	}
	if owned, err := s.DogOwnedBy(ctx, dog.ID, "someone-else"); err != nil || owned {
		t.Fatalf("expected dog to not be owned by a different user, err=%v owned=%v", err, owned)
	}
	if owned, err := s.DogOwnedBy(ctx, "nonexistent", user.ID); err != nil || owned {
		t.Fatalf("expected a nonexistent dog to not be owned, err=%v owned=%v", err, owned)
	}
}

func TestAddRecord(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	cost := 4500.0
	record, err := s.AddRecord(ctx, "leo", model.RecordVet, "定期健診(追加)", &cost)
	if err != nil {
		t.Fatalf("AddRecord: %v", err)
	}
	if record.ID == "" {
		t.Fatal("expected a generated id")
	}
	if record.Label != "定期健診(追加)" {
		t.Fatalf("unexpected label: %s", record.Label)
	}
	if record.Cost == nil || *record.Cost != 4500.0 {
		t.Fatalf("unexpected cost: %v", record.Cost)
	}

	records, err := s.healthRecords(ctx, "leo")
	if err != nil {
		t.Fatalf("healthRecords: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected 4 records for leo after adding one, got %d", len(records))
	}
	// The seeded records also carry a cost - verify it survives the round trip.
	var found bool
	for _, r := range records {
		if r.ID == "1" {
			found = true
			if r.Cost == nil || *r.Cost != 8000.0 {
				t.Fatalf("unexpected seeded cost for record 1: %v", r.Cost)
			}
		}
	}
	if !found {
		t.Fatal("expected seeded record with id 1")
	}
}

func TestAddRecordWithoutCost(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	record, err := s.AddRecord(ctx, "leo", model.RecordAICheck, "健康チェック: 問題なし", nil)
	if err != nil {
		t.Fatalf("AddRecord: %v", err)
	}
	if record.Cost != nil {
		t.Fatalf("expected nil cost, got %v", *record.Cost)
	}

	records, err := s.healthRecords(ctx, "leo")
	if err != nil {
		t.Fatalf("healthRecords: %v", err)
	}
	for _, r := range records {
		if r.ID == record.ID && r.Cost != nil {
			t.Fatalf("expected nil cost after round trip, got %v", *r.Cost)
		}
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
