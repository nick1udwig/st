package media

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxUploadBytes = 25 * 1024 * 1024

var supportedInputExtensions = map[string]struct{}{
	"flac": {},
	"mp3":  {},
	"mp4":  {},
	"mpeg": {},
	"mpga": {},
	"m4a":  {},
	"ogg":  {},
	"wav":  {},
	"webm": {},
}

type PreparedInput struct {
	Path      string
	Converted bool
	Cleanup   func()
}

func PrepareTranscriptionInput(path, ffmpegPath string) (PreparedInput, error) {
	if strings.TrimSpace(path) == "" {
		return PreparedInput{}, errors.New("input file path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return PreparedInput{}, fmt.Errorf("stat input file: %w", err)
	}
	if info.IsDir() {
		return PreparedInput{}, fmt.Errorf("input path %q is a directory", path)
	}

	cleanup := func() {}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if _, ok := supportedInputExtensions[ext]; ok {
		if info.Size() > maxUploadBytes {
			return PreparedInput{}, fmt.Errorf("input is %.2f MB; OpenAI audio uploads are limited to 25 MB", float64(info.Size())/(1024*1024))
		}
		return PreparedInput{Path: path, Converted: false, Cleanup: cleanup}, nil
	}

	if strings.TrimSpace(ffmpegPath) == "" {
		ffmpegPath = "ffmpeg"
	}
	if _, err := exec.LookPath(ffmpegPath); err != nil {
		return PreparedInput{}, fmt.Errorf("unsupported input extension %q and ffmpeg is unavailable at %q", ext, ffmpegPath)
	}

	tmp, err := os.CreateTemp("", "st-stt-*.wav")
	if err != nil {
		return PreparedInput{}, fmt.Errorf("create temp output: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()

	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", path,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		tmpPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmpPath)
		if len(out) > 0 {
			return PreparedInput{}, fmt.Errorf("ffmpeg conversion failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return PreparedInput{}, fmt.Errorf("ffmpeg conversion failed: %w", err)
	}

	convertedInfo, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return PreparedInput{}, fmt.Errorf("stat converted file: %w", err)
	}
	if convertedInfo.Size() > maxUploadBytes {
		_ = os.Remove(tmpPath)
		return PreparedInput{}, fmt.Errorf("converted audio is %.2f MB; OpenAI audio uploads are limited to 25 MB", float64(convertedInfo.Size())/(1024*1024))
	}

	cleanup = func() { _ = os.Remove(tmpPath) }
	return PreparedInput{Path: tmpPath, Converted: true, Cleanup: cleanup}, nil
}
