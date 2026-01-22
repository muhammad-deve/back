package bot

import (
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MandatoryChannel represents a channel users must subscribe to
type MandatoryChannel struct {
	ID         string `json:"id"`
	ChannelID  string `json:"channel_id"`  // @username or chat ID
	ChannelURL string `json:"channel_url"` // t.me/... URL
	Title      string `json:"title"`
	IsActive   bool   `json:"is_active"`
}

// getMandatoryChannels retrieves active mandatory channels from PocketBase
func (b *Bot) getMandatoryChannels() ([]MandatoryChannel, error) {
	collection, err := b.pb.FindCollectionByNameOrId("mandatory_channels")
	if err != nil {
		// If the collection doesn't exist, treat it as "no mandatory channels".
		return []MandatoryChannel{}, nil
	}

	records, err := b.pb.FindRecordsByFilter(collection.Id, "is_active = true", "", 50, 0)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	var channels []MandatoryChannel
	for _, record := range records {
		channels = append(channels, MandatoryChannel{
			ID:         record.Id,
			ChannelID:  record.GetString("channel_id"),
			ChannelURL: record.GetString("channel_url"),
			Title:      record.GetString("title"),
			IsActive:   record.GetBool("is_active"),
		})
	}

	return channels, nil
}

// checkSubscriptions verifies if a user is subscribed to all mandatory channels
func (b *Bot) checkSubscriptions(telegramID int64) (bool, []MandatoryChannel) {
	channels, err := b.getMandatoryChannels()
	if err != nil {
		log.Printf("Error getting mandatory channels: %v", err)
		return true, nil // Allow access if we can't check
	}

	if len(channels) == 0 {
		return true, nil
	}

	var unsubscribed []MandatoryChannel

	for _, channel := range channels {
		subscribed := b.isUserInChannel(telegramID, channel.ChannelID)
		if !subscribed {
			unsubscribed = append(unsubscribed, channel)
		}
	}

	return len(unsubscribed) == 0, unsubscribed
}

// isUserInChannel checks if a user is a member of a channel
func (b *Bot) isUserInChannel(telegramID int64, channelID string) bool {
	config := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: 0,
			UserID: telegramID,
		},
	}

	// Try parsing as username or numeric ID
	if channelID[0] == '@' {
		config.SuperGroupUsername = channelID
	} else {
		var chatID int64
		fmt.Sscanf(channelID, "%d", &chatID)
		config.ChatID = chatID
	}

	member, err := b.api.GetChatMember(config)
	if err != nil {
		log.Printf("Error checking channel membership for %d in %s: %v", telegramID, channelID, err)
		return false
	}

	// Check membership status
	switch member.Status {
	case "creator", "administrator", "member":
		return true
	default:
		return false
	}
}

// showSubscriptionRequired displays the subscription requirement UI
func (b *Bot) showSubscriptionRequired(chatID int64, telegramID int64) {
	channels, err := b.getMandatoryChannels()
	if err != nil {
		log.Printf("Error getting channels: %v", err)
		b.sendMessage(chatID, "❌ An error occurred. Please try again later.")
		return
	}
	if len(channels) == 0 {
		b.showMainMenu(chatID)
		return
	}

	text := `📢 <b>Channel Subscription Required</b>

To use this bot, please subscribe to the following channels:

`

	var rows [][]tgbotapi.InlineKeyboardButton

	for i, channel := range channels {
		// Check subscription status
		subscribed := b.isUserInChannel(telegramID, channel.ChannelID)

		var status string
		if subscribed {
			status = "✅"
		} else {
			status = "❌"
		}

		text += fmt.Sprintf("%d. %s %s\n", i+1, status, channel.Title)

		// Add URL button for channel
		button := tgbotapi.NewInlineKeyboardButtonURL(
			fmt.Sprintf("📢 %s", channel.Title),
			channel.ChannelURL,
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(button))
	}

	text += "\n✅ After subscribing, click the button below:"

	// Add check subscription button
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅ Check Subscription", "check_sub"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

// handleCheckSubscription handles the subscription check callback
func (b *Bot) handleCheckSubscription(chatID int64, telegramID int64) {
	subscribed, unsubscribed := b.checkSubscriptions(telegramID)

	if subscribed {
		b.sendMessage(chatID, "✅ All subscriptions verified!")
		b.showMainMenu(chatID)
		return
	}

	// Show which channels are still missing
	text := "❌ <b>You're still not subscribed to:</b>\n\n"
	for _, ch := range unsubscribed {
		text += fmt.Sprintf("• %s\n", ch.Title)
	}
	text += "\nPlease subscribe and try again."

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, ch := range unsubscribed {
		button := tgbotapi.NewInlineKeyboardButtonURL(
			fmt.Sprintf("📢 %s", ch.Title),
			ch.ChannelURL,
		)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(button))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅ Check Subscription", "check_sub"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}
