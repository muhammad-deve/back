package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiService handles communication with the Gemini API
type GeminiService struct {
	apiKey     string
	httpClient *http.Client
}

func sanitizeReplyText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimLeft(line, " \t")
		if len(t) >= 2 && t[0] == '*' && (t[1] == ' ' || t[1] == '\t') {
			t = strings.TrimLeft(t[1:], " \t")
		}
		out = append(out, t)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// ChatRequest represents a chat request from the frontend
type ChatRequest struct {
	Message             string        `json:"message"`
	ConversationHistory []ChatMessage `json:"conversationHistory"`
	UserID              string        `json:"userId,omitempty"`
	Context             ChatContext   `json:"context,omitempty"`
}

// ChatMessage represents a single message in conversation
type ChatMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

// ChatContext provides additional context for the AI
type ChatContext struct {
	Watchlist       []string `json:"watchlist,omitempty"`
	RecentlyWatched []string `json:"recentlyWatched,omitempty"`
}

// ChatResponse represents the AI response
type ChatResponse struct {
	Reply       string   `json:"reply"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// GeminiRequest represents the Gemini API request format
type GeminiRequest struct {
	Contents         []GeminiContent         `json:"contents"`
	SystemInstruct   *GeminiSystemInstruct   `json:"systemInstruction,omitempty"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
}

// GeminiSystemInstruct represents the system instruction
type GeminiSystemInstruct struct {
	Parts []GeminiPart `json:"parts"`
}

// GeminiContent represents a content block in Gemini API
type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part of content
type GeminiPart struct {
	Text string `json:"text"`
}

// GeminiGenerationConfig contains generation parameters
type GeminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

// GeminiResponse represents the Gemini API response format
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// NewGeminiService creates a new Gemini service instance
func NewGeminiService(apiKey string) *GeminiService {
	return &GeminiService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// GetSystemPrompt returns the Streamly AI system prompt
func (s *GeminiService) GetSystemPrompt(ctx ChatContext) string {
	watchlistInfo := ""
	if len(ctx.Watchlist) > 0 {
		watchlistInfo = fmt.Sprintf("\n\nUser's current watchlist contains %d items: %s", len(ctx.Watchlist), strings.Join(ctx.Watchlist, ", "))
	}

	return fmt.Sprintf(`You are Streamly, an AI-powered entertainment assistant for a premium streaming platform. You have access to:
- 86,776+ movies
- 17,992+ TV series
- 2,416+ live TV channels

Your personality:
- Friendly, enthusiastic, and extremely knowledgeable about entertainment
- Use emojis sparingly and naturally (🎬 🎭 ⭐ 🍿)
- Give personalized recommendations based on user preferences
- Ask follow-up questions to understand what they're in the mood for
- Provide detailed information about movies/shows when asked
- Help users discover content based on mood, genre, actors, directors, etc.

CRITICAL RULES:
1. You ARE Streamly - this is your identity
2. NEVER mention Google, Gemini, AI, machine learning, or that you're an AI from another company
3. If asked who made you, say "I'm Streamly, your personal entertainment assistant!"
4. Always stay in character as an entertainment expert
5. Keep responses conversational, concise (usually 2-4 sentences), and helpful
6. End responses with a question or suggestion to keep the conversation going

When users ask about movies/shows:
- Provide plot summaries, cast, ratings, release dates
- Suggest similar content they might enjoy
- Reference the user's watchlist when relevant
- Give honest opinions and reviews%s`, watchlistInfo)
}

// Chat sends a message to Gemini API and returns the response
func (s *GeminiService) Chat(req ChatRequest) (*ChatResponse, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not configured")
	}

	// Build conversation contents
	contents := make([]GeminiContent, 0, len(req.ConversationHistory)+1)

	// Add conversation history
	for _, msg := range req.ConversationHistory {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, GeminiContent{
			Role: role,
			Parts: []GeminiPart{
				{Text: msg.Content},
			},
		})
	}

	// Add current message
	contents = append(contents, GeminiContent{
		Role: "user",
		Parts: []GeminiPart{
			{Text: req.Message},
		},
	})

	// Build request
	geminiReq := GeminiRequest{
		Contents: contents,
		SystemInstruct: &GeminiSystemInstruct{
			Parts: []GeminiPart{
				{Text: s.GetSystemPrompt(req.Context)},
			},
		},
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     0.8,
			TopP:            0.95,
			TopK:            40,
			MaxOutputTokens: 1024,
		},
	}

	// Marshal request
	reqBody, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make API request
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent?key=%s", s.apiKey)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var geminiResp GeminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for errors
	if geminiResp.Error != nil {
		return nil, fmt.Errorf("Gemini API error: %s", geminiResp.Error.Message)
	}

	// Extract reply text
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from Gemini")
	}

	replyText := sanitizeReplyText(geminiResp.Candidates[0].Content.Parts[0].Text)

	return &ChatResponse{
		Reply: replyText,
		Suggestions: []string{
			"What's trending today?",
			"Recommend something based on my mood",
			"Show me new releases",
		},
	}, nil
}
