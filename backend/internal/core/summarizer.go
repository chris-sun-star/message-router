package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/admin/message-router/internal/types"
	"google.golang.org/genai"
)

type LLMSummarizer struct {
	provider string
	model    string
	apiKey   string
}

func NewLLMSummarizer(provider, model, apiKey string) *LLMSummarizer {
	return &LLMSummarizer{
		provider: provider,
		model:    model,
		apiKey:   apiKey,
	}
}

func (s *LLMSummarizer) Summarize(ctx context.Context, messages []types.Message) (string, error) {
	if len(messages) == 0 {
		return "No new messages to summarize.", nil
	}

	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Source, m.Sender, m.Content))
	}

	prompt := fmt.Sprintf(`Summarize these messages from various chat platforms. 
Highlight key actions, questions, or decisions. Group by topic or chat if relevant. Use Markdown.

Messages:
%s`, sb.String())

	switch s.provider {
	case "gemini":
		client, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey: s.apiKey,
		})
		if err != nil {
			return "", err
		}
		resp, err := client.Models.GenerateContent(ctx, s.model, genai.Text(prompt), nil)
		if err != nil {
			return "", err
		}
		if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
			return "No summary generated.", nil
		}
		return fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0]), nil
	default:
		return "", fmt.Errorf("unsupported provider: %s", s.provider)
	}
}
