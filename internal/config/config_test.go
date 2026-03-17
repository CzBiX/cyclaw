package config

import (
	"testing"
)

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			APIKey: "test-key",
			Model:  "gpt-4",
		},
		Telegram: TelegramConfig{
			Token: "bot-token",
		},
		Agents: []AgentConfig{
			{Id: "main", Default: true},
		},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() error: %v", err)
	}
}

func TestValidate_MissingAPIKey(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			Model: "gpt-4",
		},
		Telegram: TelegramConfig{Token: "bot-token"},
		Agents:   []AgentConfig{{Id: "main", Default: true}},
	}

	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestValidate_MissingModel(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			APIKey: "test-key",
		},
		Telegram: TelegramConfig{Token: "bot-token"},
		Agents:   []AgentConfig{{Id: "main", Default: true}},
	}

	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestValidate_MissingTelegramToken(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			APIKey: "test-key",
			Model:  "gpt-4",
		},
		Telegram: TelegramConfig{},
		Agents:   []AgentConfig{{Id: "main", Default: true}},
	}

	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for missing telegram token")
	}
}

func TestValidate_NoAgents(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			APIKey: "test-key",
			Model:  "gpt-4",
		},
		Telegram: TelegramConfig{Token: "bot-token"},
		Agents:   nil,
	}

	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for no agents")
	}
}

func TestValidate_MultipleDefaults(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			APIKey: "test-key",
			Model:  "gpt-4",
		},
		Telegram: TelegramConfig{Token: "bot-token"},
		Agents: []AgentConfig{
			{Id: "agent1", Default: true},
			{Id: "agent2", Default: true},
		},
	}

	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for multiple default agents")
	}
}

func TestValidate_NoDefault_SetsFirst(t *testing.T) {
	cfg := &Config{
		LLM: LLMConfig{
			APIKey: "test-key",
			Model:  "gpt-4",
		},
		Telegram: TelegramConfig{Token: "bot-token"},
		Agents: []AgentConfig{
			{Id: "agent1"},
			{Id: "agent2"},
		},
	}

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() error: %v", err)
	}

	if !cfg.Agents[0].Default {
		t.Error("first agent should be set as default")
	}
}

func TestResolvePath(t *testing.T) {
	cfg := &Config{DataDir: "/data"}
	got := cfg.ResolvePath("memory/MEMORY.md")
	if got != "/data/memory/MEMORY.md" {
		t.Errorf("ResolvePath() = %q, want %q", got, "/data/memory/MEMORY.md")
	}
}
