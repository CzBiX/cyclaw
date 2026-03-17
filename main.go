package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cyclaw/embed"
	"cyclaw/internal/agent"
	"cyclaw/internal/channel"
	"cyclaw/internal/channel/telegram"
	"cyclaw/internal/config"
	"cyclaw/internal/llm"
	"cyclaw/internal/memory"
	"cyclaw/internal/prompt"
	"cyclaw/internal/scheduler"
	"cyclaw/internal/session"
	"cyclaw/internal/skill"
	"cyclaw/internal/tool"
)

func loadConfig() *config.Config {
	configArg := ""
	if len(os.Args) > 1 {
		configArg = os.Args[1]
	}

	cfg, err := config.Load(configArg)
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	return cfg
}

func main() {
	logHandler := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logHandler)

	cfg := loadConfig()

	if cfg.Debug {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	if err := run(cfg); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func getCancelableContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.InfoContext(ctx, "received signal, shutting down", "signal", sig)
		cancel()
	}()

	return ctx, cancel
}

func run(cfg *config.Config) error {
	slog.Debug("config loaded", "dataDir", cfg.DataDir)

	ctx, _ := getCancelableContext()

	if err := extractEmbeddedFiles(cfg.DataDir); err != nil {
		return err
	}

	provider := newLLMProvider(cfg)
	builder := newPromptBuilder(cfg)

	skills, err := loadSkills(cfg)
	if err != nil {
		return err
	}

	router := agent.NewRouter()
	tools := newTools(cfg)
	diary := memory.NewManager(cfg.ResolvePath("memory"))

	// Register diary tool
	tools.Register(tool.NewReadDiaryTool(diary))

	tg, err := newTelegramChannel(cfg, router)
	if err != nil {
		return err
	}

	// Register send_message tool with Telegram as sender.
	// The router is passed as a SessionResolver so that proactive messages
	// are recorded in the target chat's normal session history.
	tools.Register(tool.NewSendMessageTool(tg, router))

	sched, err := newScheduler(cfg, router, tools, diary)
	if err != nil {
		return err
	}

	// Create and register agents
	if err := registerAgents(ctx, cfg, router, provider, tools, builder, skills, diary); err != nil {
		return err
	}

	// Register sub_task tool. This is done after agent registration so that
	// the router can resolve target agents. Since the registry is shared by
	// pointer, the tool is available to all agents when they read LLMDefs().
	tools.Register(tool.NewSubTaskTool(&subTaskAdapter{router: router}, tools))

	// Start scheduler
	if err := sched.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer sched.Stop()

	slog.Info("cyclaw starting")
	tg.Start(ctx)

	return nil
}

func extractEmbeddedFiles(dataDir string) error {
	// Extract embedded files to data directory
	if err := embed.Extract(dataDir); err != nil {
		return fmt.Errorf("extract embedded files: %w", err)
	}
	slog.Debug("embedded files extracted", "dir", dataDir)
	return nil
}

func newLLMProvider(cfg *config.Config) *llm.OpenAI {
	provider := llm.NewOpenAI(&cfg.LLM)
	slog.Debug("LLM provider initialized",
		"provider", cfg.LLM.Provider,
		"model", cfg.LLM.Model,
		"baseUrl", cfg.LLM.BaseURL,
	)
	return provider
}

func newPromptBuilder(cfg *config.Config) *prompt.Builder {
	resolver := prompt.NewResolver(cfg.DataDir)
	return prompt.NewBuilder(resolver)
}

func loadSkills(cfg *config.Config) ([]*skill.Skill, error) {
	skillsDir := cfg.ResolvePath("skills")
	loader := skill.NewLoader(skillsDir)
	skills, err := loader.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load skills: %w", err)
	}
	slog.Debug("skills loaded", "count", len(skills))
	return skills, nil
}

func newTools(cfg *config.Config) *tool.Registry {
	tools := tool.NewRegistry()

	tools.Register(tool.NewReadFileTool(cfg.DataDir))
	tools.Register(tool.NewWriteFileTool(cfg.DataDir))
	tools.Register(tool.NewGlobTool(cfg.DataDir))

	if cfg.Features.Tools.WebFetch {
		tools.Register(tool.NewWebFetchTool())
	}
	if cfg.Features.Tools.WebSearch {
		tools.Register(tool.NewWebSearchTool())
	}
	if cfg.Features.Tools.Exec {
		tools.Register(tool.NewExecTool(cfg.ResolvePath("workspace")))
	}

	return tools
}

func newTelegramChannel(cfg *config.Config, router *agent.Router) (*telegram.Telegram, error) {
	return telegram.New(cfg.Telegram, router)
}

func newScheduler(cfg *config.Config, router *agent.Router, tools *tool.Registry, mem *memory.Manager) (*scheduler.Scheduler, error) {
	persistFile := cfg.ResolvePath("scheduler.json")
	sched := scheduler.New(persistFile, func(ctx context.Context, task *scheduler.Task) error {
		// When a cron job fires, route it as an internal message to the agent
		msg := &channel.IncomingMessage{
			ChannelID:  "scheduler",
			ChatID:     fmt.Sprintf("%s-%s", task.ID, time.Now().Format("20060102150405")),
			Text:       fmt.Sprintf("[Scheduled Task: %s] %s", task.ID, task.Action),
			Background: true,
		}
		_, err := router.Route(ctx, msg, nil)
		return err
	})

	// Register cron tool
	// Note: The cron tool depends on the scheduler being created first.
	tools.Register(tool.NewCronTool(sched))

	// Register system daily self-reflection task
	registerSelfReflection(sched, mem, router)

	return sched, nil
}

// registerSelfReflection adds the system daily self-reflection cron job.
// It runs at midnight every day, reviewing recent diary entries and considering
// updates to long-term memory (MEMORY.md) and soul (SOUL.md).
func registerSelfReflection(sched *scheduler.Scheduler, mem *memory.Manager, router *agent.Router) {
	task := &scheduler.Task{
		ID:       prompt.SelfReflectionTaskID(),
		Schedule: prompt.SelfReflectionSchedule(),
	}

	if err := sched.AddSystem(task, func() {
		agent.RunSelfReflection(context.Background(), mem, router)
	}); err != nil {
		slog.Error("failed to register self-reflection task", "error", err)
	}
}

func registerAgents(ctx context.Context, cfg *config.Config, router *agent.Router, provider *llm.OpenAI, tools *tool.Registry, builder *prompt.Builder, skills []*skill.Skill, diary agent.DiaryAppender) error {
	for _, agentCfg := range cfg.Agents {
		agentProvider := provider
		// If agent has a custom model, use the same provider but the model will be
		// picked up from AgentConfig during the chat request
		sessionsDir := cfg.ResolvePath(fmt.Sprintf("sessions/%s", agentCfg.Id))
		store := session.NewFileStore(sessionsDir)
		a := agent.NewAgent(agentCfg, cfg, agentProvider, tools, builder, skills, diary, store)
		router.Register(a)
		a.StartAutoArchive(ctx)
		slog.Debug("agent registered", "id", agentCfg.Id, "default", agentCfg.Default)
	}
	return nil
}

// subTaskAdapter bridges agent.Router to the tool.SubTaskExecutor interface.
type subTaskAdapter struct {
	router *agent.Router
}

func (a *subTaskAdapter) ExecuteSubTask(ctx context.Context, agentName string, task string, instructions string, tools *tool.Registry) (string, error) {
	ag, ok := a.router.GetAgent(agentName)
	if !ok {
		return "", fmt.Errorf("agent not found: %s", agentName)
	}
	return ag.HandleSubTask(ctx, task, instructions, tools)
}
