package store

import (
	"time"

	"dogapp-api/internal/model"
)

// seedDogs mirrors dogapp_flutter's lib/data/mock_data.dart so a fresh
// database starts with the same two dogs the Flutter app's own mock data
// (and the dev mock server) uses.
var seedDogs = []model.Dog{
	{
		ID:        "leo",
		Name:      "レオ",
		Breed:     "スタンダードプードル",
		Color:     "アプリコット",
		BirthYear: 2021,
		WeightHistory: []model.WeightEntry{
			{Month: "3月", Kg: 24.8},
			{Month: "4月", Kg: 25.1},
			{Month: "5月", Kg: 24.9},
			{Month: "6月", Kg: 25.3},
			{Month: "7月", Kg: 25.4},
			{Month: "8月", Kg: 25.2},
		},
		Records: []model.HealthRecord{
			{ID: "1", Type: model.RecordVaccine, Label: "混合ワクチン接種", Date: time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)},
			{ID: "2", Type: model.RecordGrooming, Label: "トリミング(サマーカット)", Date: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)},
			{ID: "3", Type: model.RecordVet, Label: "定期健診", Date: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)},
		},
	},
	{
		ID:        "noa",
		Name:      "ノア",
		Breed:     "スタンダードプードル",
		Color:     "ブラック",
		BirthYear: 2022,
		WeightHistory: []model.WeightEntry{
			{Month: "3月", Kg: 22.1},
			{Month: "4月", Kg: 22.3},
			{Month: "5月", Kg: 22.6},
			{Month: "6月", Kg: 22.4},
			{Month: "7月", Kg: 22.8},
			{Month: "8月", Kg: 23.0},
		},
		Records: []model.HealthRecord{
			{ID: "1", Type: model.RecordGrooming, Label: "トリミング(全身カット)", Date: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)},
			{ID: "2", Type: model.RecordVaccine, Label: "狂犬病予防接種", Date: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)},
		},
	},
}
