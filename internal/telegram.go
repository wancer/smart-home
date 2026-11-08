package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	telegramQueueSize   = 100
	telegramBatchWindow = 2 * time.Second // messages arriving within this window are sent as one
	telegramMaxLength   = 4000            // API limit is 4096 chars per message
)

type TelegramNotifier struct {
	botToken string
	chatID   string
	client   *http.Client
	queue    chan string
}

func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	t := &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
		queue:    make(chan string, telegramQueueSize),
	}
	if !t.Enabled() {
		slog.Warn("[telegram] TELEGRAM_BOT_TOKEN / TELEGRAM_CHAT_ID not set, notifications will only be logged")
	}
	return t
}

func (t *TelegramNotifier) Enabled() bool {
	return t.botToken != "" && t.chatID != ""
}

// Notify queues a message for asynchronous delivery; it never blocks.
func (t *TelegramNotifier) Notify(text string) {
	slog.Info("[telegram] notify", "text", text)
	if !t.Enabled() {
		return
	}
	select {
	case t.queue <- text:
	default:
		slog.Warn("[telegram] queue full, dropping message", "text", text)
	}
}

// Send delivers a message synchronously.
func (t *TelegramNotifier) Send(text string) error {
	if !t.Enabled() {
		return errors.New("telegram is not configured")
	}
	return t.send(text)
}

// Run delivers queued messages. Bursts (e.g. every device going offline when the
// broker is down) are merged into one message to stay within Telegram rate limits.
func (t *TelegramNotifier) Run() chan struct{} {
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case first := <-t.queue:
				batch := []string{first}
				timer := time.NewTimer(telegramBatchWindow)
			collect:
				for {
					select {
					case text := <-t.queue:
						batch = append(batch, text)
					case <-timer.C:
						break collect
					case <-quit:
						timer.Stop()
						break collect
					}
				}
				for _, chunk := range chunkLines(batch, telegramMaxLength) {
					if err := t.send(chunk); err != nil {
						slog.Error("[telegram] send error", "err", err)
					}
				}
			case <-quit:
				return
			}
		}
	}()
	return quit
}

// chunkLines joins messages with newlines into chunks no longer than max.
func chunkLines(lines []string, max int) []string {
	var chunks []string
	var cur strings.Builder
	for _, line := range lines {
		if len(line) > max {
			line = line[:max]
		}
		if cur.Len() > 0 && cur.Len()+1+len(line) > max {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n")
		}
		cur.WriteString(line)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks
}

func (t *TelegramNotifier) SendGrabResult(updated, failed []string) error {
	return t.send(grabResultMessage(updated, failed))
}

func grabResultMessage(updated, failed []string) string {
	if len(updated) == 0 && len(failed) == 0 {
		return "No new data"
	}
	var parts []string
	if len(updated) > 0 {
		parts = append(parts, "Got new data for:\n"+bulletList(updated))
	}
	if len(failed) > 0 {
		parts = append(parts, "Errors fetching:\n"+bulletList(failed))
	}
	return strings.Join(parts, "\n\n")
}

func bulletList(items []string) string {
	lines := make([]string, len(items))
	for i, item := range items {
		lines[i] = "• " + item
	}
	return strings.Join(lines, "\n")
}

func (t *TelegramNotifier) send(text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
	resp, err := t.client.PostForm(apiURL, url.Values{
		"chat_id": {t.chatID},
		"text":    {text},
	})
	if err != nil {
		// the URL contains the bot token; don't leak it into logs
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return urlErr.Err
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}
