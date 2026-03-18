package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"cyclaw/internal/agent"
	"cyclaw/internal/channel"
	"cyclaw/internal/config"
	"cyclaw/internal/llm"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Telegram implements the channel interface using github.com/go-telegram/bot.

const maxMessageLength = 4096 // Telegram's message character limit
const channelID = "telegram"

type Telegram struct {
	b            *bot.Bot
	token        string
	router       *agent.Router
	allowedUsers map[int64]bool
	verboseChat  int64 // if non-zero, send reasoning & tool calls to this chat
}

// New creates a new Telegram channel.
func New(cfg config.TelegramConfig, router *agent.Router) (*Telegram, error) {
	allowed := make(map[int64]bool)
	for _, uid := range cfg.AllowedUsers {
		allowed[uid] = true
	}

	t := &Telegram{
		token:        cfg.Token,
		router:       router,
		allowedUsers: allowed,
		verboseChat:  cfg.VerboseChat,
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(t.handleUpdate),
		bot.WithSkipGetMe(),
	}

	b, err := bot.New(cfg.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	t.b = b
	t.registerHandlers()

	return t, nil
}

func (t *Telegram) ID() string {
	return channelID
}

// Start begins polling for updates. Blocks until Stop is called.
func (t *Telegram) Start(ctx context.Context) {
	slog.Info("telegram bot starting")

	// Register bot commands so they appear in the Telegram command menu.
	if _, err := t.b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "new", Description: "Start a new conversation"},
			{Command: "clear", Description: "Clear current session history"},
		},
	}); err != nil {
		slog.Error("failed to set bot commands", "error", err)
	}

	t.b.Start(ctx)
}

// Send sends a message to a specific chat.
func (t *Telegram) Send(ctx context.Context, msg *channel.OutgoingMessage) error {
	chatID, err := strconv.ParseInt(msg.GroupID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID %q: %w", msg.GroupID, err)
	}

	params := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   msg.Text,
	}

	switch msg.ParseMode {
	case "markdown":
		params.ParseMode = models.ParseModeMarkdown
		params.Text = escapeMarkdownV2(msg.Text)
	case "html":
		params.ParseMode = models.ParseModeHTML
	}

	if msg.DisablePreview {
		params.LinkPreviewOptions = &models.LinkPreviewOptions{
			IsDisabled: &msg.DisablePreview,
		}
	}

	_, err = t.b.SendMessage(ctx, params)
	return err
}

const telegramSystemPrompt = `# Channel: Telegram

## Formatting Rules
- Use Telegram-flavored Markdown for formatting, which only supports: *bold*, _italic_, __underline__, ||spoiler||, [inline URL](http://www.example.com/), ` + "`code`(inline), and ```code blocks```" + `, along with blockquotes and expandable block quotations.
- Do NOT use heading syntax (# or ##); use *bold* text for section titles instead.
- Do NOT use Markdown features that Telegram does not support, such as numbered lists, emojis, tables, or HTML tags.
- Messages exceeding 4096 characters will be split automatically; structure your output so splits at line boundaries remain coherent.
- Use line breaks generously for readability on mobile screens.
- When listing items, prefer simple dashes (-) over numbered lists unless order matters.`

// Prompt returns Telegram-specific system prompt instructions.
func (t *Telegram) Prompt() string {
	return telegramSystemPrompt
}

// Bot returns the underlying bot.Bot instance.
func (t *Telegram) Bot() *bot.Bot {
	return t.b
}

// sendMarkdown sends text as Markdown to the given chat, splitting at line
// boundaries if it exceeds Telegram's message length limit.
func (t *Telegram) sendMarkdown(ctx context.Context, chatID int64, text string) {
	for _, chunk := range splitMessage(text) {
		if _, err := t.b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      escapeMarkdownV2(chunk),
			ParseMode: models.ParseModeMarkdown,
		}); err != nil {
			slog.Warn("sendMessage failed", "error", err, "chat_id", chatID)
		}
	}
}

// sendText sends a plain text message to the given chat.
func (t *Telegram) sendText(ctx context.Context, chatID int64, text string) {
	if _, err := t.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}); err != nil {
		slog.Warn("sendMessage failed", "error", err, "chat_id", chatID)
	}
}

// registerHandlers sets up bot event handlers.
func (t *Telegram) registerHandlers() {
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		t.sendText(ctx, update.Message.Chat.ID, "Hello!")
	})
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/clear", bot.MatchTypeExact, t.handleClear)
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/new", bot.MatchTypeExact, t.handleNew)
}

// handleClear clears the current session history for the chat.
func (t *Telegram) handleClear(ctx context.Context, _ *bot.Bot, update *models.Update) {
	numChatID := update.Message.Chat.ID
	chatID := fmt.Sprintf("%d", numChatID)
	ag := t.router.Resolve(chatID)
	if ag == nil {
		t.sendText(ctx, numChatID, "No active session found.")
		return
	}

	ag.ClearSession(chatID)
	t.sendText(ctx, numChatID, "Session cleared.")
}

// handleNew archives the current session and starts a fresh one.
func (t *Telegram) handleNew(ctx context.Context, _ *bot.Bot, update *models.Update) {
	numChatID := update.Message.Chat.ID
	chatID := fmt.Sprintf("%d", numChatID)
	ag := t.router.Resolve(chatID)
	if ag == nil {
		t.sendText(ctx, numChatID, "No active session found.")
		return
	}

	if !ag.NewSession(ctx, chatID) {
		t.sendText(ctx, numChatID, "No conversation to archive. Starting a new session.")
		return
	}

	t.sendText(ctx, numChatID, "New session started. Previous conversation is being archived.")
}

// handleUpdate is the default handler for all updates.
func (t *Telegram) handleUpdate(ctx context.Context, _ *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	t.handleMessage(ctx, update)
}

// handleMessage processes an incoming text message.
func (t *Telegram) handleMessage(ctx context.Context, update *models.Update) {
	msg := update.Message
	if msg.From == nil {
		return
	}

	sender := msg.From

	// Check allowed users (if configured)
	if len(t.allowedUsers) > 0 && !t.allowedUsers[sender.ID] {
		slog.Warn("unauthorized user", "user_id", sender.ID, "username", sender.Username)
		return
	}

	chatID := msg.Chat.ID
	slog.Info("received message",
		"chat_id", chatID,
		"user_id", sender.ID,
		"username", sender.Username,
		"text_len", len(msg.Text),
	)

	// Send typing action continuously until processing is done.
	stopTyping := t.keepTyping(ctx, chatID)
	defer stopTyping()

	// Streaming state
	draftID := strconv.FormatInt(chatID, 16)
	var accumulated strings.Builder
	lastSent := time.UnixMilli(0)
	const draftInterval = 3 * time.Second

	streamCb := func(delta llm.StreamDelta) error {
		// Handle function calls notification from the agent loop.
		if len(delta.FunctionCalls) > 0 {
			// Flush any pending streamed text first.
			if accumulated.Len() > 0 {
				t.sendMarkdown(ctx, chatID, accumulated.String())
				accumulated.Reset()
			}

			// Only send function call details when verboseChat is configured.
			if t.verboseChat != 0 {
				var sb strings.Builder
				sb.WriteString("🔧 *Function Calls*\n")
				for _, fc := range delta.FunctionCalls {
					fmt.Fprintf(&sb, "\n`%s`", fc.Name)
					if fc.Arguments != "" && fc.Arguments != "{}" {
						fmt.Fprintf(&sb, "\n```json\n%s\n```", fc.Arguments)
					}
				}
				t.sendMarkdown(ctx, t.verboseChat, sb.String())
			}
			return nil
		}

		accumulated.WriteString(delta.Content)

		// Throttle: only send draft if enough time has passed.
		if time.Since(lastSent) < draftInterval && !delta.Done {
			return nil
		}
		lastSent = time.Now()

		if accumulated.Len() == 0 {
			return nil
		}
		draftText := accumulated.String()

		if delta.Done {
			t.sendMarkdown(ctx, chatID, draftText)
			accumulated.Reset()
			return nil
		}

		// Split long text; send all complete chunks as final messages.
		chunks := splitMessage(draftText)
		if len(chunks) > 1 {
			for _, chunk := range chunks[:len(chunks)-1] {
				t.sendMarkdown(ctx, chatID, chunk)
			}

			draftText = chunks[len(chunks)-1]

			accumulated.Reset()
			accumulated.WriteString(draftText)
		}

		if _, err := t.b.SendMessageDraft(ctx, &bot.SendMessageDraftParams{
			ChatID:    chatID,
			DraftID:   draftID,
			Text:      escapeMarkdownV2(draftText),
			ParseMode: models.ParseModeMarkdown,
		}); err != nil {
			slog.Warn("sendMessageDraft failed", "error", err, "chat_id", chatID)
		}

		return nil
	}

	userText := buildUserText(msg)

	incoming := &channel.IncomingMessage{
		ChannelID:     channelID,
		ChatID:        fmt.Sprintf("%d", chatID),
		UserID:        fmt.Sprintf("%d", sender.ID),
		UserName:      sender.Username,
		Text:          userText,
		ChannelPrompt: t.Prompt(),
	}
	_, err := t.router.Route(ctx, incoming, streamCb)
	if err != nil {
		slog.Error("agent error", "error", err, "chat_id", chatID)
		t.sendText(ctx, chatID, "Sorry, something went wrong. Please try again.")
		return
	}

	slog.Info("reply sent", "chat_id", chatID)
}

// buildUserText constructs the user text from the message, prepending any
// quoted or replied-to content so the LLM can see the full context.
func buildUserText(msg *models.Message) string {
	var quoted string
	switch {
	case msg.Quote != nil && msg.Quote.Text != "":
		quoted = msg.Quote.Text
	case msg.ReplyToMessage != nil && msg.ReplyToMessage.Text != "":
		quoted = msg.ReplyToMessage.Text
	}
	if quoted == "" {
		return msg.Text
	}
	return fmt.Sprintf("<quoted>\n%s\n</quoted>\n\n%s", quoted, msg.Text)
}

// keepTyping sends the "typing" chat action immediately and repeats every 4
// seconds until the returned cancel function is called.
func (t *Telegram) keepTyping(ctx context.Context, chatID int64) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		t.b.SendChatAction(ctx, &bot.SendChatActionParams{
			ChatID: chatID,
			Action: models.ChatActionTyping,
		})
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.b.SendChatAction(ctx, &bot.SendChatActionParams{
					ChatID: chatID,
					Action: models.ChatActionTyping,
				})
			}
		}
	}()
	return cancel
}

// splitMessage splits text into chunks that fit within Telegram's message
// length limit, breaking at line boundaries.
func splitMessage(text string) []string {
	const maxLen = maxMessageLength
	if len(text) <= maxLen {
		return []string{text}
	}

	lines := strings.Split(text, "\n")
	var chunks []string
	var chunk strings.Builder

	for _, line := range lines {
		if chunk.Len()+len(line)+1 > maxLen {
			if chunk.Len() > 0 {
				chunks = append(chunks, chunk.String())
				chunk.Reset()
			}
		}
		if chunk.Len() > 0 {
			chunk.WriteString("\n")
		}
		chunk.WriteString(line)
	}

	if chunk.Len() > 0 {
		chunks = append(chunks, chunk.String())
	}

	return chunks
}
