// Package llm is the BYOM provider abstraction. The provider is selected by
// env/config; the rest of the system is identical regardless of model.
// LLM output is ADVISORY ONLY — nothing here reaches policy inputs.
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Result struct {
	Text   string
	Tokens int
}

type Provider interface {
	Name() string
	Complete(system, user string) (Result, error)
}

// FromEnv: AI_PROVIDER = anthropic | openai | stub (default stub for determinism).
func FromEnv() Provider {
	switch os.Getenv("AI_PROVIDER") {
	case "anthropic":
		return &Anthropic{Model: envOr("AI_MODEL", "claude-sonnet-5"), Key: os.Getenv("ANTHROPIC_API_KEY")}
	case "openai":
		return &OpenAI{Model: envOr("AI_MODEL", "gpt-4o"), Key: os.Getenv("OPENAI_API_KEY"),
			Base: envOr("OPENAI_BASE_URL", "https://api.openai.com/v1")}
	default:
		return Stub{}
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Stub: deterministic canned responses for tests/CI (ARCHITECTURE self-critique 3).
type Stub struct{}

func (Stub) Name() string { return "stub" }
func (Stub) Complete(system, user string) (Result, error) {
	switch {
	case strings.Contains(system, "risk summary"):
		return Result{Text: "Routine dependency update. Changelog indicates bug fixes only; no API changes detected. [stub model]", Tokens: 40}, nil
	case strings.Contains(system, "widen"):
		return Result{Text: "NONE", Tokens: 5}, nil
	case strings.Contains(system, "classify"):
		return Result{Text: `{"type":"bug","area_labels":[],"missing_info":[],"rationale":"stub classification"}`, Tokens: 30}, nil
	}
	return Result{Text: "[stub]", Tokens: 2}, nil
}

// Anthropic provider (Messages API).
type Anthropic struct{ Model, Key string }

func (a *Anthropic) Name() string { return "anthropic/" + a.Model }
func (a *Anthropic) Complete(system, user string) (Result, error) {
	if a.Key == "" {
		return Result{}, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	body, _ := json.Marshal(map[string]any{
		"model": a.Model, "max_tokens": 1024, "system": system,
		"messages": []map[string]string{{"role": "user", "content": user}},
	})
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	req.Header.Set("x-api-key", a.Key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := do(req, &out); err != nil {
		return Result{}, err
	}
	var sb strings.Builder
	for _, cnt := range out.Content {
		sb.WriteString(cnt.Text)
	}
	return Result{Text: sb.String(), Tokens: out.Usage.InputTokens + out.Usage.OutputTokens}, nil
}

// OpenAI-compatible provider (also covers vLLM/Ollama/OpenRouter via base URL).
type OpenAI struct{ Model, Key, Base string }

func (o *OpenAI) Name() string { return "openai/" + o.Model }
func (o *OpenAI) Complete(system, user string) (Result, error) {
	body, _ := json.Marshal(map[string]any{
		"model": o.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system}, {"role": "user", "content": user},
		},
	})
	req, _ := http.NewRequest("POST", o.Base+"/chat/completions", bytes.NewReader(body))
	if o.Key != "" {
		req.Header.Set("Authorization", "Bearer "+o.Key)
	}
	req.Header.Set("content-type", "application/json")
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := do(req, &out); err != nil {
		return Result{}, err
	}
	if len(out.Choices) == 0 {
		return Result{}, fmt.Errorf("no choices")
	}
	return Result{Text: out.Choices[0].Message.Content, Tokens: out.Usage.TotalTokens}, nil
}

func do(req *http.Request, v any) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e bytes.Buffer
		e.ReadFrom(resp.Body)
		return fmt.Errorf("llm http %d: %s", resp.StatusCode, truncate(e.String(), 200))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
