package gpt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gogpt "github.com/sashabaranov/go-gpt3"
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
			if perr.StatusCode == 429 {
				return "", ErrTooManyRequests
			}
		}

		var cerr *gogpt.RequestError
		if errors.As(err, &cerr) {
			if cerr.StatusCode == 429 {
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
