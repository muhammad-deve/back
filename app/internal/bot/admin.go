package bot

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pocketbase/pocketbase/core"
)

// handleAdminCommand handles the /admin command
func (b *Bot) handleAdminCommand(chatID int64, userID int64) {
	if !b.isAdmin(userID) {
		b.sendMessage(chatID, "⛔ Unauthorized. This command is only for admins.")
		return
	}

	b.showAdminPanel(chatID)
}

// showAdminPanel displays the admin panel
func (b *Bot) showAdminPanel(chatID int64) {
	text := `🔐 <b>Admin Panel</b>

Select an option below:`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📢 Mandatory Channels", "admin:channels"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📤 Broadcast Message", "admin:broadcast"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("👥 Users Management", "admin:users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Statistics", "admin:stats"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// handleAdminCallback routes admin panel callbacks
func (b *Bot) handleAdminCallback(chatID int64, userID int64, parts []string) {
	if !b.isAdmin(userID) {
		b.sendMessage(chatID, "⛔ Unauthorized")
		return
	}

	if len(parts) == 0 {
		return
	}

	action := parts[0]

	switch action {
	case "channels":
		b.showChannelsManagement(chatID)
	case "add_channel":
		b.promptAddChannel(chatID)
	case "toggle_channel":
		if len(parts) > 1 {
			b.toggleChannel(chatID, parts[1])
		}
	case "delete_channel":
		if len(parts) > 1 {
			b.deleteChannel(chatID, parts[1])
		}
	case "broadcast":
		b.promptBroadcast(chatID)
	case "users":
		b.showUsersManagement(chatID)
	case "export_users":
		b.exportUsers(chatID)
	case "block_user":
		if len(parts) > 1 {
			b.blockUser(chatID, parts[1], true)
		}
	case "unblock_user":
		if len(parts) > 1 {
			b.blockUser(chatID, parts[1], false)
		}
	case "stats":
		b.showStatistics(chatID)
	case "back":
		b.showAdminPanel(chatID)
	}
}

// showChannelsManagement displays mandatory channels management UI
func (b *Bot) showChannelsManagement(chatID int64) {
	channels, err := b.getMandatoryChannels()
	if err != nil {
		b.sendMessage(chatID, "❌ Failed to load channels")
		return
	}

	text := "📢 <b>Mandatory Channels</b>\n\n"

	if len(channels) == 0 {
		text += "<i>No channels configured</i>\n"
	}

	var rows [][]tgbotapi.InlineKeyboardButton

	for i, ch := range channels {
		status := "❌"
		if ch.IsActive {
			status = "✅"
		}

		text += fmt.Sprintf("%d. %s %s\n", i+1, status, ch.Title)

		// Toggle and delete buttons
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("🔄 Toggle: %s", ch.Title),
				fmt.Sprintf("admin:toggle_channel:%s", ch.ID),
			),
			tgbotapi.NewInlineKeyboardButtonData("🗑", fmt.Sprintf("admin:delete_channel:%s", ch.ID)),
		))
	}

	text += "\n📌 To add a new channel, send a message in this format:\n<code>/addchannel @username|title|t.me/link</code>"

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back", "admin:back"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

// promptAddChannel shows instructions for adding a channel
func (b *Bot) promptAddChannel(chatID int64) {
	text := `📢 <b>Add Mandatory Channel</b>

Send a message in this format:
<code>/addchannel @channel_id|Channel Title|https://t.me/channel_link</code>

Example:
<code>/addchannel @mychannel|My Channel|https://t.me/mychannel</code>`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Back", "admin:channels"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// toggleChannel toggles a channel's active status
func (b *Bot) toggleChannel(chatID int64, channelID string) {
	record, err := b.pb.FindRecordById("mandatory_channels", channelID)
	if err != nil {
		b.sendMessage(chatID, "❌ Channel not found")
		return
	}

	isActive := record.GetBool("is_active")
	record.Set("is_active", !isActive)

	if err := b.pb.Save(record); err != nil {
		b.sendMessage(chatID, "❌ Failed to update channel")
		return
	}

	status := "disabled"
	if !isActive {
		status = "enabled"
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ Channel %s", status))
	b.showChannelsManagement(chatID)
}

// deleteChannel removes a mandatory channel
func (b *Bot) deleteChannel(chatID int64, channelID string) {
	record, err := b.pb.FindRecordById("mandatory_channels", channelID)
	if err != nil {
		b.sendMessage(chatID, "❌ Channel not found")
		return
	}

	if err := b.pb.Delete(record); err != nil {
		b.sendMessage(chatID, "❌ Failed to delete channel")
		return
	}

	b.sendMessage(chatID, "✅ Channel deleted")
	b.showChannelsManagement(chatID)
}

// promptBroadcast shows broadcast instructions
func (b *Bot) promptBroadcast(chatID int64) {
	text := `📤 <b>Broadcast Message</b>

Send your message after this command:
<code>/broadcast Your message here</code>

Or forward any message to broadcast it.

⚠️ This will send to ALL registered users.`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Back", "admin:back"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// BroadcastMessage sends a message to all active users
func (b *Bot) BroadcastMessage(adminChatID int64, message string) {
	collection, err := b.pb.FindCollectionByNameOrId("telegram_users")
	if err != nil {
		b.sendMessage(adminChatID, "❌ Failed to get users")
		return
	}

	records, err := b.pb.FindRecordsByFilter(collection.Id, "is_blocked = false", "", 0, 0)
	if err != nil {
		b.sendMessage(adminChatID, "❌ Failed to get users")
		return
	}

	if len(records) == 0 {
		b.sendMessage(adminChatID, "❌ No users to broadcast to")
		return
	}

	b.sendMessage(adminChatID, fmt.Sprintf("📤 Starting broadcast to %d users...", len(records)))

	sent := 0
	failed := 0

	for i, record := range records {
		telegramID := record.GetInt("telegram_id")

		msg := tgbotapi.NewMessage(int64(telegramID), message)
		msg.ParseMode = "HTML"

		_, err := b.api.Send(msg)
		if err != nil {
			failed++
		} else {
			sent++
		}

		// Rate limiting: 50ms delay between messages
		time.Sleep(50 * time.Millisecond)

		// Progress update every 100 users
		if (i+1)%100 == 0 {
			b.sendMessage(adminChatID, fmt.Sprintf("📤 Progress: %d/%d", i+1, len(records)))
		}
	}

	b.sendMessage(adminChatID, fmt.Sprintf("✅ Broadcast complete!\n\n📨 Sent: %d\n❌ Failed: %d", sent, failed))
}

// showUsersManagement displays users management UI
func (b *Bot) showUsersManagement(chatID int64) {
	text := `👥 <b>Users Management</b>

Choose an action:

📌 To get user info: <code>/user telegram_id</code>
📌 To block user: <code>/block telegram_id</code>
📌 To unblock user: <code>/unblock telegram_id</code>
📌 To send DM: <code>/dm telegram_id message</code>`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 Export Users (.xlsx)", "admin:export_users"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Back", "admin:back"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// exportUsers exports all users (simplified - just sends user count and list for now)
func (b *Bot) exportUsers(chatID int64) {
	collection, err := b.pb.FindCollectionByNameOrId("telegram_users")
	if err != nil {
		b.sendMessage(chatID, "❌ Failed to get users")
		return
	}

	records, err := b.pb.FindRecordsByFilter(collection.Id, "", "-created", 0, 0)
	if err != nil {
		b.sendMessage(chatID, "❌ Failed to get users")
		return
	}

	if len(records) == 0 {
		b.sendMessage(chatID, "📊 No users found")
		return
	}

	// For now, just send a summary (proper xlsx export would require additional library)
	text := fmt.Sprintf("👥 <b>Users Export</b>\n\nTotal users: %d\n\n", len(records))

	text += "Recent users:\n"
	for i, record := range records {
		if i >= 20 {
			text += fmt.Sprintf("... and %d more", len(records)-20)
			break
		}

		username := record.GetString("username")
		firstName := record.GetString("first_name")
		telegramID := record.GetInt("telegram_id")
		isBlocked := record.GetBool("is_blocked")

		status := ""
		if isBlocked {
			status = " 🚫"
		}

		name := firstName
		if username != "" {
			name = "@" + username
		}

		text += fmt.Sprintf("%d. %s (%d)%s\n", i+1, name, telegramID, status)
	}

	b.sendMessage(chatID, text)
}

// blockUser blocks or unblocks a user
func (b *Bot) blockUser(chatID int64, telegramIDStr string, block bool) {
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		b.sendMessage(chatID, "❌ Invalid user ID")
		return
	}

	user, err := b.getUser(telegramID)
	if err != nil || user == nil {
		b.sendMessage(chatID, "❌ User not found")
		return
	}

	record, err := b.pb.FindRecordById("telegram_users", user.ID)
	if err != nil {
		b.sendMessage(chatID, "❌ User not found")
		return
	}

	record.Set("is_blocked", block)
	if err := b.pb.Save(record); err != nil {
		b.sendMessage(chatID, "❌ Failed to update user")
		return
	}

	action := "unblocked"
	if block {
		action = "blocked"
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ User %d %s", telegramID, action))
}

// showStatistics displays bot statistics
func (b *Bot) showStatistics(chatID int64) {
	// Count total users
	userCollection, _ := b.pb.FindCollectionByNameOrId("telegram_users")
	users, _ := b.pb.FindRecordsByFilter(userCollection.Id, "", "", 0, 0)
	totalUsers := len(users)

	activeUsers := 0
	blockedUsers := 0
	for _, u := range users {
		if u.GetBool("is_blocked") {
			blockedUsers++
		} else {
			activeUsers++
		}
	}

	// Count watch history
	historyCollection, _ := b.pb.FindCollectionByNameOrId("watch_history")
	history, _ := b.pb.FindRecordsByFilter(historyCollection.Id, "", "", 0, 0)
	totalWatches := len(history)

	// Today's activity
	today := time.Now().Format("2006-01-02")
	todayFilter := fmt.Sprintf("created >= '%s'", today)
	todayHistory, _ := b.pb.FindRecordsByFilter(historyCollection.Id, todayFilter, "", 0, 0)
	todayWatches := len(todayHistory)

	todayUsers, _ := b.pb.FindRecordsByFilter(userCollection.Id, todayFilter, "", 0, 0)
	newUsersToday := len(todayUsers)

	text := fmt.Sprintf(`📊 <b>Bot Statistics</b>

👥 <b>Users:</b>
├ Total: %d
├ Active: %d
└ Blocked: %d

🎬 <b>Content:</b>
├ Total watches: %d
└ Today: %d

📈 <b>Today's Activity:</b>
├ New users: %d
└ Content watched: %d`,
		totalUsers, activeUsers, blockedUsers,
		totalWatches, todayWatches,
		newUsersToday, todayWatches,
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Refresh", "admin:stats"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("◀️ Back", "admin:back"),
		),
	)

	b.sendMessageWithInline(chatID, text, keyboard)
}

// ProcessAdminTextCommand processes admin text commands
func (b *Bot) ProcessAdminTextCommand(chatID int64, userID int64, text string) bool {
	if !b.isAdmin(userID) {
		return false
	}

	parts := strings.SplitN(text, " ", 2)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/addchannel":
		if len(parts) > 1 {
			b.addChannel(chatID, parts[1])
			return true
		}
	case "/broadcast":
		if len(parts) > 1 {
			b.BroadcastMessage(chatID, parts[1])
			return true
		}
	case "/user":
		if len(parts) > 1 {
			b.showUserInfo(chatID, parts[1])
			return true
		}
	case "/block":
		if len(parts) > 1 {
			b.blockUser(chatID, parts[1], true)
			return true
		}
	case "/unblock":
		if len(parts) > 1 {
			b.blockUser(chatID, parts[1], false)
			return true
		}
	case "/dm":
		if len(parts) > 1 {
			b.sendDirectMessage(chatID, parts[1])
			return true
		}
	}

	return false
}

// addChannel adds a new mandatory channel
func (b *Bot) addChannel(chatID int64, data string) {
	parts := strings.Split(data, "|")
	if len(parts) != 3 {
		b.sendMessage(chatID, "❌ Invalid format. Use: @channel_id|Title|https://t.me/link")
		return
	}

	channelID := strings.TrimSpace(parts[0])
	title := strings.TrimSpace(parts[1])
	url := strings.TrimSpace(parts[2])

	collection, err := b.pb.FindCollectionByNameOrId("mandatory_channels")
	if err != nil {
		b.sendMessage(chatID, "❌ Collection not found")
		return
	}

	record := core.NewRecord(collection)
	record.Set("channel_id", channelID)
	record.Set("title", title)
	record.Set("channel_url", url)
	record.Set("is_active", true)

	if err := b.pb.Save(record); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Failed to add channel: %v", err))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ Channel '%s' added successfully!", title))
	b.showChannelsManagement(chatID)
}

// showUserInfo displays information about a user
func (b *Bot) showUserInfo(chatID int64, telegramIDStr string) {
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil {
		b.sendMessage(chatID, "❌ Invalid user ID")
		return
	}

	user, err := b.getUser(telegramID)
	if err != nil || user == nil {
		b.sendMessage(chatID, "❌ User not found")
		return
	}

	status := "✅ Active"
	if user.IsBlocked {
		status = "🚫 Blocked"
	}

	text := fmt.Sprintf(`👤 <b>User Info</b>

🆔 Telegram ID: <code>%d</code>
👤 Username: @%s
📛 Name: %s %s
📱 Phone: %s
📅 Registered: %s
📊 Status: %s`,
		user.TelegramID,
		user.Username,
		user.FirstName,
		user.LastName,
		user.PhoneNumber,
		user.CreatedAt,
		status,
	)

	var rows [][]tgbotapi.InlineKeyboardButton

	if user.IsBlocked {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Unblock", fmt.Sprintf("admin:unblock_user:%d", telegramID)),
		))
	} else {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚫 Block", fmt.Sprintf("admin:block_user:%d", telegramID)),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back", "admin:users"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

// sendDirectMessage sends a direct message to a user
func (b *Bot) sendDirectMessage(adminChatID int64, data string) {
	parts := strings.SplitN(data, " ", 2)
	if len(parts) != 2 {
		b.sendMessage(adminChatID, "❌ Usage: /dm telegram_id message")
		return
	}

	telegramID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		b.sendMessage(adminChatID, "❌ Invalid user ID")
		return
	}

	message := parts[1]

	msg := tgbotapi.NewMessage(telegramID, message)
	msg.ParseMode = "HTML"

	_, err = b.api.Send(msg)
	if err != nil {
		b.sendMessage(adminChatID, fmt.Sprintf("❌ Failed to send: %v", err))
		return
	}

	b.sendMessage(adminChatID, "✅ Message sent successfully!")
}
