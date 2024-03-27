package gpt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gogpt "github.com/sashabaranov/go-openai"
)

var ErrTooManyRequests = errors.New("too many requests")

type Config struct {
	Keys    []string
	Timeout time.Duration
}

type Client struct {
	id int
	*gogpt.Client
}

type Handler struct {
	sync.Mutex
	cfg     Config
	index   int
	clients []*Client
}

type expectedType struct {
	Translations []string `json:"translations"`
}

func New(cfg Config) *Handler {
	h := &Handler{
		cfg:     cfg,
		clients: make([]*Client, len(cfg.Keys)),
	}
	for i, key := range cfg.Keys {
		c := &Client{
			id:     i,
			Client: gogpt.NewClient(key),
		}
		h.clients[i] = c
	}
	return h
}

func (h *Handler) Translate(ctx context.Context, src, lang string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, h.cfg.Timeout)
	defer cancel()
	h.Lock()
	client := h.clients[h.index]
	h.index = (h.index + 1) % len(h.clients)
	h.Unlock()

	msg := gogpt.ChatCompletionMessage{
		Role: "user", Content: fmt.Sprintf("Translate \"%s\" to %s. Give the result directly. Don't explain. Don't quote output.", src, lang),
	}

	request := gogpt.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []gogpt.ChatCompletionMessage{
			msg,
		},
		MaxTokens:   1024,
		Stop:        []string{"STOP"},
		Temperature: 0.1,
	}

	resp, err := client.CreateChatCompletion(ctx, request)
	if err != nil {
		var perr *gogpt.APIError

		if errors.As(err, &perr) {
			if perr.HTTPStatusCode == 429 {
				return "", ErrTooManyRequests
			}
		}

		var cerr *gogpt.RequestError
		if errors.As(err, &cerr) {
			if cerr.HTTPStatusCode == 429 {
				return "", ErrTooManyRequests
			}
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return "", ErrTooManyRequests
		}

		return "", err
	}

	result := ""
	if len(resp.Choices) > 0 {
		result = strings.TrimSpace(resp.Choices[0].Message.Content)
	}
	return result, err
}

func (h *Handler) BatchTranslate(ctx context.Context, srcs []string, lang string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, h.cfg.Timeout)
	defer cancel()

	h.Lock()
	client := h.clients[h.index]
	h.index = (h.index + 1) % len(h.clients)
	h.Unlock()

	// Construct the prompt as a single instruction for the model
	phrasesJSON, _ := json.Marshal(srcs)
	prompt := fmt.Sprintf("Translate the following phrases into %s, and return the translations as a JSON array (JSON object must be of type {translations: ['a','a'...]}) : %s", lang, string(phrasesJSON))

	// Prepare the chat completion request
	messages := []gogpt.ChatCompletionMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	request := gogpt.ChatCompletionRequest{
		Model:       "gpt-3.5-turbo-0125",
		Messages:    messages,
		MaxTokens:   1024,
		Temperature: 0.1,
		ResponseFormat: &gogpt.ChatCompletionResponseFormat{
			Type: "json_object",
		},
		// Stop: []string{"\"],\""}, // Adjust based on how you expect the model to close the JSON array
	}

	// Perform the chat completion request
	resp, err := client.CreateChatCompletion(ctx, request)
	if err != nil {
		return nil, err
	}

	// Extract the last message assuming it contains the JSON array with translations
	if len(resp.Choices) < 1 || len(resp.Choices[0].Message.Content) == 0 {
		return nil, fmt.Errorf("no translation response received")
	}
	lastMessage := resp.Choices[0].Message.Content

	// Parse the JSON array from the last message
	var translations expectedType
	err = json.Unmarshal([]byte(lastMessage), &translations)
	if err != nil {
		return nil, fmt.Errorf("failed to parse translations from response: %w", err)
	}

	return translations.Translations, nil
}
