// Package claude wraps the Anthropic API calls dogapp-api needs: judging a
// dog's skin/coat from a photo, and judging gait from a handful of video
// frames. Both return the same model.AICheckResult shape the Flutter client
// already expects from POST /dogs/{dogId}/ai-check and /gait-check.
package claude

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"dogapp-api/internal/model"
)

// requestTimeout bounds a single Claude call so a stalled upstream request
// can't hang a handler indefinitely (the inbound HTTP request's own
// deadline isn't a substitute - the client may keep the connection open
// far longer than a health judgment should reasonably take).
const requestTimeout = 60 * time.Second

// Checker is the interface handlers depend on, so tests can substitute a
// fake instead of calling the real Anthropic API.
type Checker interface {
	// CheckPhoto judges a single photo of the given body part ("skin",
	// "eye", "ear", or "mouth" - see bodyPartPrompts).
	CheckPhoto(ctx context.Context, imageBytes []byte, mediaType string, bodyPart string) (model.AICheckResult, error)
	CheckGaitFrames(ctx context.Context, frames [][]byte, mediaType string) (model.AICheckResult, error)
}

type AnthropicChecker struct {
	client anthropic.Client
}

// NewAnthropicChecker builds a checker backed by the real Claude API.
// It reads credentials the same way the SDK always does (ANTHROPIC_API_KEY
// env var, or another resolvable credential source) - see option.WithAPIKey
// only if you need to override that.
func NewAnthropicChecker() *AnthropicChecker {
	return &AnthropicChecker{client: anthropic.NewClient()}
}

// NewAnthropicCheckerWithKey is used when the key is supplied explicitly
// (e.g. read from a config file) rather than via the environment.
func NewAnthropicCheckerWithKey(apiKey string) *AnthropicChecker {
	return &AnthropicChecker{client: anthropic.NewClient(option.WithAPIKey(apiKey))}
}

const skinCheckPrompt = `あなたは犬の皮膚・被毛の写真を確認する獣医アシスタントです。
添付された写真を見て、皮膚の赤み・脱毛・乾燥・かゆみを示すしぐさの痕跡など、
気になる所見がないか簡易チェックしてください。これは診断ではなく、
動物病院に相談すべきかどうかの目安を提供するものです。

以下のJSON形式で、日本語で、それだけを出力してください(説明文やコードブロックの記法は不要です):
{"level": "normal" | "watch" | "concern", "title": "一言の見出し", "detail": "1〜2文の説明"}

- normal: 特に気になる所見がない
- watch: 軽度の乾燥・パサつきなど、様子見でよい所見がある
- concern: 赤み・脱毛・傷など、動物病院への相談を勧めるべき所見がある`

const eyeCheckPrompt = `あなたは犬の目の写真を確認する獣医アシスタントです。
添付された写真を見て、目やに・充血・涙やけ・白濁・腫れなど、
気になる所見がないか簡易チェックしてください。これは診断ではなく、
動物病院に相談すべきかどうかの目安を提供するものです。

以下のJSON形式で、日本語で、それだけを出力してください(説明文やコードブロックの記法は不要です):
{"level": "normal" | "watch" | "concern", "title": "一言の見出し", "detail": "1〜2文の説明"}

- normal: 特に気になる所見がない
- watch: 軽度の目やに・涙やけなど、様子見でよい所見がある
- concern: 強い充血・白濁・腫れなど、動物病院への相談を勧めるべき所見がある`

const earCheckPrompt = `あなたは犬の耳の写真を確認する獣医アシスタントです。
添付された写真を見て、耳垢の量や色・赤み・腫れ・においを示すような
汚れの付着など、気になる所見がないか簡易チェックしてください。
これは診断ではなく、動物病院に相談すべきかどうかの目安を提供するものです。

以下のJSON形式で、日本語で、それだけを出力してください(説明文やコードブロックの記法は不要です):
{"level": "normal" | "watch" | "concern", "title": "一言の見出し", "detail": "1〜2文の説明"}

- normal: 特に気になる所見がない
- watch: 軽度の耳垢の増加など、様子見でよい所見がある
- concern: 強い赤み・腫れ・大量の耳垢など、動物病院への相談を勧めるべき所見がある`

const mouthCheckPrompt = `あなたは犬の口の写真を確認する獣医アシスタントです。
添付された写真を見て、歯石・歯茎の腫れや変色・口臭を示唆する汚れ・
よだれの異常など、気になる所見がないか簡易チェックしてください。
これは診断ではなく、動物病院に相談すべきかどうかの目安を提供するものです。

以下のJSON形式で、日本語で、それだけを出力してください(説明文やコードブロックの記法は不要です):
{"level": "normal" | "watch" | "concern", "title": "一言の見出し", "detail": "1〜2文の説明"}

- normal: 特に気になる所見がない
- watch: 軽度の歯石の付着など、様子見でよい所見がある
- concern: 歯茎の腫れ・出血・強い口臭など、動物病院への相談を勧めるべき所見がある`

// Body part identifiers accepted by CheckPhoto - these are the wire values
// sent by dogapp_flutter's ai-check request body.
const (
	BodyPartSkin  = "skin"
	BodyPartEye   = "eye"
	BodyPartEar   = "ear"
	BodyPartMouth = "mouth"
)

var bodyPartPrompts = map[string]string{
	BodyPartSkin:  skinCheckPrompt,
	BodyPartEye:   eyeCheckPrompt,
	BodyPartEar:   earCheckPrompt,
	BodyPartMouth: mouthCheckPrompt,
}

// ValidBodyPart reports whether bodyPart is one of the photo check types
// CheckPhoto supports. Handlers use this to validate the request before
// calling CheckPhoto.
func ValidBodyPart(bodyPart string) bool {
	_, ok := bodyPartPrompts[bodyPart]
	return ok
}

const gaitCheckPrompt = `あなたは犬の歩き方(歩様)を確認する獣医アシスタントです。
添付された、短い動画から等間隔に抽出した複数枚の連続写真を見て、
足を引きずっていないか、歩様に左右差がないかなど、歩行の異常がないか
簡易チェックしてください。これは診断ではなく、動物病院に相談すべきかどうかの
目安を提供するものです。

以下のJSON形式で、日本語で、それだけを出力してください(説明文やコードブロックの記法は不要です):
{"level": "normal" | "watch" | "concern", "title": "一言の見出し", "detail": "1〜2文の説明"}

- normal: 左右バランスよく歩けている
- watch: わずかな左右差など、様子見でよい所見がある
- concern: 明らかに足を引きずる・かばうなど、動物病院への相談を勧めるべき所見がある`

func (c *AnthropicChecker) CheckPhoto(ctx context.Context, imageBytes []byte, mediaType string, bodyPart string) (model.AICheckResult, error) {
	prompt, ok := bodyPartPrompts[bodyPart]
	if !ok {
		return model.AICheckResult{}, fmt.Errorf("unknown body part %q", bodyPart)
	}
	return c.checkImages(ctx, prompt, [][]byte{imageBytes}, mediaType)
}

func (c *AnthropicChecker) CheckGaitFrames(ctx context.Context, frames [][]byte, mediaType string) (model.AICheckResult, error) {
	return c.checkImages(ctx, gaitCheckPrompt, frames, mediaType)
}

func (c *AnthropicChecker) checkImages(ctx context.Context, prompt string, images [][]byte, mediaType string) (model.AICheckResult, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(images)+1)
	for _, img := range images {
		blocks = append(blocks, anthropic.NewImageBlockBase64(mediaType, base64.StdEncoding.EncodeToString(img)))
	}
	blocks = append(blocks, anthropic.NewTextBlock(prompt))

	message, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     "claude-opus-5",
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(blocks...),
		},
	})
	if err != nil {
		return model.AICheckResult{}, fmt.Errorf("claude request failed: %w", err)
	}

	var text strings.Builder
	for _, block := range message.Content {
		if b, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(b.Text)
		}
	}

	return parseResult(text.String())
}

// parseResult tolerates Claude wrapping the JSON in prose or a code fence by
// extracting the outermost {...} span before decoding.
func parseResult(text string) (model.AICheckResult, error) {
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start == -1 || end == -1 || end < start {
		return model.AICheckResult{}, fmt.Errorf("no JSON object found in Claude response: %q", text)
	}

	var result model.AICheckResult
	if err := json.Unmarshal([]byte(text[start:end+1]), &result); err != nil {
		return model.AICheckResult{}, fmt.Errorf("parse Claude response: %w", err)
	}
	switch result.Level {
	case model.LevelNormal, model.LevelWatch, model.LevelConcern:
	default:
		return model.AICheckResult{}, fmt.Errorf("unexpected level %q in Claude response", result.Level)
	}
	return result, nil
}
