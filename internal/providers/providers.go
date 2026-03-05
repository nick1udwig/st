package providers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ttstt/st/internal/config"
)

// TranscribeRequest models provider-agnostic STT options.
type TranscribeRequest struct {
	FilePath       string
	Model          string
	Language       string
	Prompt         string
	ResponseFormat string
	Temperature    *float64
	IncludeLogprob bool
}

type TranscribeResult struct {
	Text    string
	RawJSON string
}

type StreamEvent struct {
	Type    string
	Delta   string
	Text    string
	RawJSON string
}

// SynthesizeRequest models provider-agnostic TTS options.
type SynthesizeRequest struct {
	Text         string
	Model        string
	Voice        string
	Instructions string
	Format       string
	Speed        *float64
}

type SynthesizeResult struct {
	Audio       []byte
	ContentType string
}

// Provider defines the integration contract for TTS + STT backends.
type Provider interface {
	Name() string
	Transcribe(context.Context, TranscribeRequest) (TranscribeResult, error)
	TranscribeStream(context.Context, TranscribeRequest, func(StreamEvent) error) (TranscribeResult, error)
	Synthesize(context.Context, SynthesizeRequest) (SynthesizeResult, error)
}

type Factory func(config.Config) (Provider, error)

var registry = map[string]Factory{}

func Register(name string, f Factory) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		panic("provider name is empty")
	}
	if f == nil {
		panic("provider factory is nil")
	}
	if _, exists := registry[key]; exists {
		panic("duplicate provider registration: " + key)
	}
	registry[key] = f
}

func New(name string, cfg config.Config) (Provider, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	factory, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return factory(cfg)
}

func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
