package bot

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pocketbase/pocketbase/core"
)

// TelegramUser represents a user stored in PocketBase
type TelegramUser struct {
	ID          string `json:"id"`
	TelegramID  int64  `json:"telegram_id"`
	Username    string `json:"username"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	PhoneNumber string `json:"phone_number"`
	IsBlocked   bool   `json:"is_blocked"`
	CreatedAt   string `json:"created"`
}

// handleStart handles the /start command
func (b *Bot) handleStart(chatID int64, from *tgbotapi.User) {
	userID := from.ID

	// Check if user exists
	user, err := b.getUser(userID)
	if err != nil {
		log.Printf("Error checking user: %v", err)
	}

	if user == nil {
		// New user - request contact
		b.requestContact(chatID, from)
		return
	}

	// Check if user is blocked
	if user.IsBlocked {
		b.sendMessage(chatID, "⛔ Your access has been blocked. Contact support for assistance.")
		return
	}

	// Check channel subscriptions (skip for admins)
	if !b.isAdmin(userID) {
		subscribed, _ := b.checkSubscriptions(userID)
		if !subscribed {
			b.showSubscriptionRequired(chatID, userID)
			return
		}
	}

	// Show main menu
	b.setMode(userID, modeNone)
	b.showMainMenu(chatID)
}

// requestContact asks the user to share their contact
func (b *Bot) requestContact(chatID int64, from *tgbotapi.User) {
	text := fmt.Sprintf(`👋 <b>Welcome, %s!</b>

To use this bot, you need to share your phone number for verification.

📱 Please tap the button below to share your contact:`, from.FirstName)

	// Create contact request keyboard
	contactButton := tgbotapi.NewKeyboardButtonContact("📱 Share Phone Number")
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(contactButton),
	)
	keyboard.OneTimeKeyboard = true
	keyboard.ResizeKeyboard = true

	b.sendMessageWithKeyboard(chatID, text, keyboard)
}

// handleContactShared handles when a user shares their contact
func (b *Bot) handleContactShared(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	from := msg.From
	contact := msg.Contact

	// Verify the contact belongs to the sender
	if contact.UserID != from.ID {
		b.sendMessage(chatID, "⚠️ Please share your own contact, not someone else's.")
		return
	}

	// Create or update user in PocketBase
	user := &TelegramUser{
		TelegramID:  from.ID,
		Username:    from.UserName,
		FirstName:   from.FirstName,
		LastName:    from.LastName,
		PhoneNumber: contact.PhoneNumber,
	}

	if err := b.createOrUpdateUser(user); err != nil {
		log.Printf("Error creating user: %v", err)
		b.sendMessage(chatID, "❌ An error occurred. Please try again later.")
		return
	}

	// Remove custom keyboard
	removeKeyboard := tgbotapi.NewRemoveKeyboard(true)
	msg2 := tgbotapi.NewMessage(chatID, "✅ Registration successful!")
	msg2.ReplyMarkup = removeKeyboard
	b.api.Send(msg2)

	// Check subscriptions
	if !b.isAdmin(from.ID) {
		subscribed, _ := b.checkSubscriptions(from.ID)
		if !subscribed {
			b.showSubscriptionRequired(chatID, from.ID)
			return
		}
	}

	// Show main menu
	b.setMode(from.ID, modeNone)
	b.showMainMenu(chatID)
}

// getUser retrieves a user from PocketBase by Telegram ID
func (b *Bot) getUser(telegramID int64) (*TelegramUser, error) {
	collection, err := b.pb.FindCollectionByNameOrId("telegram_users")
	if err != nil {
		return nil, fmt.Errorf("collection not found: %w", err)
	}

	filter := fmt.Sprintf("telegram_id = %d", telegramID)
	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "", 1, 0)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	if len(records) == 0 {
		return nil, nil
	}

	record := records[0]
	user := &TelegramUser{
		ID:          record.Id,
		TelegramID:  int64(record.GetInt("telegram_id")),
		Username:    record.GetString("username"),
		FirstName:   record.GetString("first_name"),
		LastName:    record.GetString("last_name"),
		PhoneNumber: record.GetString("phone_number"),
		IsBlocked:   record.GetBool("is_blocked"),
		CreatedAt:   record.GetString("created"),
	}

	return user, nil
}

// createOrUpdateUser creates or updates a user in PocketBase
func (b *Bot) createOrUpdateUser(user *TelegramUser) error {
	collection, err := b.pb.FindCollectionByNameOrId("telegram_users")
	if err != nil {
		return fmt.Errorf("collection not found: %w", err)
	}

	// Check if user exists
	filter := fmt.Sprintf("telegram_id = %d", user.TelegramID)
	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "", 1, 0)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	if len(records) > 0 {
		// Update existing user
		record := records[0]
		record.Set("username", user.Username)
		record.Set("first_name", user.FirstName)
		record.Set("last_name", user.LastName)
		record.Set("phone_number", user.PhoneNumber)
		return b.pb.Save(record)
	}

	// Create new user
	record := core.NewRecord(collection)
	record.Set("telegram_id", user.TelegramID)
	record.Set("username", user.Username)
	record.Set("first_name", user.FirstName)
	record.Set("last_name", user.LastName)
	record.Set("phone_number", user.PhoneNumber)
	record.Set("is_blocked", false)

	return b.pb.Save(record)
}

// isAdmin checks if a user is an admin
func (b *Bot) isAdmin(telegramID int64) bool {
	collection, err := b.pb.FindCollectionByNameOrId("admins")
	if err != nil {
		log.Printf("Admins collection not found: %v", err)
		return false
	}

	filter := fmt.Sprintf("telegram_id = %d", telegramID)
	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "", 1, 0)
	if err != nil {
		log.Printf("Error checking admin: %v", err)
		return false
	}

	return len(records) > 0
}

// showMainMenu displays the main menu
func (b *Bot) showMainMenu(chatID int64) {
	text := `🎬 <b>Main Menu</b>

Choose an option below:`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎬 Movies", "movies"),
			tgbotapi.NewInlineKeyboardButtonData("📺 TV Series", "series"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔍 Search TV Channels", "channels"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// showMoviesMenu displays the movies menu
func (b *Bot) showMoviesMenu(chatID int64) {
	text := `🎬 <b>Movies</b>

Send me:
• Movie name (e.g., "Inception")
• IMDb ID (e.g., tt1375666)
• TMDB ID (e.g., 27205)

I'll find and send you the movie!`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// showSeriesMenu displays the TV series menu
func (b *Bot) showSeriesMenu(chatID int64) {
	text := `📺 <b>TV Series</b>

Send me:
• Series name (e.g., "Breaking Bad")
• IMDb ID (e.g., tt0903747)
• TMDB ID (e.g., 1396)

I'll show you available seasons and episodes!`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// showChannelsSearch displays the channels search interface
func (b *Bot) showChannelsSearch(chatID int64) {
	text := `🔍 <b>TV Channels</b>

Send me a channel name to search.

Or browse by category:`

	// Get channel categories from PocketBase
	categories, err := b.getChannelCategories()
	if err != nil {
		log.Printf("Error getting categories: %v", err)
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	for i, cat := range categories {
		button := tgbotapi.NewInlineKeyboardButtonData(cat.Name, fmt.Sprintf("channel_cat:%s", cat.ID))
		row = append(row, button)

		// 2 buttons per row
		if len(row) == 2 || i == len(categories)-1 {
			rows = append(rows, row)
			row = nil
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

// ChannelCategory represents a channel category
type ChannelCategory struct {
	ID   string
	Name string
}

// getChannelCategories retrieves channel categories from PocketBase
func (b *Bot) getChannelCategories() ([]ChannelCategory, error) {
	collection, err := b.pb.FindCollectionByNameOrId("categories")
	if err != nil {
		return nil, err
	}

	records, err := b.pb.FindRecordsByFilter(collection.Id, "", "", 50, 0)
	if err != nil {
		return nil, err
	}

	var categories []ChannelCategory
	for _, record := range records {
		name := strings.TrimSpace(record.GetString("name"))
		if name != "" {
			name = strings.ToUpper(name[:1]) + name[1:]
		}
		categories = append(categories, ChannelCategory{
			ID:   record.Id,
			Name: name,
		})
	}

	return categories, nil
}
