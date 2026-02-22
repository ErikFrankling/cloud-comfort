package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamCallbacks receives streaming deltas as they arrive.
type StreamCallbacks struct {
	OnContent   func(text string)
	OnReasoning func(text string)
}

// Config holds the OpenAI-compatible API configuration.
type Config struct {
	BaseURL       string // e.g. https://openrouter.ai/api/v1
	APIKey        string
	Model         string // e.g. openai/gpt-5.2
	Streaming     bool
	MaxTokens     int    // 0 = provider default
	ProviderSort  string // "throughput", "latency", or "price" (OpenRouter)
	Quantizations string // comma-separated: "fp16,fp8,int4" (OpenRouter)
}

// Client is an OpenAI-compatible chat completions client.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient creates a new LLM client.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{},
	}
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool call from the assistant.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool defines a tool available to the LLM.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction defines a function tool.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// providerPreferences controls OpenRouter provider routing.
type providerPreferences struct {
	AllowFallbacks bool     `json:"allow_fallbacks"`
	Sort           string   `json:"sort,omitempty"`           // "price", "throughput", "latency"
	Quantizations  []string `json:"quantizations,omitempty"`  // e.g. ["fp16","fp8","int8","int4"]
}

// chatRequest is the request body for chat completions.
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
	MaxTokens int      `json:"max_tokens,omitempty"`
	Provider  *providerPreferences `json:"provider,omitempty"`
}

// chatResponse is the non-streaming response.
type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// streamChunk is a single SSE chunk from the streaming API.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string      `json:"content,omitempty"`
			ReasoningContent string      `json:"reasoning_content,omitempty"`
			ToolCalls        []toolDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type toolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

// buildRequest constructs a chatRequest with provider preferences from config.
func (c *Client) buildRequest(messages []Message, tools []Tool, stream bool) chatRequest {
	req := chatRequest{
		Model:     c.cfg.Model,
		Messages:  messages,
		Tools:     tools,
		Stream:    stream,
		MaxTokens: c.cfg.MaxTokens,
	}

	// Build OpenRouter provider preferences if configured
	if c.cfg.ProviderSort != "" || c.cfg.Quantizations != "" {
		pref := &providerPreferences{AllowFallbacks: true}
		if c.cfg.ProviderSort != "" {
			pref.Sort = c.cfg.ProviderSort
		}
		if c.cfg.Quantizations != "" {
			for _, q := range strings.Split(c.cfg.Quantizations, ",") {
				q = strings.TrimSpace(q)
				if q != "" {
					pref.Quantizations = append(pref.Quantizations, q)
				}
			}
		}
		req.Provider = pref
	}

	return req
}

// ChatStream sends a streaming chat completion request. Content and reasoning
// deltas are forwarded via callbacks as they arrive. Returns the full
// accumulated assistant message.
func (c *Client) ChatStream(ctx context.Context, messages []Message, tools []Tool, cb *StreamCallbacks) (*Message, error) {
	if !c.cfg.Streaming {
		return c.Chat(ctx, messages, tools)
	}

	body, err := json.Marshal(c.buildRequest(messages, tools, true))
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	// Accumulate the full message
	result := &Message{Role: "assistant"}
	var contentBuf strings.Builder
	var reasoningBuf strings.Builder
	toolCallMap := make(map[int]*ToolCall) // index -> accumulated tool call

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// Stream reasoning deltas
		if delta.ReasoningContent != "" {
			reasoningBuf.WriteString(delta.ReasoningContent)
			if cb != nil && cb.OnReasoning != nil {
				cb.OnReasoning(delta.ReasoningContent)
			}
		}

		// Stream content deltas
		if delta.Content != "" {
			contentBuf.WriteString(delta.Content)
			if cb != nil && cb.OnContent != nil {
				cb.OnContent(delta.Content)
			}
		}

		// Accumulate tool call deltas
		for _, tc := range delta.ToolCalls {
			existing, ok := toolCallMap[tc.Index]
			if !ok {
				existing = &ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
				}
				toolCallMap[tc.Index] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Type != "" {
				existing.Type = tc.Type
			}
			if tc.Function.Name != "" {
				existing.Function.Name = tc.Function.Name
			}
			existing.Function.Arguments += tc.Function.Arguments
		}
	}

	result.Content = contentBuf.String()
	result.Reasoning = reasoningBuf.String()

	// Collect tool calls in order
	for i := 0; i < len(toolCallMap); i++ {
		if tc, ok := toolCallMap[i]; ok {
			result.ToolCalls = append(result.ToolCalls, *tc)
		}
	}

	return result, nil
}

// Chat sends a non-streaming chat completion request.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	body, err := json.Marshal(c.buildRequest(messages, tools, false))
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &chatResp.Choices[0].Message, nil
}
