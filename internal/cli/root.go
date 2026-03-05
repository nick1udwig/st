package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ttstt/st/internal/config"
	"github.com/ttstt/st/internal/media"
	"github.com/ttstt/st/internal/providers"
	_ "github.com/ttstt/st/internal/providers/openai"
)

type App struct {
	stdout io.Writer
	stderr io.Writer
}

func New(stdout, stderr io.Writer) *App {
	return &App{stdout: stdout, stderr: stderr}
}

func (a *App) RootCommand() *cobra.Command {
	defaultConfigPath, err := config.DefaultPath()
	if err != nil {
		defaultConfigPath = "~/.st/config.toml"
	}

	var configPath string
	var providerOverride string

	rootCmd := &cobra.Command{
		Use:   "st",
		Short: "Speech-to-text and text-to-speech CLI",
	}
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	rootCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath, "path to config TOML")
	rootCmd.PersistentFlags().StringVar(&providerOverride, "provider", "", "provider override (e.g. openai)")

	rootCmd.AddCommand(a.newConfigCmd(&configPath))
	rootCmd.AddCommand(a.newSTTCmd(&configPath, &providerOverride))
	rootCmd.AddCommand(a.newTTSCmd(&configPath, &providerOverride))
	rootCmd.AddCommand(a.newProvidersCmd())

	return rootCmd
}

func (a *App) newProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List installed provider integrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, name := range providers.Names() {
				fmt.Fprintln(a.stdout, name)
			}
			return nil
		},
	}
}

func (a *App) newConfigCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage local configuration",
	}

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the resolved config path",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(a.stdout, resolveConfigPath(*configPath))
			return nil
		},
	}

	var force bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create ~/.st/config.toml with defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := resolveConfigPath(*configPath)
			if force {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove existing config: %w", err)
				}
			}
			if err := config.Init(path); err != nil {
				return err
			}
			fmt.Fprintf(a.stdout, "initialized config at %s\n", path)
			return nil
		},
	}
	initCmd.Flags().BoolVar(&force, "force", false, "overwrite existing config")

	cmd.AddCommand(initCmd)
	cmd.AddCommand(pathCmd)
	return cmd
}

func (a *App) newSTTCmd(configPath *string, providerOverride *string) *cobra.Command {
	var outputPath string
	var model string
	var language string
	var prompt string
	var responseFormat string
	var stream bool
	var includeLogprobs bool
	var temperature float64

	cmd := &cobra.Command{
		Use:   "stt <audio-file>",
		Short: "Transcribe audio to text",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolveConfigPath(*configPath))
			if err != nil {
				return err
			}

			providerName := providerFromArgs(cfg, *providerOverride)
			provider, err := providers.New(providerName, cfg)
			if err != nil {
				return err
			}

			prepared, err := media.PrepareTranscriptionInput(args[0], cfg.Tools.FFmpegPath)
			if err != nil {
				return err
			}
			defer prepared.Cleanup()
			if prepared.Converted {
				fmt.Fprintf(a.stderr, "converted %s to %s using ffmpeg\n", args[0], prepared.Path)
			}

			req := providers.TranscribeRequest{
				FilePath:       prepared.Path,
				Model:          firstNonEmpty(model, cfg.OpenAI.STTModel),
				Language:       language,
				Prompt:         prompt,
				ResponseFormat: firstNonEmpty(responseFormat, cfg.OpenAI.STTFormat),
				IncludeLogprob: includeLogprobs,
			}
			if cmd.Flags().Changed("temperature") {
				req.Temperature = &temperature
			}

			writer, closeFn, err := openOutputWriter(outputPath)
			if err != nil {
				return err
			}
			defer closeFn()

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			if stream {
				var sawDelta bool
				result, err := provider.TranscribeStream(ctx, req, func(ev providers.StreamEvent) error {
					if ev.Delta != "" {
						sawDelta = true
						_, werr := io.WriteString(writer, ev.Delta)
						return werr
					}
					if ev.Text != "" && !sawDelta {
						_, werr := io.WriteString(writer, ev.Text)
						return werr
					}
					return nil
				})
				if err != nil {
					return err
				}
				if !sawDelta && result.Text != "" {
					_, err = io.WriteString(writer, result.Text)
					if err != nil {
						return err
					}
				}
				if outputPath == "" {
					if _, err := io.WriteString(writer, "\n"); err != nil {
						return err
					}
				}
				return nil
			}

			result, err := provider.Transcribe(ctx, req)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(writer, result.Text); err != nil {
				return err
			}
			if outputPath == "" {
				if _, err := io.WriteString(writer, "\n"); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "output text file path (default: stdout)")
	cmd.Flags().StringVar(&model, "model", "", "STT model")
	cmd.Flags().StringVar(&language, "language", "", "language code (e.g. en)")
	cmd.Flags().StringVar(&prompt, "prompt", "", "optional prompt for transcription")
	cmd.Flags().StringVar(&responseFormat, "response-format", "", "response format (e.g. json)")
	cmd.Flags().BoolVar(&stream, "stream", false, "stream transcription deltas")
	cmd.Flags().BoolVar(&includeLogprobs, "include-logprobs", false, "include token logprobs when supported")
	cmd.Flags().Float64Var(&temperature, "temperature", 0, "sampling temperature (0-1)")
	return cmd
}

func (a *App) newTTSCmd(configPath *string, providerOverride *string) *cobra.Command {
	var outputPath string
	var textFlag string
	var model string
	var voice string
	var format string
	var instructions string
	var speed float64

	cmd := &cobra.Command{
		Use:   "tts [text-file]",
		Short: "Generate speech audio from text",
		Args: func(cmd *cobra.Command, args []string) error {
			if textFlag != "" && len(args) > 0 {
				return errors.New("provide either a text file argument or --txt, not both")
			}
			if textFlag == "" && len(args) == 0 {
				return errors.New("provide a text file argument or --txt")
			}
			if len(args) > 1 {
				return errors.New("only one text file argument is supported")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(resolveConfigPath(*configPath))
			if err != nil {
				return err
			}

			providerName := providerFromArgs(cfg, *providerOverride)
			provider, err := providers.New(providerName, cfg)
			if err != nil {
				return err
			}

			inputText, err := resolveTextInput(textFlag, args)
			if err != nil {
				return err
			}
			if strings.TrimSpace(inputText) == "" {
				return errors.New("input text is empty")
			}

			effectiveSpeed := cfg.OpenAI.TTSSpeed
			if cmd.Flags().Changed("speed") {
				effectiveSpeed = speed
			}

			req := providers.SynthesizeRequest{
				Text:         inputText,
				Model:        firstNonEmpty(model, cfg.OpenAI.TTSModel),
				Voice:        firstNonEmpty(voice, cfg.OpenAI.TTSVoice),
				Instructions: instructions,
				Format:       firstNonEmpty(format, cfg.OpenAI.TTSFormat),
				Speed:        &effectiveSpeed,
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			result, err := provider.Synthesize(ctx, req)
			if err != nil {
				return err
			}

			if outputPath != "" {
				return writeBinaryFile(outputPath, result.Audio)
			}
			_, err = a.stdout.Write(result.Audio)
			return err
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "output audio path (default: stdout)")
	cmd.Flags().StringVarP(&textFlag, "txt", "t", "", "raw input text")
	cmd.Flags().StringVar(&model, "model", "", "TTS model")
	cmd.Flags().StringVar(&voice, "voice", "", "voice name or id")
	cmd.Flags().StringVar(&format, "format", "", "audio format (mp3, wav, ...)")
	cmd.Flags().StringVar(&instructions, "instructions", "", "voice/style instructions")
	cmd.Flags().Float64Var(&speed, "speed", 1.0, "speech speed (0.25 - 4.0)")
	return cmd
}

func resolveTextInput(flagValue string, args []string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if len(args) == 0 {
		return "", errors.New("no text input provided")
	}
	if args[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return "", fmt.Errorf("read text file: %w", err)
	}
	return string(data), nil
}

func providerFromArgs(cfg config.Config, override string) string {
	if v := strings.TrimSpace(override); v != "" {
		return v
	}
	return cfg.PreferredProvider
}

func openOutputWriter(path string) (io.Writer, func(), error) {
	if strings.TrimSpace(path) == "" {
		return os.Stdout, func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create output directory: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("create output file: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}

func writeBinaryFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func resolveConfigPath(path string) string {
	if strings.TrimSpace(path) == "" {
		if p, err := config.DefaultPath(); err == nil {
			return p
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
