// Package model defines the domain types shared across dogapp-api.
// Field names and JSON shapes mirror dogapp_flutter's Dart models
// (lib/models/dog.dart, lib/models/walk.dart) exactly, since that Flutter
// client is the only consumer of this API today.
package model

import "time"

type RecordType string

const (
	RecordVaccine    RecordType = "vaccine"
	RecordGrooming   RecordType = "grooming"
	RecordVet        RecordType = "vet"
	RecordMedication RecordType = "medication"
	RecordAICheck    RecordType = "aiCheck"
)

type WeightEntry struct {
	Month string  `json:"month"`
	Kg    float64 `json:"kg"`
}

type HealthRecord struct {
	ID    string     `json:"id"`
	Type  RecordType `json:"type"`
	Label string     `json:"label"`
	Date  time.Time  `json:"date"`
}

type Dog struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Breed         string         `json:"breed"`
	Color         string         `json:"color"`
	BirthYear     int            `json:"birthYear"`
	WeightHistory []WeightEntry  `json:"weightHistory"`
	Records       []HealthRecord `json:"records"`
}

type AICheckLevel string

const (
	LevelNormal  AICheckLevel = "normal"
	LevelWatch   AICheckLevel = "watch"
	LevelConcern AICheckLevel = "concern"
)

type AICheckResult struct {
	Level  AICheckLevel `json:"level"`
	Title  string       `json:"title"`
	Detail string       `json:"detail"`
}

type GeoPoint struct {
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Timestamp time.Time `json:"timestamp"`
}

type WalkRoute struct {
	ID              string     `json:"id"`
	DogID           string     `json:"dogId"`
	StartedAt       time.Time  `json:"startedAt"`
	DurationSeconds int        `json:"durationSeconds"`
	DistanceMeters  float64    `json:"distanceMeters"`
	Points          []GeoPoint `json:"points"`
}
