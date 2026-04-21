package backend

import (
	"os"
	"path/filepath"
	"strings"

	config "codeagent-wrapper/internal/config"
)

type KimiBackend struct{}

func (KimiBackend) Name() string    { return "kimi" }
func (KimiBackend) Command() string { return "kimi" }
func (KimiBackend) Env(baseURL, apiKey string) map[string]string {
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" && apiKey == "" {
		return nil
	}
	env := make(map[string]string, 2)
	if baseURL != "" {
		env["KIMI_BASE_URL"] = baseURL
	}
	if apiKey != "" {
		env["KIMI_API_KEY"] = apiKey
	}
	return env
}
func (KimiBackend) BuildArgs(cfg *config.Config, targetArg string) []string {
	return buildKimiArgs(cfg, targetArg)
}

// LoadKimiEnv loads environment variables from ~/.kimi/.env if it exists.
// Supports KIMI_API_KEY, KIMI_BASE_URL, KIMI_MODEL_NAME.
func LoadKimiEnv() map[string]string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}

	envDir := filepath.Clean(filepath.Join(home, ".kimi"))
	envPath := filepath.Clean(filepath.Join(envDir, ".env"))
	rel, err := filepath.Rel(envDir, envPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil
	}

	data, err := os.ReadFile(envPath) // #nosec G304 -- path is fixed under user home and validated within envDir
	if err != nil {
		return nil
	}

	env := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key != "" && value != "" {
			env[key] = value
		}
	}

	if len(env) == 0 {
		return nil
	}
	return env
}

func buildKimiArgs(cfg *config.Config, targetArg string) []string {
	if cfg == nil {
		return nil
	}
	// --print: non-interactive mode (implies --yolo)
	// --output-format stream-json: JSON line output
	// --final-message-only: emit only the final assistant message
	args := []string{"--print", "--output-format", "stream-json", "--final-message-only"}

	if model := strings.TrimSpace(cfg.Model); model != "" {
		args = append(args, "-m", model)
	}

	if cfg.Mode == "resume" && cfg.SessionID != "" {
		args = append(args, "-S", cfg.SessionID)
	}

	// Working directory is set via cmd.SetDir in the executor (like Claude/Gemini).
	// kimi defaults to the process CWD when --work-dir is omitted.

	if targetArg != "-" {
		args = append(args, "-p", targetArg)
	}
	// When targetArg == "-", task is written to stdin.
	// kimi reads stdin automatically when it is not a TTY and --input-format is "text" (the default).

	return args
}
