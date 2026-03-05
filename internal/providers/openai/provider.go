package openai

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/ttstt/st/internal/config"
	"github.com/ttstt/st/internal/providers"
)

const name = "openai"

type Provider struct {
	client openai.Client
}

func init() {
	providers.Register(name, NewFromConfig)
}

func NewFromConfig(cfg config.Config) (providers.Provider, error) {
	apiKey, err := cfg.OpenAIResolvedAPIKey()
	if err != nil {
		return nil, err
	}

	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if v := strings.TrimSpace(cfg.OpenAI.BaseURL); v != "" {
		opts = append(opts, option.WithBaseURL(v))
	}
	if v := strings.TrimSpace(cfg.OpenAI.Organization); v != "" {
		opts = append(opts, option.WithOrganization(v))
	}
	if v := strings.TrimSpace(cfg.OpenAI.Project); v != "" {
		opts = append(opts, option.WithProject(v))
	}

	client := openai.NewClient(opts...)
	return &Provider{client: client}, nil
}

func (p *Provider) Name() string { return name }

func (p *Provider) Transcribe(ctx context.Context, req providers.TranscribeRequest) (providers.TranscribeResult, error) {
	params, closer, err := buildTranscribeParams(req)
	if err != nil {
		return providers.TranscribeResult{}, err
	}
	defer closer()

	res, err := p.client.Audio.Transcriptions.New(ctx, params)
	if err != nil {
		return providers.TranscribeResult{}, fmt.Errorf("transcription request failed: %w", err)
	}

	return providers.TranscribeResult{
		Text:    res.Text,
		RawJSON: res.RawJSON(),
	}, nil
}

func (p *Provider) TranscribeStream(
	ctx context.Context,
	req providers.TranscribeRequest,
	onEvent func(providers.StreamEvent) error,
) (providers.TranscribeResult, error) {
	params, closer, err := buildTranscribeParams(req)
	if err != nil {
		return providers.TranscribeResult{}, err
	}
	defer closer()

	stream := p.client.Audio.Transcriptions.NewStreaming(ctx, params)
	defer stream.Close()

	var collected strings.Builder
	finalText := ""

	for stream.Next() {
		eventUnion := stream.Current()
		switch event := eventUnion.AsAny().(type) {
		case openai.TranscriptionTextDeltaEvent:
			collected.WriteString(event.Delta)
			if onEvent != nil {
				err := onEvent(providers.StreamEvent{
					Type:    string(event.Type),
					Delta:   event.Delta,
					RawJSON: event.RawJSON(),
				})
				if err != nil {
					return providers.TranscribeResult{}, err
				}
			}
		case openai.TranscriptionTextDoneEvent:
			if event.Text != "" {
				finalText = event.Text
			}
			if onEvent != nil {
				err := onEvent(providers.StreamEvent{
					Type:    string(event.Type),
					Text:    event.Text,
					RawJSON: event.RawJSON(),
				})
				if err != nil {
					return providers.TranscribeResult{}, err
				}
			}
		default:
			if onEvent != nil {
				err := onEvent(providers.StreamEvent{
					Type:    eventUnion.Type,
					Delta:   eventUnion.Delta,
					Text:    eventUnion.Text,
					RawJSON: eventUnion.RawJSON(),
				})
				if err != nil {
					return providers.TranscribeResult{}, err
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return providers.TranscribeResult{}, fmt.Errorf("streaming transcription failed: %w", err)
	}

	if finalText == "" {
		finalText = collected.String()
	}

	return providers.TranscribeResult{Text: finalText}, nil
}

func (p *Provider) Synthesize(ctx context.Context, req providers.SynthesizeRequest) (providers.SynthesizeResult, error) {
	params := openai.AudioSpeechNewParams{
		Input: req.Text,
		Model: openai.SpeechModel(req.Model),
		Voice: openai.AudioSpeechNewParamsVoice(req.Voice),
	}
	if req.Instructions != "" {
		params.Instructions = openai.String(req.Instructions)
	}
	if req.Speed != nil {
		params.Speed = openai.Float(*req.Speed)
	}
	if req.Format != "" {
		params.ResponseFormat = openai.AudioSpeechNewParamsResponseFormat(req.Format)
	}

	res, err := p.client.Audio.Speech.New(ctx, params)
	if err != nil {
		return providers.SynthesizeResult{}, fmt.Errorf("speech request failed: %w", err)
	}
	defer res.Body.Close()

	audio, err := io.ReadAll(res.Body)
	if err != nil {
		return providers.SynthesizeResult{}, fmt.Errorf("read speech response: %w", err)
	}

	return providers.SynthesizeResult{
		Audio:       audio,
		ContentType: res.Header.Get("Content-Type"),
	}, nil
}

func buildTranscribeParams(req providers.TranscribeRequest) (openai.AudioTranscriptionNewParams, func(), error) {
	file, err := os.Open(req.FilePath)
	if err != nil {
		return openai.AudioTranscriptionNewParams{}, nil, fmt.Errorf("open audio file: %w", err)
	}

	params := openai.AudioTranscriptionNewParams{
		File:  file,
		Model: openai.AudioModel(req.Model),
	}
	if req.Language != "" {
		params.Language = openai.String(req.Language)
	}
	if req.Prompt != "" {
		params.Prompt = openai.String(req.Prompt)
	}
	if req.ResponseFormat != "" {
		params.ResponseFormat = openai.AudioResponseFormat(req.ResponseFormat)
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	if req.IncludeLogprob {
		params.Include = []openai.TranscriptionInclude{openai.TranscriptionIncludeLogprobs}
	}

	cleanup := func() {
		_ = file.Close()
	}
	return params, cleanup, nil
}
