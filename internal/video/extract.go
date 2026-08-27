// Package video extracts still frames from a short clip so they can be sent
// to Claude's vision API, which accepts images but not raw video.
package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrFFmpegNotFound is returned when ffmpeg isn't on PATH. Frame extraction
// shells out to it rather than pulling in a video-decoding dependency,
// since ffmpeg is the standard tool for this and Go has no general-purpose
// video codec support in std or via a lightweight pure-Go package.
var ErrFFmpegNotFound = errors.New("ffmpeg not found on PATH; install it (e.g. `brew install ffmpeg`) to use gait-check")

// ExtractFrames writes videoBytes to a temp file and runs ffmpeg to pull up
// to frameCount JPEG frames, sampled every 30th frame (roughly one per
// second at typical 30fps clips), and returns their bytes in order.
func ExtractFrames(ctx context.Context, videoBytes []byte, frameCount int) ([][]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, ErrFFmpegNotFound
	}

	dir, err := os.MkdirTemp("", "dogapp-gait-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	inputPath := filepath.Join(dir, "input.mp4")
	if err := os.WriteFile(inputPath, videoBytes, 0o600); err != nil {
		return nil, fmt.Errorf("write temp video: %w", err)
	}

	outputPattern := filepath.Join(dir, "frame_%02d.jpg")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-loglevel", "error",
		"-i", inputPath,
		"-vf", `select='not(mod(n\,30))'`,
		"-vsync", "vfr",
		"-frames:v", fmt.Sprintf("%d", frameCount),
		outputPattern,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w: %s", err, output)
	}

	var frames [][]byte
	for i := 1; i <= frameCount; i++ {
		path := filepath.Join(dir, fmt.Sprintf("frame_%02d.jpg", i))
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				break // shorter clip than expected; fewer frames is fine
			}
			return nil, err
		}
		frames = append(frames, data)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("ffmpeg extracted no frames from the video")
	}
	return frames, nil
}
