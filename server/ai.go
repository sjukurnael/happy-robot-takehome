package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// Sentinel errors the breakdown handler maps to HTTP statuses. Raw
// Anthropic API errors are logged server-side but never echoed to the
// client (same policy as writeStoreError for SQL errors).
var (
	ErrAINotConfigured = errors.New("AI breakdown is not configured (ANTHROPIC_API_KEY is not set)")
	ErrAIUnavailable   = errors.New("AI service is temporarily unavailable — try again")
	ErrAIBadOutput     = errors.New("AI returned unusable suggestions — try again")
)

// AIClient wraps the Anthropic API for the task-breakdown feature. When
// ANTHROPIC_API_KEY is unset the client is disabled and the breakdown
// endpoint answers 503 — everything else in the app works without a key
// (CI and the e2e suite run keyless).
type AIClient struct {
	client  anthropic.Client
	enabled bool
}

func newAIClient() *AIClient {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		return &AIClient{}
	}
	return &AIClient{client: anthropic.NewClient(), enabled: true}
}

const breakdownSystemPrompt = `You help break a task into subtasks in a collaborative task manager.
Propose between 3 and 8 subtasks. Each subtask needs a short actionable title,
a one-to-two sentence description, a priority (low, medium, or high), and 0-3
short lowercase tags.

Express ordering with dependsOn: an array of zero-based indices into your own
subtasks array, naming which of the OTHER proposed subtasks must be completed
first. Only reference indices that exist in this batch, never a subtask's own
index, and keep the graph acyclic. Independent subtasks should have an empty
dependsOn so they can be worked in parallel.

Avoid duplicating tasks that already exist in the project (listed in the user
message). Titles should be concrete next actions, not restatements of the
parent task.`

// breakdownSchema constrains the model's output. Structured outputs
// require additionalProperties:false and full required lists; count and
// length limits are unsupported schema keywords, so those are enforced by
// the prompt above and validateSuggestions after.
var breakdownSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"subtasks": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]any{"type": "string", "description": "Short actionable title"},
					"description": map[string]any{"type": "string", "description": "1-2 sentence description"},
					"priority":    map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
					"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"dependsOn":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Zero-based indices of subtasks in this batch that must finish first"},
				},
				"required":             []string{"title", "description", "priority", "tags", "dependsOn"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"subtasks"},
	"additionalProperties": false,
}

// BreakdownTask asks Claude to decompose task into subtasks. Pure read —
// nothing is persisted; the caller returns the validated suggestions for
// the user to review. siblingTitles is the project's other task titles, so
// the model can avoid proposing duplicates.
func (a *AIClient) BreakdownTask(ctx context.Context, task *Task, siblingTitles []string) ([]BreakdownSuggestion, error) {
	if !a.enabled {
		return nil, ErrAINotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	var b strings.Builder
	fmt.Fprintf(&b, "Break down this task:\n\nTitle: %s\n", task.Title)
	if d := strings.TrimSpace(task.Configuration.Description); d != "" {
		fmt.Fprintf(&b, "Description: %s\n", d)
	}
	if task.Configuration.Priority != "" {
		fmt.Fprintf(&b, "Priority: %s\n", task.Configuration.Priority)
	}
	if len(task.Configuration.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(task.Configuration.Tags, ", "))
	}
	if len(siblingTitles) > 0 {
		fmt.Fprintf(&b, "\nTasks that already exist in this project (do not duplicate):\n- %s\n", strings.Join(siblingTitles, "\n- "))
	}

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_8,
		MaxTokens: 16000,
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		System:    []anthropic.TextBlockParam{{Text: breakdownSystemPrompt}},
		OutputConfig: anthropic.OutputConfigParam{
			Format: anthropic.JSONOutputFormatParam{Schema: breakdownSchema},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(b.String())),
		},
	})
	if err != nil {
		var apierr *anthropic.Error
		if errors.As(err, &apierr) {
			log.Printf("anthropic API error (status %d): %v", apierr.StatusCode, err)
			if apierr.StatusCode == 401 || apierr.StatusCode == 403 {
				return nil, fmt.Errorf("%w: invalid API key", ErrAINotConfigured)
			}
			return nil, ErrAIUnavailable
		}
		log.Printf("anthropic request failed: %v", err)
		return nil, ErrAIUnavailable
	}

	switch resp.StopReason {
	case anthropic.StopReasonRefusal, anthropic.StopReasonMaxTokens:
		log.Printf("anthropic breakdown stopped early: stop_reason=%s", resp.StopReason)
		return nil, ErrAIBadOutput
	}

	var text string
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text = tb.Text
			break
		}
	}
	if text == "" {
		return nil, ErrAIBadOutput
	}

	var out struct {
		Subtasks []BreakdownSuggestion `json:"subtasks"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		log.Printf("anthropic breakdown returned unparseable JSON: %v", err)
		return nil, ErrAIBadOutput
	}
	if len(out.Subtasks) > 10 {
		out.Subtasks = out.Subtasks[:10]
	}
	// The model's output is untrusted like any other input — same checks
	// the apply endpoint runs on the client-submitted batch.
	if err := validateSuggestions(out.Subtasks); err != nil {
		log.Printf("anthropic breakdown failed validation: %v", err)
		return nil, ErrAIBadOutput
	}
	return out.Subtasks, nil
}
