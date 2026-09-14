package engine

import (
	"encoding/json"
	"time"
)

// Role of a chat message.
type Role string

// Chat roles. Tool results carry RoleTool and Message.ToolCallID.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// PartType is the kind of a content part.
type PartType string

// Content part kinds.
const (
	PartText  PartType = "text"
	PartImage PartType = "image"
)

// Part is one piece of message content.
type Part struct {
	Type  PartType `json:"type"`
	Text  string   `json:"text,omitempty"`
	Image *Image   `json:"image,omitempty"`
}

// Image is an inline or referenced image. Exactly one of Data or URL is set.
type Image struct {
	MIME string `json:"mime"`
	Data []byte `json:"data,omitempty"`
	URL  string `json:"url,omitempty"`
}

// Message is one turn of the conversation in the engine-neutral model.
type Message struct {
	Role       Role       `json:"role"`
	Parts      []Part     `json:"parts,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant turns that called tools
	ToolCallID string     `json:"tool_call_id,omitempty"` // RoleTool: which call this answers
	Name       string     `json:"name,omitempty"`         // RoleTool: tool name
}

// Text returns the concatenated text parts of the message.
func (m Message) Text() string {
	var out string
	for _, p := range m.Parts {
		if p.Type == PartText {
			out += p.Text
		}
	}
	return out
}

// TextMessage builds a single-text-part message.
func TextMessage(role Role, text string) Message {
	return Message{Role: role, Parts: []Part{{Type: PartText, Text: text}}}
}

// ToolCall is a completed tool invocation requested by the model.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"` // JSON object
}

// Tool is a function the model may call, with a JSON Schema for its arguments.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolChoiceMode constrains tool use.
type ToolChoiceMode string

// Tool choice modes.
const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceNamed    ToolChoiceMode = "named"
)

// ToolChoice selects how the model may use tools. Zero value means auto.
type ToolChoice struct {
	Mode ToolChoiceMode `json:"mode,omitempty"`
	Name string         `json:"name,omitempty"` // ToolChoiceNamed
}

// Sampling holds decoding parameters. Nil pointers mean "engine default".
type Sampling struct {
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
	TopK        *int     `json:"top_k,omitempty"`
	MinP        *float64 `json:"min_p,omitempty"`
	Seed        *int64   `json:"seed,omitempty"`
	MaxTokens   int      `json:"max_tokens,omitempty"` // 0 = engine default
	Stop        []string `json:"stop,omitempty"`
}

// ResponseFormat constrains the output shape (phase 8 for real engines).
type ResponseFormat struct {
	Type   string          `json:"type"` // "text", "json_object", "json_schema"
	Schema json.RawMessage `json:"schema,omitempty"`
}

// ChatRequest is the engine-neutral completion request, already normalized
// by the server layer from the OpenAI or Anthropic dialect.
type ChatRequest struct {
	RequestID      string          `json:"request_id,omitempty"`
	Messages       []Message       `json:"messages"`
	Tools          []Tool          `json:"tools,omitempty"`
	ToolChoice     ToolChoice      `json:"tool_choice,omitempty"`
	Sampling       Sampling        `json:"sampling"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Stream         bool            `json:"stream"`
	Think          *bool           `json:"think,omitempty"` // reasoning toggle when supported
}

// EventType is the kind of a streamed Event.
type EventType string

// Event kinds.
const (
	EventToken    EventType = "token"     // Text carries a text delta
	EventToolCall EventType = "tool_call" // ToolCall carries an arguments delta
	EventUsage    EventType = "usage"     // Usage carries the final counts and timings
	EventDone     EventType = "done"      // FinishReason is set; last event of a success
	EventError    EventType = "error"     // Err is set; last event of a failure
)

// FinishReason explains why generation ended.
type FinishReason string

// Finish reasons.
const (
	FinishStop      FinishReason = "stop"
	FinishLength    FinishReason = "length"
	FinishToolCalls FinishReason = "tool_calls"
	FinishCancelled FinishReason = "cancelled"
	FinishError     FinishReason = "error"
)

// Event is one item of a Runner.Chat stream.
//
// Contract: zero or more EventToken/EventToolCall events, then at most one
// EventUsage, then exactly one terminal event (EventDone or EventError),
// after which the channel is closed. When the request context is cancelled
// the runner stops promptly and closes the channel; it may emit a final
// EventError carrying the context error if the consumer is still reading.
type Event struct {
	Type         EventType      `json:"type"`
	Text         string         `json:"text,omitempty"`
	ToolCall     *ToolCallDelta `json:"tool_call,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
	FinishReason FinishReason   `json:"finish_reason,omitempty"`
	Err          error          `json:"-"`
}

// ToolCallDelta is an incremental piece of a tool call. ID and Name are set
// on the first delta of a given Index; ArgumentsDelta pieces concatenate to
// a JSON object.
type ToolCallDelta struct {
	Index          int    `json:"index"`
	ID             string `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	ArgumentsDelta string `json:"arguments_delta,omitempty"`
}

// Usage holds token counts and timings of one completion.
type Usage struct {
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	TTFT             time.Duration `json:"ttft"`
	PrefillTPS       float64       `json:"prefill_tps"`
	DecodeTPS        float64       `json:"decode_tps"`
	Duration         time.Duration `json:"duration"`
}
