package bot

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Channel struct {
	ID        string
	Title     string
	Website   string
	StreamURL string
	Quality   string
	LogoURL   string
	IsWorking bool
}

func (b *Bot) searchChannelsByName(chatID int64, userID int64, query string) {
	q := strings.TrimSpace(query)
	if q == "" {
		b.sendMessage(chatID, "❌ Please enter a channel name to search.")
		return
	}

	b.sendMessage(chatID, "🔍 Searching channels...")

	channels, err := b.findChannelsByName(q)
	if err != nil {
		log.Printf("Error searching channels: %v", err)
		b.sendMessage(chatID, "❌ Failed to search channels.")
		return
	}
	if len(channels) == 0 {
		b.sendMessage(chatID, "❌ No channels found.")
		return
	}

	b.displayChannelSearchResults(chatID, channels)
}

func (b *Bot) handleChannelCategory(chatID int64, userID int64, categoryID string) {
	channels, err := b.findChannelsByCategory(categoryID)
	if err != nil {
		log.Printf("Error finding channels by category: %v", err)
		b.sendMessage(chatID, "❌ Failed to load channels.")
		return
	}
	if len(channels) == 0 {
		b.sendMessage(chatID, "❌ No channels found in this category.")
		return
	}

	b.displayChannelSearchResults(chatID, channels)
}

func (b *Bot) handleChannelSelection(chatID int64, userID int64, channelID string) {
	ch, err := b.findChannelByID(channelID)
	if err != nil || ch == nil {
		b.sendMessage(chatID, "❌ Channel not found.")
		return
	}

	status := "✅ Working"
	if !ch.IsWorking {
		status = "⚠️ Might be offline"
	}

	text := fmt.Sprintf(`📺 <b>%s</b>

%s
📶 Quality: %s`, ch.Title, status, safeText(ch.Quality, "N/A"))

	var rows [][]tgbotapi.InlineKeyboardButton
	if strings.TrimSpace(ch.Website) != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("🌐 Open Website", ch.Website),
		))
	}
	if strings.TrimSpace(ch.StreamURL) != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("▶️ Open Stream", ch.StreamURL),
		))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔍 Search Channels", "channels"),
		tgbotapi.NewInlineKeyboardButtonData("🏠 Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)

	if strings.TrimSpace(ch.LogoURL) != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(ch.LogoURL))
		photo.Caption = text
		photo.ParseMode = "HTML"
		photo.ReplyMarkup = keyboard
		if _, err := b.api.Send(photo); err != nil {
			log.Printf("Failed to send channel photo: %v", err)
			b.sendMessageWithInline(chatID, text, keyboard)
		}
	} else {
		b.sendMessageWithInline(chatID, text, keyboard)
	}

	b.updateWatchHistory(userID, ch.ID, "channel")
}

func (b *Bot) displayChannelSearchResults(chatID int64, channels []*Channel) {
	text := "🔍 <b>Channels:</b>\n\n"
	var rows [][]tgbotapi.InlineKeyboardButton

	for i, ch := range channels {
		if i >= 10 {
			text += fmt.Sprintf("... and %d more", len(channels)-10)
			break
		}

		q := safeText(ch.Quality, "")
		if q != "" {
			q = " • " + q
		}
		text += fmt.Sprintf("%d. <b>%s</b>%s\n", i+1, ch.Title, q)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("%d. %s", i+1, truncateString(ch.Title, 25)), fmt.Sprintf("channel:%s", ch.ID)),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back", "channels"),
		tgbotapi.NewInlineKeyboardButtonData("🏠 Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

func (b *Bot) findChannelByID(id string) (*Channel, error) {
	record, err := b.pb.FindRecordById("channels", id)
	if err != nil {
		return nil, err
	}

	return &Channel{
		ID:        record.Id,
		Title:     record.GetString("title"),
		Website:   record.GetString("wbsite"),
		StreamURL: record.GetString("stream_url"),
		Quality:   record.GetString("quality"),
		LogoURL:   record.GetString("logo_url"),
		IsWorking: record.GetBool("is_url_working"),
	}, nil
}

func (b *Bot) findChannelsByName(query string) ([]*Channel, error) {
	collection, err := b.pb.FindCollectionByNameOrId("channels")
	if err != nil {
		return nil, err
	}

	q := strings.ReplaceAll(strings.TrimSpace(query), "'", "''")
	filter := fmt.Sprintf("title ~ '%s'", q)

	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "-is_url_working,-created", 20, 0)
	if err != nil {
		return nil, err
	}

	out := make([]*Channel, 0, len(records))
	for _, r := range records {
		out = append(out, &Channel{
			ID:        r.Id,
			Title:     r.GetString("title"),
			Website:   r.GetString("wbsite"),
			StreamURL: r.GetString("stream_url"),
			Quality:   r.GetString("quality"),
			LogoURL:   r.GetString("logo_url"),
			IsWorking: r.GetBool("is_url_working"),
		})
	}

	return out, nil
}

func (b *Bot) findChannelsByCategory(categoryID string) ([]*Channel, error) {
	collection, err := b.pb.FindCollectionByNameOrId("channels")
	if err != nil {
		return nil, err
	}

	cid := strings.ReplaceAll(strings.TrimSpace(categoryID), "'", "''")
	filter := fmt.Sprintf("category ~ '%s'", cid)

	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "-is_url_working,-created", 20, 0)
	if err != nil {
		return nil, err
	}

	out := make([]*Channel, 0, len(records))
	for _, r := range records {
		out = append(out, &Channel{
			ID:        r.Id,
			Title:     r.GetString("title"),
			Website:   r.GetString("wbsite"),
			StreamURL: r.GetString("stream_url"),
			Quality:   r.GetString("quality"),
			LogoURL:   r.GetString("logo_url"),
			IsWorking: r.GetBool("is_url_working"),
		})
	}

	return out, nil
}

func safeText(v string, fallback string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return fallback
	}
	return s
}
