// Package ai talks to whichever LLM the dashboard is configured with: the
// Anthropic Messages API, or any OpenAI-compatible /chat/completions API
// (OpenAI, Vikey, OpenRouter, Gemini's OpenAI endpoint, DeepSeek…).
//
// Callers only need text in and JSON out, so the surface is two methods.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type Provider struct {
	Kind    string // "anthropic" | "openai"
	BaseURL string
	Model   string
	APIKey  string
}

type Client interface {
	Complete(ctx context.Context, system, user string) (string, error)
	Models(ctx context.Context) ([]string, error)
}

func New(p Provider) (Client, error) {
	if strings.TrimSpace(p.APIKey) == "" {
		return nil, errors.New("the AI provider has no API key")
	}
	switch p.Kind {
	case "anthropic":
		opts := []option.RequestOption{option.WithAPIKey(p.APIKey), option.WithRequestTimeout(3 * time.Minute)}
		if p.BaseURL != "" {
			opts = append(opts, option.WithBaseURL(p.BaseURL))
		}
		return &claude{client: anthropic.NewClient(opts...), model: p.Model}, nil
	case "openai":
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return &chat{base: base, key: p.APIKey, model: p.Model, http: &http.Client{Timeout: 3 * time.Minute}}, nil
	}
	return nil, fmt.Errorf("unknown AI provider kind %q", p.Kind)
}

// CompleteJSON asks for a JSON object and decodes it into out. Models wrap
// JSON in prose or code fences often enough that the first {...} block is
// extracted rather than trusting the whole reply.
func CompleteJSON(ctx context.Context, c Client, system, user string, out any) error {
	system += "\n\nReply with a single JSON object and nothing else."
	text, err := c.Complete(ctx, system, user)
	if err != nil {
		return err
	}
	raw := extractJSON(text)
	if raw == "" {
		return fmt.Errorf("the model did not return JSON: %.200s", text)
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("the model returned malformed JSON: %w", err)
	}
	return nil
}

func extractJSON(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth, inStr, esc := 0, false, false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case c == '\\' && inStr:
			esc = true
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// ── Anthropic ────────────────────────────────────────────────────────────────

type claude struct {
	client anthropic.Client
	model  string
}

func (c *claude) Complete(ctx context.Context, system, user string) (string, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     c.model,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(user))},
	}
	// Models with refusal classifiers get re-served by a fallback model
	// instead of stopping, so one false positive doesn't stall a run.
	if strings.HasPrefix(c.model, "claude-opus-5") || strings.HasPrefix(c.model, "claude-fable-5") {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}
	msg, err := c.client.Beta.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return "", fmt.Errorf("anthropic: HTTP %d: %s", apiErr.StatusCode, firstLine(apiErr.Error()))
		}
		return "", fmt.Errorf("anthropic: %w", err)
	}
	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		return "", errors.New("anthropic: the model declined this request")
	}
	var b strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("anthropic: empty reply (stop reason %s)", msg.StopReason)
	}
	return b.String(), nil
}

func (c *claude) Models(ctx context.Context) ([]string, error) {
	var ids []string
	iter := c.client.Models.ListAutoPaging(ctx, anthropic.ModelListParams{})
	for iter.Next() {
		ids = append(ids, iter.Current().ID)
	}
	return ids, iter.Err()
}

// ── OpenAI-compatible ────────────────────────────────────────────────────────

type chat struct {
	base, key, model string
	http             *http.Client
}

func (c *chat) Complete(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := c.do(ctx, http.MethodPost, "/chat/completions", body, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("the model returned an empty reply")
	}
	return out.Choices[0].Message.Content, nil
}

func (c *chat) Models(ctx context.Context) ([]string, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/models", nil, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func (c *chat) do(ctx context.Context, method, path string, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", c.base, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s: HTTP %d: %.300s", c.base, res.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: unexpected response: %.200s", c.base, data)
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}
