package claude

import (
	"testing"

	"dogapp-api/internal/model"
)

func TestParseResult(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		want    model.AICheckResult
		wantErr bool
	}{
		{
			name: "clean JSON",
			text: `{"level": "normal", "title": "問題なし", "detail": "特に異常はありません"}`,
			want: model.AICheckResult{Level: model.LevelNormal, Title: "問題なし", Detail: "特に異常はありません"},
		},
		{
			name: "wrapped in a code fence and prose",
			text: "以下が結果です:\n```json\n{\"level\": \"watch\", \"title\": \"軽度の乾燥\", \"detail\": \"様子を見てください\"}\n```\nご参考まで。",
			want: model.AICheckResult{Level: model.LevelWatch, Title: "軽度の乾燥", Detail: "様子を見てください"},
		},
		{
			name:    "no JSON object",
			text:    "申し訳ありませんが、判定できません。",
			wantErr: true,
		},
		{
			name:    "invalid level value",
			text:    `{"level": "unknown", "title": "t", "detail": "d"}`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			text:    `{"level": "normal", "title": }`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseResult(tc.text)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got result %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
