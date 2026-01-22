package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pocketbase/pocketbase"
)

// Bot represents the Telegram bot instance
type Bot struct {
	api         *tgbotapi.BotAPI
	pb          *pocketbase.PocketBase
	rateLimiter *RateLimiter
	downloads   *DownloadManager
	stateMu     sync.RWMutex
	userMode    map[int64]string
	adminMode   map[int64]string
	pendingBC   map[int64]*pendingBroadcast
	ctx         context.Context
	cancel      context.CancelFunc
}

type pendingBroadcast struct {
	FromChatID int64
	MessageID  int
}

const (
	modeNone     = ""
	modeMovies   = "movies"
	modeSeries   = "series"
	modeChannels = "channels"

	adminModeNone           = ""
	adminModeAwaitBroadcast = "await_broadcast"
)

const (
	adminMenuMandatoryChannelsButton = "📢 Mandatory Channels"
	adminMenuBroadcastButton         = "📤 Broadcast Message"
	adminMenuUsersButton             = "👥 Users Management"
	adminMenuStatsButton             = "📊 Statistics"
	adminMenuBackToMenuButton        = "◀️ Back to Menu"
)

func (b *Bot) setMode(userID int64, mode string) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.userMode == nil {
		b.userMode = make(map[int64]string)
	}
	b.userMode[userID] = mode
}

func (b *Bot) getMode(userID int64) string {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	return b.userMode[userID]
}

func (b *Bot) setAdminMode(userID int64, mode string) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.adminMode == nil {
		b.adminMode = make(map[int64]string)
	}
	b.adminMode[userID] = mode
}

func (b *Bot) getAdminMode(userID int64) string {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	if b.adminMode == nil {
		return ""
	}
	return b.adminMode[userID]
}

func (b *Bot) setPendingBroadcast(userID int64, fromChatID int64, messageID int) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.pendingBC == nil {
		b.pendingBC = make(map[int64]*pendingBroadcast)
	}
	b.pendingBC[userID] = &pendingBroadcast{FromChatID: fromChatID, MessageID: messageID}
}

func (b *Bot) clearPendingBroadcast(userID int64) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.pendingBC != nil {
		delete(b.pendingBC, userID)
	}
}

func (b *Bot) getPendingBroadcast(userID int64) *pendingBroadcast {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	if b.pendingBC == nil {
		return nil
	}
	return b.pendingBC[userID]
}

// RateLimiter implements per-user rate limiting
type RateLimiter struct {
	mu       sync.RWMutex
	requests map[int64][]time.Time
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[int64][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if a user can make a request
func (r *RateLimiter) Allow(userID int64) bool {
	if r == nil || r.limit <= 0 {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Clean old requests
	if times, exists := r.requests[userID]; exists {
		filtered := times[:0]
		for _, t := range times {
			if t.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		r.requests[userID] = filtered
	}

	// Check limit
	if len(r.requests[userID]) >= r.limit {
		return false
	}

	// Add new request
	r.requests[userID] = append(r.requests[userID], now)
	return true
}

// NewBot creates a new bot instance
func NewBot(pb *pocketbase.PocketBase) (*Bot, error) {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("BOT_TOKEN environment variable not set")
	}

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	bot := &Bot{
		api:         api,
		pb:          pb,
		rateLimiter: NewRateLimiter(0, time.Hour),
		downloads:   NewDownloadManager("./movies"),
		userMode:    make(map[int64]string),
		ctx:         ctx,
		cancel:      cancel,
	}

	log.Printf("🤖 Bot authorized on account %s", api.Self.UserName)

	return bot, nil
}

// Start begins processing updates
func (b *Bot) Start() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Println("📡 Bot started, waiting for updates...")
	for update := range updates {
		go b.handleUpdate(update)
	}
	return nil
}

// Stop gracefully shuts down the bot
func (b *Bot) Stop() {
	// Stop receiving updates so Start() can return.
	b.api.StopReceivingUpdates()
	b.cancel()
}

// handleUpdate processes a single update
func (b *Bot) handleUpdate(update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ Panic in update handler: %v", r)
		}
	}()

	if update.Message != nil {
		b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		b.handleCallback(update.CallbackQuery)
	}
}

// handleMessage processes incoming messages
func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	userID := msg.From.ID
	chatID := msg.Chat.ID

	log.Printf("📨 Message from %s (%d): %s", msg.From.UserName, userID, msg.Text)

	// Handle contact shared
	if msg.Contact != nil {
		b.handleContactShared(msg)
		return
	}

	// Handle commands
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			b.handleStart(chatID, msg.From)
		case "admin":
			b.handleAdminCommand(chatID, userID)
		case "help":
			b.sendHelp(chatID)
		default:
			if b.ProcessAdminTextCommand(chatID, userID, msg.Text) {
				return
			}
			b.sendMessage(chatID, "❓ Unknown command. Use /start to begin.")
		}
		return
	}

	// Check if user is registered
	user, err := b.getUser(userID)
	if err != nil || user == nil {
		b.handleStart(chatID, msg.From)
		return
	}

	// Check subscription before processing any request
	if !b.isAdmin(userID) {
		subscribed, _ := b.checkSubscriptions(userID)
		if !subscribed {
			b.showSubscriptionRequired(chatID, userID)
			return
		}
	}

	// Admin broadcast flow: capture ANY message type
	if b.isAdmin(userID) && b.getAdminMode(userID) == adminModeAwaitBroadcast {
		b.setPendingBroadcast(userID, chatID, msg.MessageID)
		b.setAdminMode(userID, adminModeNone)
		// show preview by copying back to admin (best-effort)
		copyCfg := tgbotapi.NewCopyMessage(chatID, chatID, msg.MessageID)
		_, _ = b.api.Request(copyCfg)
		b.showBroadcastConfirm(chatID)
		return
	}

	// Handle text messages based on context
	text := strings.TrimSpace(msg.Text)
	if text != "" {
		if b.ProcessAdminTextCommand(chatID, userID, text) {
			return
		}
		b.handleTextInput(chatID, userID, text)
	}
}

// handleCallback processes callback queries (inline button presses)
func (b *Bot) handleCallback(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID
	chatID := callback.Message.Chat.ID
	data := callback.Data

	log.Printf("🔘 Callback from %s (%d): %s", callback.From.UserName, userID, data)

	// Answer callback to remove loading state
	ack := tgbotapi.NewCallback(callback.ID, "")
	b.api.Request(ack)

	// Check registration
	user, err := b.getUser(userID)
	if err != nil || user == nil {
		b.handleStart(chatID, callback.From)
		return
	}

	// Check subscription (except for check_subscription callback)
	if !strings.HasPrefix(data, "check_sub") && !b.isAdmin(userID) {
		subscribed, _ := b.checkSubscriptions(userID)
		if !subscribed {
			b.showSubscriptionRequired(chatID, userID)
			return
		}
	}

	// Route callback
	parts := strings.Split(data, ":")
	action := parts[0]

	switch action {
	case "check_sub":
		b.handleCheckSubscription(chatID, userID)
	case "main_menu":
		b.setMode(userID, modeNone)
		b.showMainMenu(chatID)
	case "movies":
		b.setMode(userID, modeMovies)
		b.showMoviesMenu(chatID)
	case "series":
		b.setMode(userID, modeSeries)
		b.showSeriesMenu(chatID)
	case "channels":
		b.setMode(userID, modeChannels)
		b.showChannelsSearch(chatID)
	case "channel_cat":
		if len(parts) > 1 {
			b.handleChannelCategory(chatID, userID, parts[1])
		}
	case "channel":
		if len(parts) > 1 {
			b.handleChannelSelection(chatID, userID, parts[1])
		}
	case "movie":
		if len(parts) > 1 {
			b.handleMovieSelection(chatID, userID, parts[1])
		}
	case "watch_movie":
		if len(parts) > 1 {
			b.handleWatchMovie(chatID, userID, parts[1])
		}
	case "series_info":
		if len(parts) > 1 {
			b.handleSeriesInfo(chatID, userID, parts[1])
		}
	case "season":
		if len(parts) > 2 {
			b.handleSeasonSelection(chatID, userID, parts[1], parts[2])
		}
	case "episode":
		if len(parts) > 3 {
			b.handleEpisodeSelection(chatID, userID, parts[1], parts[2], parts[3])
		}
	case "watch_episode":
		if len(parts) > 3 {
			b.handleWatchEpisode(chatID, userID, parts[1], parts[2], parts[3])
		}
	// Admin callbacks
	case "admin":
		if len(parts) > 1 {
			b.handleAdminCallback(chatID, userID, parts[1:])
		}
	default:
		b.sendMessage(chatID, "❓ Unknown action.")
	}
}

// sendMessage sends a text message
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	}
}

// sendMessageWithKeyboard sends a message with a reply keyboard
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboard
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	}
}

// sendMessageWithInline sends a message with inline keyboard
func (b *Bot) sendMessageWithInline(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = keyboard
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("❌ Failed to send message: %v", err)
	}
}

// editMessageWithInline edits an existing message with new inline keyboard
func (b *Bot) editMessageWithInline(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = &keyboard
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("❌ Failed to edit message: %v", err)
	}
}

// sendHelp sends the help message
func (b *Bot) sendHelp(chatID int64) {
	text := `🎬 <b>Movie & TV Series Bot</b>

📌 <b>Commands:</b>
/start - Start the bot
/help - Show this help

📌 <b>How to use:</b>
1. Subscribe to required channels
2. Use menu buttons to browse
3. Search movies by name or IMDb ID
4. Select quality and download

📌 <b>Supported formats:</b>
• IMDb ID: tt1234567
• TMDB ID: 12345
• Movie/Series name

Enjoy watching! 🍿`

	b.sendMessage(chatID, text)
}

// handleTextInput processes text input from users
func (b *Bot) handleTextInput(chatID int64, userID int64, text string) {
	mode := b.getMode(userID)

	// Global reply-keyboard actions (work regardless of current mode)
	switch text {
	case backToMenuButton:
		b.setMode(userID, modeNone)
		b.showMainMenu(chatID)
		return
	case mainMenuMoviesButton:
		b.setMode(userID, modeMovies)
		b.showMoviesMenu(chatID)
		return
	case mainMenuSeriesButton:
		b.setMode(userID, modeSeries)
		b.showSeriesMenu(chatID)
		return
	case mainMenuChannelsButton:
		b.setMode(userID, modeChannels)
		b.showChannelsSearch(chatID)
		return
	}

	// Admin reply-keyboard actions
	if b.isAdmin(userID) {
		switch text {
		case adminMenuMandatoryChannelsButton:
			b.showChannelsManagement(chatID)
			return
		case adminMenuBroadcastButton:
			b.promptBroadcast(chatID, userID)
			return
		case adminMenuUsersButton:
			b.showUsersManagement(chatID)
			return
		case adminMenuStatsButton:
			b.showStatistics(chatID)
			return
		case adminMenuBackToMenuButton:
			b.setMode(userID, modeNone)
			b.showMainMenu(chatID)
			return
		}
	}

	switch mode {
	case modeChannels:
		b.searchChannelsByName(chatID, userID, text)
		return
	case modeSeries:
		b.searchSeries(chatID, userID, text)
		return
	case modeMovies:
		b.searchMovies(chatID, userID, text)
		return
	default:
		b.searchMovies(chatID, userID, text)
		return
	}
}

func (b *Bot) searchMovies(chatID int64, userID int64, text string) {
	if strings.HasPrefix(strings.ToLower(text), "tt") {
		b.searchByIMDbID(chatID, userID, text)
		return
	}
	if isNumericString(text) {
		b.searchByTMDbID(chatID, userID, text)
		return
	}
	b.searchByName(chatID, userID, text)
}

func (b *Bot) searchSeries(chatID int64, userID int64, text string) {
	if strings.HasPrefix(strings.ToLower(text), "tt") {
		movie, err := b.findMovieByIMDbID(text)
		if err != nil || movie == nil || !isSeriesType(movie.Type) {
			b.sendMessage(chatID, "❌ Series not found. Please check the ID and try again.")
			return
		}
		b.displaySeriesInfo(chatID, userID, movie)
		return
	}
	if isNumericString(text) {
		movie, err := b.findMovieByTMDbID(text)
		if err != nil || movie == nil || !isSeriesType(movie.Type) {
			b.sendMessage(chatID, "❌ Series not found. Please check the ID and try again.")
			return
		}
		b.displaySeriesInfo(chatID, userID, movie)
		return
	}

	movies, err := b.findMoviesByName(text)
	if err != nil || len(movies) == 0 {
		b.sendMessage(chatID, "❌ No series found. Try a different search term.")
		return
	}

	series := make([]*Movie, 0, len(movies))
	for _, m := range movies {
		if m != nil && isSeriesType(m.Type) {
			series = append(series, m)
		}
	}
	if len(series) == 0 {
		b.sendMessage(chatID, "❌ No series found. Try a different search term.")
		return
	}
	if len(series) == 1 {
		b.displaySeriesInfo(chatID, userID, series[0])
		return
	}

	b.displaySeriesSearchResults(chatID, series)
}

func isSeriesType(t string) bool {
	s := strings.ToLower(strings.TrimSpace(t))
	return s == "serie" || s == "series" || s == "tv" || s == "tv_series" || s == "tvseries"
}

// isNumericString checks if a string contains only digits
func isNumericString(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func (b *Bot) broadcastPending(adminUserID int64) error {
	p := b.getPendingBroadcast(adminUserID)
	if p == nil {
		return fmt.Errorf("no pending broadcast")
	}

	collection, err := b.pb.FindCollectionByNameOrId("telegram_users")
	if err != nil {
		return err
	}

	records, err := b.pb.FindRecordsByFilter(collection.Id, "is_blocked = false", "", 0, 0)
	if err != nil {
		return err
	}

	for _, r := range records {
		telegramIDStr := strings.TrimSpace(r.GetString("telegram_id"))
		if telegramIDStr == "" {
			continue
		}
		telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
		if err != nil || telegramID == 0 {
			continue
		}

		copyCfg := tgbotapi.NewCopyMessage(telegramID, p.FromChatID, p.MessageID)
		_, _ = b.api.Request(copyCfg)

		// small delay to avoid hitting Telegram rate limits
		time.Sleep(50 * time.Millisecond)
	}

	return nil
}

// GetAPI returns the bot API (for external use)
func (b *Bot) GetAPI() *tgbotapi.BotAPI {
	return b.api
}
