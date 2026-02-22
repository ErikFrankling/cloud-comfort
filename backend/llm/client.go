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

// Provider represents the LLM provider type
type Provider string

const (
	ProviderOpenRouter Provider = "openrouter"
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderOllama     Provider = "ollama"
)

// StreamCallbacks receives streaming deltas as they arrive.
type StreamCallbacks struct {
	OnContent   func(text string)
	OnReasoning func(text string)
}

// Config holds the API configuration.
type Config struct {
	Provider      Provider // openrouter, openai, anthropic, ollama
	BaseURL       string
	APIKey        string
	Model         string
	Streaming     bool
	MaxTokens     int
	ProviderSort  string // OpenRouter specific
	Quantizations string // OpenRouter specific
}

// Client is an LLM client that abstracts over different providers.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient creates a new LLM client.
func NewClient(cfg Config) *Client {
	// Set defaults based on provider
	if cfg.Provider == "" {
		cfg.Provider = ProviderOpenRouter
	}

	if cfg.BaseURL == "" {
		switch cfg.Provider {
		case ProviderAnthropic:
			cfg.BaseURL = "https://api.anthropic.com"
		case ProviderOpenAI:
			cfg.BaseURL = "https://api.openai.com/v1"
		case ProviderOllama:
			cfg.BaseURL = "http://localhost:11434"
		default: // ProviderOpenRouter and others
			cfg.BaseURL = "https://openrouter.ai/api/v1"
		}
	}

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

// ChatStream sends a streaming chat completion request.
func (c *Client) ChatStream(ctx context.Context, messages []Message, tools []Tool, cb *StreamCallbacks) (*Message, error) {
	switch c.cfg.Provider {
	case ProviderAnthropic:
		return c.chatStreamAnthropic(ctx, messages, tools, cb)
	default:
		// OpenAI-compatible providers (OpenRouter, OpenAI, Ollama)
		return c.chatStreamOpenAI(ctx, messages, tools, cb)
	}
}

// Chat sends a non-streaming chat completion request.
func (c *Client) Chat(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	switch c.cfg.Provider {
	case ProviderAnthropic:
		return c.chatAnthropic(ctx, messages, tools)
	default:
		return c.chatOpenAI(ctx, messages, tools)
	}
}

// OpenAI-compatible implementation

// providerPreferences controls OpenRouter provider routing.
type providerPreferences struct {
	AllowFallbacks bool     `json:"allow_fallbacks"`
	Sort           string   `json:"sort,omitempty"`
	Quantizations  []string `json:"quantizations,omitempty"`
}

// openAIChatRequest is the request body for OpenAI-compatible APIs.
type openAIChatRequest struct {
	Model     string               `json:"model"`
	Messages  []Message            `json:"messages"`
	Tools     []Tool               `json:"tools,omitempty"`
	Stream    bool                 `json:"stream"`
	MaxTokens int                  `json:"max_tokens,omitempty"`
	Provider  *providerPreferences `json:"provider,omitempty"`
}

// openAIChatResponse is the non-streaming response.
type openAIChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

// openAIStreamChunk is a single SSE chunk.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string            `json:"content,omitempty"`
			ReasoningContent string            `json:"reasoning_content,omitempty"`
			ToolCalls        []openAIToolDelta `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type openAIToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

func (c *Client) buildOpenAIRequest(messages []Message, tools []Tool, stream bool) openAIChatRequest {
	req := openAIChatRequest{
		Model:     c.cfg.Model,
		Messages:  messages,
		Tools:     tools,
		Stream:    stream,
		MaxTokens: c.cfg.MaxTokens,
	}

	// Build OpenRouter provider preferences if configured
	if c.cfg.Provider == ProviderOpenRouter && (c.cfg.ProviderSort != "" || c.cfg.Quantizations != "") {
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

func (c *Client) chatStreamOpenAI(ctx context.Context, messages []Message, tools []Tool, cb *StreamCallbacks) (*Message, error) {
	if !c.cfg.Streaming {
		return c.chatOpenAI(ctx, messages, tools)
	}

	body, err := json.Marshal(c.buildOpenAIRequest(messages, tools, true))
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
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

	result := &Message{Role: "assistant"}
	var contentBuf strings.Builder
	var reasoningBuf strings.Builder
	toolCallMap := make(map[int]*ToolCall)

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

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.ReasoningContent != "" {
			reasoningBuf.WriteString(delta.ReasoningContent)
			if cb != nil && cb.OnReasoning != nil {
				cb.OnReasoning(delta.ReasoningContent)
			}
		}

		if delta.Content != "" {
			contentBuf.WriteString(delta.Content)
			if cb != nil && cb.OnContent != nil {
				cb.OnContent(delta.Content)
			}
		}

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

	for i := 0; i < len(toolCallMap); i++ {
		if tc, ok := toolCallMap[i]; ok {
			result.ToolCalls = append(result.ToolCalls, *tc)
		}
	}

	return result, nil
}

func (c *Client) chatOpenAI(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	body, err := json.Marshal(c.buildOpenAIRequest(messages, tools, false))
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
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

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &chatResp.Choices[0].Message, nil
}

// Anthropic implementation

// anthropicMessage represents a message in Anthropic format.
// Content can be a string or []anthropicRequestBlock for tool interactions.
type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// anthropicTool represents a tool in Anthropic format
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicChatRequest is the request body for Anthropic API
type anthropicChatRequest struct {
	Model     string             `json:"model"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens,omitempty"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

// anthropicChatResponse is the non-streaming response
type anthropicChatResponse struct {
	Content []anthropicContent `json:"content"`
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Role    string             `json:"role"`
	Usage   anthropicUsage     `json:"usage"`
}

type anthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicStreamEvent represents a streaming event from Anthropic
type anthropicStreamEvent struct {
	Type         string                `json:"type"`
	Index        int                   `json:"index,omitempty"`
	Delta        anthropicDelta        `json:"delta,omitempty"`
	ContentBlock anthropicContentBlock `json:"content_block,omitempty"`
}

type anthropicDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// anthropicRequestBlock represents a content block in an Anthropic API request message.
// Used for tool_use (in assistant messages) and tool_result (in user messages).
type anthropicRequestBlock struct {
	Type      string          `json:"type"`                  // "text", "tool_use", "tool_result"
	Text      string          `json:"text,omitempty"`        // for type="text"
	ID        string          `json:"id,omitempty"`          // for type="tool_use"
	Name      string          `json:"name,omitempty"`        // for type="tool_use"
	Input     json.RawMessage `json:"input,omitempty"`       // for type="tool_use"
	ToolUseID string          `json:"tool_use_id,omitempty"` // for type="tool_result"
	Content   string          `json:"content,omitempty"`     // for type="tool_result"
}

// convertMessagesToAnthropic converts OpenAI-style messages to Anthropic format.
// Handles tool_use content blocks (assistant), tool_result blocks (tool→user),
// and merges consecutive same-role messages as required by the Anthropic API.
func convertMessagesToAnthropic(messages []Message) []anthropicMessage {
	var result []anthropicMessage

	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}

		switch {
		case msg.Role == "assistant" && len(msg.ToolCalls) > 0:
			// Assistant message with tool calls → content block array
			var blocks []anthropicRequestBlock
			if msg.Content != "" {
				blocks = append(blocks, anthropicRequestBlock{
					Type: "text",
					Text: msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicRequestBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			result = append(result, anthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})

		case msg.Role == "assistant":
			// Simple assistant text message
			result = append(result, anthropicMessage{
				Role:    "assistant",
				Content: msg.Content,
			})

		case msg.Role == "tool":
			// Tool result → user message with tool_result content block.
			// Merge with previous user message if one exists (Anthropic
			// requires all tool_results in a single user turn).
			block := anthropicRequestBlock{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   msg.Content,
			}
			if len(result) > 0 && result[len(result)-1].Role == "user" {
				if blocks, ok := result[len(result)-1].Content.([]anthropicRequestBlock); ok {
					result[len(result)-1].Content = append(blocks, block)
					continue
				}
			}
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: []anthropicRequestBlock{block},
			})

		default: // user message
			// Merge consecutive user messages (Anthropic disallows them)
			if len(result) > 0 && result[len(result)-1].Role == "user" {
				switch prev := result[len(result)-1].Content.(type) {
				case string:
					result[len(result)-1].Content = prev + "\n\n" + msg.Content
				case []anthropicRequestBlock:
					result[len(result)-1].Content = append(prev, anthropicRequestBlock{
						Type: "text",
						Text: msg.Content,
					})
				}
				continue
			}
			result = append(result, anthropicMessage{
				Role:    "user",
				Content: msg.Content,
			})
		}
	}

	return result
}

// convertToolsToAnthropic converts OpenAI-style tools to Anthropic format
func convertToolsToAnthropic(tools []Tool) []anthropicTool {
	result := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	return result
}

// extractSystemMessage extracts the system message from messages
func extractSystemMessage(messages []Message) string {
	for _, msg := range messages {
		if msg.Role == "system" {
			return msg.Content
		}
	}
	return ""
}

func (c *Client) chatStreamAnthropic(ctx context.Context, messages []Message, tools []Tool, cb *StreamCallbacks) (*Message, error) {
	if !c.cfg.Streaming {
		return c.chatAnthropic(ctx, messages, tools)
	}

	systemMsg := extractSystemMessage(messages)
	anthropicMsgs := convertMessagesToAnthropic(messages)

	// Anthropic requires max_tokens, default to 4096 if not set
	maxTokens := c.cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	reqBody := anthropicChatRequest{
		Model:     c.cfg.Model,
		System:    systemMsg,
		Messages:  anthropicMsgs,
		MaxTokens: maxTokens,
		Tools:     convertToolsToAnthropic(tools),
		Stream:    true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.cfg.BaseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	result := &Message{Role: "assistant"}
	var contentBuf strings.Builder
	var reasoningBuf strings.Builder
	toolCallMap := make(map[int]*ToolCall)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		// SSE format: event: xxx\ndata: xxx\n\n
		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")
			// Read the data line
			if !scanner.Scan() {
				break
			}
			dataLine := strings.TrimRight(scanner.Text(), "\r")
			if !strings.HasPrefix(dataLine, "data: ") {
				continue
			}
			data := strings.TrimPrefix(dataLine, "data: ")

			if data == "[DONE]" {
				break
			}

			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch eventType {
			case "content_block_delta":
				if event.Delta.Type == "text_delta" {
					text := event.Delta.Text
					contentBuf.WriteString(text)
					if cb != nil && cb.OnContent != nil {
						cb.OnContent(text)
					}
				} else if event.Delta.Type == "input_json_delta" {
					// Accumulate tool call arguments
					if tc, ok := toolCallMap[event.Index]; ok {
						tc.Function.Arguments += event.Delta.PartialJSON
					}
				}
			case "content_block_start":
				if event.ContentBlock.Type == "tool_use" {
					toolCallMap[event.Index] = &ToolCall{
						ID:   event.ContentBlock.ID,
						Type: "function",
						Function: FunctionCall{
							Name:      event.ContentBlock.Name,
							Arguments: "", // Will be accumulated from input_json_delta events
						},
					}
				}
			}
		}
	}

	result.Content = contentBuf.String()
	result.Reasoning = reasoningBuf.String()

	// Collect tool calls in index order. Indices may not start at 0
	// (e.g. index 0 is text, indices 1+ are tool_use blocks).
	maxIdx := -1
	for idx := range toolCallMap {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	for i := 0; i <= maxIdx; i++ {
		if tc, ok := toolCallMap[i]; ok {
			result.ToolCalls = append(result.ToolCalls, *tc)
		}
	}

	return result, nil
}

func (c *Client) chatAnthropic(ctx context.Context, messages []Message, tools []Tool) (*Message, error) {
	systemMsg := extractSystemMessage(messages)
	anthropicMsgs := convertMessagesToAnthropic(messages)

	// Anthropic requires max_tokens, default to 4096 if not set
	maxTokens := c.cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	reqBody := anthropicChatRequest{
		Model:     c.cfg.Model,
		System:    systemMsg,
		Messages:  anthropicMsgs,
		MaxTokens: maxTokens,
		Tools:     convertToolsToAnthropic(tools),
		Stream:    false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := c.cfg.BaseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var anthResp anthropicChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	result := &Message{Role: "assistant"}

	// Process content blocks
	for _, content := range anthResp.Content {
		switch content.Type {
		case "text":
			result.Content += content.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   content.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      content.Name,
					Arguments: string(content.Input),
				},
			})
		}
	}

	return result, nil
}
