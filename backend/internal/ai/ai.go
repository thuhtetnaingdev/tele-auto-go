package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"tele-auto-go/internal/util"
)

type Client struct {
	baseURL   string
	apiKey    string
	model     string
	maxTokens int
	http      *http.Client
	logger    *slog.Logger
}

func New(baseURL, apiKey, model string, maxTokens int, logger *slog.Logger) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		maxTokens: func() int {
			if maxTokens < 64 {
				return 64
			}
			return maxTokens
		}(),
		http: &http.Client{
			Timeout: 90 * time.Second,
		},
		logger: logger,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	N           int           `json:"n"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatCompletionResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content any `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage map[string]any `json:"usage"`
}

type ChatParams struct {
	SystemPrompt string
	UserPrompt   string
	Model        string
	Temperature  float64
	MaxTokens    int
}

func (c *Client) GenerateReply(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reply, finishReason, usage, err := c.generateOnce(ctx, ChatParams{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0.45,
	})
	if err != nil {
		return "", err
	}

	if reply == "" {
		c.logger.Warn("AI returned blank output, retrying once", "model", c.model, "finish_reason", finishReason, "usage", usage)
		retrySystem := systemPrompt + "\n\nCritical output rule: output only one final short reply sentence. No analysis."
		reply, _, _, err = c.generateOnce(ctx, ChatParams{
			SystemPrompt: retrySystem,
			UserPrompt:   userPrompt,
			Temperature:  0.25,
		})
		if err != nil {
			return "", err
		}
	}

	reply = util.NormalizeSpace(util.StripThinking(reply))
	if reply == "" {
		return "", nil
	}
	reply = util.ClampWords(reply, 40)
	return reply, nil
}

func (c *Client) Chat(ctx context.Context, params ChatParams) (string, error) {
	reply, _, _, err := c.generateOnce(ctx, params)
	if err != nil {
		return "", err
	}
	return util.NormalizeSpace(util.StripThinking(reply)), nil
}

func (c *Client) generateOnce(
	ctx context.Context,
	params ChatParams,
) (reply string, finishReason string, usage map[string]any, err error) {
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = c.model
	}
	temperature := params.Temperature
	if temperature == 0 {
		temperature = 0.4
	}
	maxTokens := params.MaxTokens
	if maxTokens < 64 {
		maxTokens = c.maxTokens
	}

	payload := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: params.SystemPrompt},
			{Role: "user", Content: params.UserPrompt},
		},
		Temperature: temperature,
		N:           1,
		MaxTokens:   maxTokens,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", "", nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", nil, fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", nil, fmt.Errorf("ai status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", "", parsed.Usage, nil
	}

	content := extractContent(parsed.Choices[0].Message.Content)
	content = util.NormalizeSpace(util.StripThinking(content))
	return content, parsed.Choices[0].FinishReason, parsed.Usage, nil
}

func extractContent(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var parts []string
		for _, p := range t {
			obj, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if txt, ok := obj["text"].(string); ok {
				parts = append(parts, txt)
				continue
			}
			if txt, ok := obj["content"].(string); ok {
				parts = append(parts, txt)
				continue
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}
