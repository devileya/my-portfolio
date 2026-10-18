// Package telegram is a minimal Bot API client: send messages with inline
// buttons, and long-poll for button presses and replies from one chat.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Bot struct {
	token  string
	chatID int64
	http   *http.Client
}

// New returns nil when the bot isn't configured; every method on a nil Bot
// is a no-op, so callers never have to check.
func New(token string, chatID int64) *Bot {
	if token == "" || chatID == 0 {
		return nil
	}
	return &Bot{token: token, chatID: chatID, http: &http.Client{Timeout: 70 * time.Second}}
}

type Button struct {
	Text string `json:"text"`
	Data string `json:"callback_data,omitempty"`
	URL  string `json:"url,omitempty"`
}

type Message struct {
	MessageID int64 `json:"message_id"`
	Chat      struct {
		ID int64 `json:"id"`
	} `json:"chat"`
	Text           string   `json:"text"`
	ReplyToMessage *Message `json:"reply_to_message"`
}

type Callback struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
	From    struct {
		ID int64 `json:"id"`
	} `json:"from"`
}

type Update struct {
	UpdateID      int64     `json:"update_id"`
	Message       *Message  `json:"message"`
	CallbackQuery *Callback `json:"callback_query"`
}

// Chat is someone who messaged the bot, as offered when setting it up.
type Chat struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Verify checks a token and returns the bot's @username.
func Verify(ctx context.Context, token string) (string, error) {
	b := &Bot{token: token, http: &http.Client{Timeout: 15 * time.Second}}
	var me struct {
		Username string `json:"username"`
	}
	if err := b.call(ctx, "getMe", map[string]any{}, &me); err != nil {
		return "", err
	}
	return me.Username, nil
}

// RecentChats lists the chats that recently messaged the bot, newest first.
// It fails while another process is long-polling the same bot.
func RecentChats(ctx context.Context, token string) ([]Chat, error) {
	b := &Bot{token: token, http: &http.Client{Timeout: 15 * time.Second}}
	var updates []struct {
		Message *struct {
			Chat struct {
				ID        int64  `json:"id"`
				Title     string `json:"title"`
				Username  string `json:"username"`
				FirstName string `json:"first_name"`
				LastName  string `json:"last_name"`
			} `json:"chat"`
		} `json:"message"`
	}
	if err := b.call(ctx, "getUpdates", map[string]any{"limit": 100, "allowed_updates": []string{"message"}}, &updates); err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var out []Chat
	for i := len(updates) - 1; i >= 0; i-- {
		m := updates[i].Message
		if m == nil || seen[m.Chat.ID] {
			continue
		}
		seen[m.Chat.ID] = true
		name := strings.TrimSpace(m.Chat.Title + " " + m.Chat.FirstName + " " + m.Chat.LastName)
		if m.Chat.Username != "" {
			name = strings.TrimSpace(name + " (@" + m.Chat.Username + ")")
		}
		out = append(out, Chat{ID: m.Chat.ID, Name: name})
	}
	return out, nil
}

// Send posts an HTML message to the configured chat. rows are rows of inline
// buttons. It returns the new message's id.
func (b *Bot) Send(ctx context.Context, html string, rows ...[]Button) (int64, error) {
	if b == nil {
		return 0, nil
	}
	body := map[string]any{
		"chat_id":                  b.chatID,
		"text":                     truncate(html, 4000),
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	if len(rows) > 0 {
		body["reply_markup"] = map[string]any{"inline_keyboard": rows}
	}
	var msg Message
	if err := b.call(ctx, "sendMessage", body, &msg); err != nil {
		return 0, err
	}
	return msg.MessageID, nil
}

// Notify is Send for fire-and-forget messages: failures are logged.
func (b *Bot) Notify(ctx context.Context, html string, rows ...[]Button) {
	if b == nil {
		return
	}
	if _, err := b.Send(ctx, html, rows...); err != nil {
		slog.Warn("telegram send", "err", err)
	}
}

// Edit replaces a message's text and drops its buttons.
func (b *Bot) Edit(ctx context.Context, messageID int64, html string) error {
	if b == nil {
		return nil
	}
	return b.call(ctx, "editMessageText", map[string]any{
		"chat_id": b.chatID, "message_id": messageID, "text": truncate(html, 4000),
		"parse_mode": "HTML", "disable_web_page_preview": true,
	}, nil)
}

func (b *Bot) Answer(ctx context.Context, callbackID, text string) {
	if b == nil {
		return
	}
	_ = b.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID, "text": text}, nil)
}

// Poll long-polls getUpdates until ctx ends, passing only updates from the
// configured chat to handle.
func (b *Bot) Poll(ctx context.Context, handle func(context.Context, Update)) {
	if b == nil {
		return
	}
	var offset int64
	backoff := time.Second
	for ctx.Err() == nil {
		var updates []Update
		err := b.call(ctx, "getUpdates", map[string]any{
			"offset": offset, "timeout": 50, "allowed_updates": []string{"message", "callback_query"},
		}, &updates)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("telegram poll", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message != nil && u.Message.Chat.ID != b.chatID {
				continue
			}
			if u.CallbackQuery != nil && (u.CallbackQuery.Message == nil || u.CallbackQuery.Message.Chat.ID != b.chatID) {
				continue
			}
			handle(ctx, u)
		}
	}
}

func (b *Bot) call(ctx context.Context, method string, body any, out any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+b.token+"/"+method, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := b.http.Do(req)
	if err != nil {
		// The URL carries the token; never let it reach a log line.
		var uerr interface{ Unwrap() error }
		if errors.As(err, &uerr) {
			return fmt.Errorf("telegram %s: %w", method, uerr.Unwrap())
		}
		return fmt.Errorf("telegram %s: request failed", method)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	var env struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("telegram %s: HTTP %d", method, res.StatusCode)
	}
	if !env.OK {
		return fmt.Errorf("telegram %s: %s", method, env.Description)
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// Escape makes text safe inside an HTML-mode message.
func Escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
