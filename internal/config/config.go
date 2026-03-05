package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	defaultDirName  = ".st"
	defaultFileName = "config.toml"
)

// Config is the source-of-truth runtime configuration loaded from disk.
type Config struct {
	PreferredProvider string       `toml:"preferred_provider"`
	OpenAI            OpenAIConfig `toml:"openai"`
	Tools             ToolsConfig  `toml:"tools"`
}

type OpenAIConfig struct {
	APIKey       string  `toml:"api_key"`
	APIKeyEnv    string  `toml:"api_key_env"`
	BaseURL      string  `toml:"base_url"`
	Organization string  `toml:"organization"`
	Project      string  `toml:"project"`
	STTModel     string  `toml:"stt_model"`
	STTFormat    string  `toml:"stt_response_format"`
	TTSModel     string  `toml:"tts_model"`
	TTSVoice     string  `toml:"tts_voice"`
	TTSFormat    string  `toml:"tts_format"`
	TTSSpeed     float64 `toml:"tts_speed"`
}

type ToolsConfig struct {
	FFmpegPath string `toml:"ffmpeg_path"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, defaultDirName, defaultFileName), nil
}

func Default() Config {
	return Config{
		PreferredProvider: "openai",
		OpenAI: OpenAIConfig{
			APIKeyEnv: "OPENAI_API_KEY",
			STTModel:  "gpt-4o-transcribe",
			STTFormat: "json",
			TTSModel:  "gpt-4o-mini-tts",
			TTSVoice:  "marin",
			TTSFormat: "mp3",
			TTSSpeed:  1.0,
		},
		Tools: ToolsConfig{
			FFmpegPath: "ffmpeg",
		},
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("config not found at %s (run: st config init)", path)
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

func Init(path string) error {
	cfg := Default()
	cfg.Normalize()

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists at %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config path: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	payload := renderTemplate(cfg)
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (c *Config) Normalize() {
	def := Default()
	if strings.TrimSpace(c.PreferredProvider) == "" {
		c.PreferredProvider = def.PreferredProvider
	}
	if strings.TrimSpace(c.OpenAI.APIKeyEnv) == "" {
		c.OpenAI.APIKeyEnv = def.OpenAI.APIKeyEnv
	}
	if strings.TrimSpace(c.OpenAI.STTModel) == "" {
		c.OpenAI.STTModel = def.OpenAI.STTModel
	}
	if strings.TrimSpace(c.OpenAI.STTFormat) == "" {
		c.OpenAI.STTFormat = def.OpenAI.STTFormat
	}
	if strings.TrimSpace(c.OpenAI.TTSModel) == "" {
		c.OpenAI.TTSModel = def.OpenAI.TTSModel
	}
	if strings.TrimSpace(c.OpenAI.TTSVoice) == "" {
		c.OpenAI.TTSVoice = def.OpenAI.TTSVoice
	}
	if strings.TrimSpace(c.OpenAI.TTSFormat) == "" {
		c.OpenAI.TTSFormat = def.OpenAI.TTSFormat
	}
	if c.OpenAI.TTSSpeed == 0 {
		c.OpenAI.TTSSpeed = def.OpenAI.TTSSpeed
	}
	if strings.TrimSpace(c.Tools.FFmpegPath) == "" {
		c.Tools.FFmpegPath = def.Tools.FFmpegPath
	}
}

func (c Config) OpenAIResolvedAPIKey() (string, error) {
	if key := strings.TrimSpace(c.OpenAI.APIKey); key != "" {
		return key, nil
	}
	envName := strings.TrimSpace(c.OpenAI.APIKeyEnv)
	if envName == "" {
		envName = "OPENAI_API_KEY"
	}
	if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("openai api key is not configured (set openai.api_key or %s)", envName)
}

func renderTemplate(cfg Config) string {
	return fmt.Sprintf(`# st configuration
# Source of truth: %s/%s

preferred_provider = %q

[openai]
# Choose one of:
# 1) Set api_key directly
# 2) Leave api_key empty and set api_key_env to an environment variable name
api_key = %q
api_key_env = %q

# Optional overrides
base_url = %q
organization = %q
project = %q

# Defaults for commands
stt_model = %q
stt_response_format = %q
tts_model = %q
tts_voice = %q
tts_format = %q
tts_speed = %.2f

[tools]
ffmpeg_path = %q
`,
		defaultDirName, defaultFileName,
		cfg.PreferredProvider,
		cfg.OpenAI.APIKey,
		cfg.OpenAI.APIKeyEnv,
		cfg.OpenAI.BaseURL,
		cfg.OpenAI.Organization,
		cfg.OpenAI.Project,
		cfg.OpenAI.STTModel,
		cfg.OpenAI.STTFormat,
		cfg.OpenAI.TTSModel,
		cfg.OpenAI.TTSVoice,
		cfg.OpenAI.TTSFormat,
		cfg.OpenAI.TTSSpeed,
		cfg.Tools.FFmpegPath,
	)
}
