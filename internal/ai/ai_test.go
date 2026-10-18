package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\": 1}\n```":                `{"a": 1}`,
		"Sure! {\"s\": \"a } brace\", \"n\": {}}": `{"s": "a } brace", "n": {}}`,
		"no json here":                            "",
	}
	for in, want := range cases {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenAICompatible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, `{"error":"bad key"}`, http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/chat/completions":
			var body struct {
				Model    string              `json:"model"`
				Messages []map[string]string `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Model != "qwen/qwen3.8-max" || len(body.Messages) != 2 || body.Messages[0]["role"] != "system" {
				http.Error(w, "unexpected request", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Here you go: {\"ok\": true}"}}]}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen/qwen3.8-max"},{"id":"openai/gpt-5.5"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(Provider{Kind: "openai", BaseURL: srv.URL + "/v1/", Model: "qwen/qwen3.8-max", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	var out struct{ OK bool }
	if err := CompleteJSON(context.Background(), c, "sys", "user", &out); err != nil || !out.OK {
		t.Fatalf("CompleteJSON: %v %+v", err, out)
	}
	ids, err := c.Models(context.Background())
	if err != nil || len(ids) != 2 {
		t.Fatalf("Models: %v %v", ids, err)
	}

	bad, _ := New(Provider{Kind: "openai", BaseURL: srv.URL + "/v1", Model: "m", APIKey: "wrong"})
	if _, err := bad.Complete(context.Background(), "s", "u"); err == nil {
		t.Fatal("expected an auth error")
	}
}
