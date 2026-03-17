package config

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	LLM                LLMConfig
	Telegram           TelegramConfig
	Agents             []AgentConfig
	DataDir            string
	Debug              bool
	Features           FeaturesConfig
	MaxToolRounds      int           // max LLM-tool loop iterations per message (default 20)
	CompressionRatio   float64       // compression threshold as ratio of maxTokens (default 0.8)
	SessionIdleTimeout time.Duration // auto-archive sessions after this idle duration (default 30m)
}

type LLMConfig struct {
	Provider        string
	BaseURL         string
	APIKey          string
	Model           string
	MaxTokens       int
	ReasoningEffort string
	// to avoid http timeouts for long-running tool calls
	AlwaysStream bool
	ExtraTools   []string
}

type TelegramConfig struct {
	Token        string
	AllowedUsers []int64
	VerboseChat  int64 // optional: send tool calls to a separate chat
}

type AgentConfig struct {
	Id      string
	Default bool
	Model   string
	Groups  []string
}

type SchedulerConfig struct {
	Enabled bool
}

type ToolsConfig struct {
	WebSearch bool
	WebFetch  bool
	Exec      bool
}

type FeaturesConfig struct {
	Scheduler SchedulerConfig
	Tools     ToolsConfig
}

func Load(path string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("dataDir", "./data")
	v.SetDefault("features.scheduler.enabled", true)
	v.SetDefault("features.tools", ToolsConfig{
		WebSearch: true,
		WebFetch:  true,
		Exec:      true,
	})
	v.SetDefault("llm.maxTokens", 128000)
	v.SetDefault("maxToolRounds", 20)
	v.SetDefault("compressionRatio", 0.8)
	v.SetDefault("sessionIdleTimeout", "1h")
	v.SetDefault("agents", []AgentConfig{
		{Id: "main", Default: true},
	})

	// Config file
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yml")
		v.AddConfigPath(".")
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	slog.Info("config file used", "path", v.ConfigFileUsed())

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.LLM.APIKey == "" {
		return fmt.Errorf("llm.apiKey is required")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("llm.model is required")
	}
	if c.Telegram.Token == "" {
		return fmt.Errorf("telegram.token is required")
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("at least one agent must be configured")
	}

	hasDefault := false
	for _, agent := range c.Agents {
		if agent.Default {
			if hasDefault {
				return fmt.Errorf("only one agent can be the default")
			}
			hasDefault = true
		}
	}

	if !hasDefault {
		slog.Warn("No default agent specified, setting the first agent as default")
		c.Agents[0].Default = true
	}

	return nil
}

// ResolvePath resolves a relative path against the data directory.
func (c *Config) ResolvePath(rel string) string {
	return filepath.Join(c.DataDir, rel)
}
