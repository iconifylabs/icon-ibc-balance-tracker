package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

var (
	telegramBotToken  = os.Getenv("TELEGRAM_BOT_TOKEN")
	telegramChatID    = os.Getenv("TELEGRAM_CHAT_ID")
	discordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
	balanceThreshold  = 100 // replace with your threshold
)

type TelegramMessage struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type DiscordMessage struct {
	Content string `json:"content"`
}

func sendTelegramAlert(message string) error {
	msg := TelegramMessage{
		ChatID: telegramChatID,
		Text:   message,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = http.Post("https://api.telegram.org/bot"+telegramBotToken+"/sendMessage", "application/json", bytes.NewBuffer(jsonMsg))
	return err
}

func sendDiscordAlert(message string) error {
	msg := DiscordMessage{
		Content: message,
	}
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	resp, err := http.Post(discordWebhookURL, "application/json", bytes.NewBuffer(jsonMsg))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
