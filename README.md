# st

`st` is a Go CLI for speech-to-text (`stt`) and text-to-speech (`tts`) with pluggable provider integrations.

This repository currently includes:
- OpenAI provider (official Go SDK)
- Batch and streaming transcription
- ffmpeg fallback conversion for unsupported input file extensions
- Disk-backed TOML config at `~/.st/config.toml`

## Install

```bash
go build -o st ./cmd/st
```

## Initialize config

```bash
./st config init
```

This creates `~/.st/config.toml`.

Set your API key either:
- In config: `openai.api_key = "..."`
- Via env var (default): `OPENAI_API_KEY`

## Usage

### Transcribe audio (batch)

```bash
./st stt ./audio.mp3
./st stt ./audio.wav -o transcript.txt
```

### Transcribe audio (streaming)

```bash
./st stt ./audio.mp3 --stream
```

### Synthesize speech from text file

```bash
./st tts ./script.txt -o speech.mp3
```

### Synthesize speech from raw text

```bash
./st tts -t "hello? can you hear me?" > speech.mp3
./st tts -t "hello? can you hear me?" -o speech.mp3
```

## Provider architecture

Providers implement `internal/providers.Provider` and register themselves via `providers.Register(name, factory)`.

Add another provider by:
1. Implementing the interface in a new package under `internal/providers/<name>`
2. Registering it in `init()`
3. Adding provider-specific config section support

## Notes

- OpenAI upload limit is 25 MB per transcription request.
- If an input extension is unsupported, `st` attempts conversion via ffmpeg to `wav` automatically.
- Without `-o`, commands write to stdout.
