package bot

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pocketbase/pocketbase/core"
)

// Movie represents a movie from PocketBase
type Movie struct {
	ID             string  `json:"id"`
	ImdbID         string  `json:"imdb_id"`
	TmdbID         string  `json:"tmdb_id"`
	Title          string  `json:"title"`
	Plot           string  `json:"plot"`
	Quality        string  `json:"quality"`
	StartYear      int     `json:"start_year"`
	RuntimeSeconds int     `json:"runtime_seconds"`
	Rating         float64 `json:"imdb_rating"`
	VoteCount      int     `json:"vote_count"`
	PosterURL      string  `json:"poster_url"`
	VidsrcURL      string  `json:"vidsrc_url"`
	VidlinkProURL  string  `json:"vidlink_pro_url"`
	AutoembedURL   string  `json:"autoembed_url"`
	GomoURL        string  `json:"gomo_url"`
	MoviesAPIdURL  string  `json:"moviesapi_url"`
	Type           string  `json:"type"` // movie, series, etc.
}

func websiteBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("DOMAIN")); v != "" {
		return strings.TrimRight(v, "/")
	}
	if v := strings.TrimSpace(os.Getenv("WEBSITE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:3000"
}

// searchByIMDbID searches for a movie by IMDb ID
func (b *Bot) searchByIMDbID(chatID int64, userID int64, imdbID string) {
	b.sendMessage(chatID, "🔍 Searching...")

	movie, err := b.findMovieByIMDbID(imdbID)
	if err != nil {
		log.Printf("Error finding movie: %v", err)
		b.sendMessage(chatID, "❌ Movie not found. Please check the ID and try again.")
		return
	}

	if movie == nil || isSeriesType(movie.Type) {
		b.sendMessage(chatID, "❌ No movies found. Try a different search term.")
		return
	}

	b.displayMovieInfo(chatID, userID, movie)
}

// searchByTMDbID searches for a movie by TMDB ID
func (b *Bot) searchByTMDbID(chatID int64, userID int64, tmdbID string) {
	b.sendMessage(chatID, "🔍 Searching...")

	movie, err := b.findMovieByTMDbID(tmdbID)
	if err != nil {
		log.Printf("Error finding movie: %v", err)
		b.sendMessage(chatID, "❌ Movie not found. Please check the ID and try again.")
		return
	}

	if movie == nil || isSeriesType(movie.Type) {
		b.sendMessage(chatID, "❌ No movies found. Try a different search term.")
		return
	}

	b.displayMovieInfo(chatID, userID, movie)
}

// searchByName searches for movies by name
func (b *Bot) searchByName(chatID int64, userID int64, name string) {
	b.sendMessage(chatID, "🔍 Searching...")

	movies, err := b.findMoviesByName(name)
	if err != nil || len(movies) == 0 {
		log.Printf("Error finding movies: %v", err)
		b.sendMessage(chatID, "❌ No movies found. Try a different search term.")
		return
	}

	var moviesOnly []*Movie
	for _, m := range movies {
		if m == nil || isSeriesType(m.Type) {
			continue
		}
		moviesOnly = append(moviesOnly, m)
	}

	if len(moviesOnly) == 0 {
		b.sendMessage(chatID, "❌ No movies found. Try a different search term.")
		return
	}

	movies = moviesOnly

	if len(movies) == 1 {
		b.displayMovieInfo(chatID, userID, movies[0])
		return
	}

	// Display search results
	b.displaySearchResults(chatID, movies)
}

// findMovieByIMDbID finds a movie by IMDb ID in PocketBase
func (b *Bot) findMovieByIMDbID(imdbID string) (*Movie, error) {
	collection, err := b.pb.FindCollectionByNameOrId("movies")
	if err != nil {
		return nil, err
	}

	filter := fmt.Sprintf("imdb_id = '%s'", imdbID)
	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "", 1, 0)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("movie not found")
	}

	return b.recordToMovie(records[0]), nil
}

// findMovieByTMDbID finds a movie by TMDB ID in PocketBase
func (b *Bot) findMovieByTMDbID(tmdbID string) (*Movie, error) {
	collection, err := b.pb.FindCollectionByNameOrId("movies")
	if err != nil {
		return nil, err
	}

	filter := fmt.Sprintf("tmdb_id = '%s'", tmdbID)
	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "", 1, 0)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("movie not found")
	}

	return b.recordToMovie(records[0]), nil
}

// findMoviesByName searches movies by name in PocketBase
func (b *Bot) findMoviesByName(name string) ([]*Movie, error) {
	collection, err := b.pb.FindCollectionByNameOrId("movies")
	if err != nil {
		return nil, err
	}

	// Escape single quotes in name
	escapedName := strings.ReplaceAll(name, "'", "''")
	filter := fmt.Sprintf("title ~ '%s'", escapedName)

	records, err := b.pb.FindRecordsByFilter(collection.Id, filter, "-imdb_rating", 10, 0)
	if err != nil {
		return nil, err
	}

	var movies []*Movie
	for _, record := range records {
		movies = append(movies, b.recordToMovie(record))
	}

	return movies, nil
}

// recordToMovie converts a PocketBase record to a Movie struct
func (b *Bot) recordToMovie(record interface{}) *Movie {
	// Type assertion for PocketBase record
	type RecordGetter interface {
		GetString(key string) string
		GetInt(key string) int
		GetFloat(key string) float64
	}

	r, ok := record.(RecordGetter)
	if !ok {
		return nil
	}

	id := r.GetString("id")
	if strings.TrimSpace(id) == "" {
		v := reflect.ValueOf(record)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if v.IsValid() && v.Kind() == reflect.Struct {
			f := v.FieldByName("Id")
			if f.IsValid() && f.Kind() == reflect.String {
				id = f.String()
			}
		}
	}

	m := &Movie{
		ID:             id,
		ImdbID:         r.GetString("imdb_id"),
		TmdbID:         r.GetString("tmdb_id"),
		Title:          r.GetString("title"),
		Plot:           r.GetString("plot"),
		Quality:        r.GetString("quality"),
		StartYear:      r.GetInt("start_year"),
		RuntimeSeconds: r.GetInt("runtime_seconds"),
		Rating:         r.GetFloat("imdb_rating"),
		VoteCount:      r.GetInt("vote_count"),
		PosterURL:      r.GetString("poster_url"),
		VidsrcURL:      r.GetString("vidsrc_url"),
		VidlinkProURL:  r.GetString("vidlink_pro_url"),
		AutoembedURL:   r.GetString("autoembed_url"),
		GomoURL:        r.GetString("gomo_url"),
		MoviesAPIdURL:  r.GetString("moviesapi_url"),
		Type:           r.GetString("type"),
	}

	// Align with current PocketBase schema
	// - released_year: number
	// - duration: number (seconds)
	if y := r.GetInt("released_year"); y > 0 {
		m.StartYear = y
	}
	if d := r.GetInt("duration"); d > 0 {
		m.RuntimeSeconds = d
	}

	// posters and video urls are stored in the related "contents" record referenced by content_id
	contentID := strings.TrimSpace(r.GetString("content_id"))
	if contentID != "" {
		if contentRec, err := b.pb.FindRecordById("contents", contentID); err == nil && contentRec != nil {
			if v := strings.TrimSpace(contentRec.GetString("poster_url")); v != "" {
				m.PosterURL = v
			}
			if v := strings.TrimSpace(contentRec.GetString("vidsrc_url")); v != "" {
				m.VidsrcURL = v
			}
			if v := strings.TrimSpace(contentRec.GetString("vidlink_url")); v != "" {
				// keep existing field name for backward compatibility
				m.VidlinkProURL = v
			}
			if v := strings.TrimSpace(contentRec.GetString("autoembed_url")); v != "" {
				m.AutoembedURL = v
			}
			if v := strings.TrimSpace(contentRec.GetString("gomo_url")); v != "" {
				m.GomoURL = v
			}
			if v := strings.TrimSpace(contentRec.GetString("moviesapi_url")); v != "" {
				m.MoviesAPIdURL = v
			}
		}
	}

	// fallback aliases
	if strings.TrimSpace(m.VidlinkProURL) == "" {
		m.VidlinkProURL = r.GetString("vidlink_url")
	}

	return m
}

func movieBestEmbedURL(movie *Movie) string {
	if movie == nil {
		return ""
	}
	if strings.TrimSpace(movie.AutoembedURL) != "" {
		return movie.AutoembedURL
	}
	if strings.TrimSpace(movie.VidlinkProURL) != "" {
		return movie.VidlinkProURL
	}
	if strings.TrimSpace(movie.VidsrcURL) != "" {
		return movie.VidsrcURL
	}
	return ""
}

// displayMovieInfo displays movie information with inline buttons
func (b *Bot) displayMovieInfo(chatID int64, userID int64, movie *Movie) {
	plot := strings.TrimSpace(movie.Plot)
	yearText := ""
	if movie.StartYear > 0 {
		yearText = fmt.Sprintf(" (%d)", movie.StartYear)
	}

	runtime := formatRuntimeSeconds(movie.RuntimeSeconds)
	qualityLine := ""
	if q := strings.TrimSpace(movie.Quality); q != "" {
		qualityLine = fmt.Sprintf("\n📺 Quality: %s", q)
	}

	prefix := fmt.Sprintf(`🎬 <b>%s</b>%s

⭐ Rating: %.1f/10
⏱ Duration: %s
%s

📝 <b>Description:</b>
`,
		movie.Title,
		yearText,
		movie.Rating,
		runtime,
		qualityLine,
	)

	captionMax := 950
	remaining := captionMax - runeLen(prefix)
	if remaining < 0 {
		remaining = 0
	}
	plot = truncateRunes(plot, remaining)
	caption := prefix + plot

	// Build inline keyboard
	var rows [][]tgbotapi.InlineKeyboardButton

	// Watch button - all users get website link
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonURL("🌐 Watch online", fmt.Sprintf("%s/movie/%s", websiteBaseURL(), movie.ImdbID)),
	))

	if b.isAdmin(userID) {
		if u := movieBestEmbedURL(movie); u != "" {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonURL("🚫 Watch without ads", u),
			))
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)

	// Send poster if available
	if movie.PosterURL != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(movie.PosterURL))
		photo.Caption = caption
		photo.ParseMode = "HTML"
		photo.ReplyMarkup = keyboard
		if _, err := b.api.Send(photo); err != nil {
			// Fallback to text only
			log.Printf("Failed to send photo: %v", err)
			b.sendMessageWithInline(chatID, caption, keyboard)
			return
		}
	} else {
		b.sendMessageWithInline(chatID, caption, keyboard)
	}

	// Update watch history
	b.updateWatchHistory(userID, movie.ID, "movie")
}

// displaySearchResults displays search results with selection buttons
func (b *Bot) displaySearchResults(chatID int64, movies []*Movie) {
	text := "🔍 <b>Search Results:</b>\n\n"

	var rows [][]tgbotapi.InlineKeyboardButton

	for i, movie := range movies {
		if i >= 8 {
			text += fmt.Sprintf("... and %d more results", len(movies)-8)
			break
		}

		text += fmt.Sprintf("%d. <b>%s</b> ⭐%.1f\n", i+1, movie.Title, movie.Rating)

		// Add selection button
		buttonText := fmt.Sprintf("%s ⭐%.1f", truncateString(movie.Title, 25), movie.Rating)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(buttonText, fmt.Sprintf("movie:%s", movie.ID)),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

// handleMovieSelection handles when a user selects a movie from search results
func (b *Bot) handleMovieSelection(chatID int64, userID int64, movieID string) {
	movie, err := b.findMovieByID(movieID)
	if err != nil {
		log.Printf("Error finding movie: %v", err)
		b.sendMessage(chatID, "❌ Movie not found.")
		return
	}

	b.displayMovieInfo(chatID, userID, movie)
}

// findMovieByID finds a movie by its PocketBase ID
func (b *Bot) findMovieByID(id string) (*Movie, error) {
	record, err := b.pb.FindRecordById("movies", id)
	if err != nil {
		return nil, err
	}

	return b.recordToMovie(record), nil
}

// handleWatchMovie initiates the video download and send process
func (b *Bot) handleWatchMovie(chatID int64, userID int64, movieID string) {
	movie, err := b.findMovieByID(movieID)
	if err != nil {
		b.sendMessage(chatID, "❌ Movie not found.")
		return
	}

	watchURL := fmt.Sprintf("%s/movie/%s", websiteBaseURL(), movie.ImdbID)
	msg := fmt.Sprintf("🌐 Watch online: %s", watchURL)
	if b.isAdmin(userID) {
		if u := movieBestEmbedURL(movie); u != "" {
			msg = fmt.Sprintf("%s\n� Watch without ads: %s", msg, u)
		}
	}
	b.sendMessage(chatID, msg)
}

// sendVideoFile sends a video file to the user
func (b *Bot) sendVideoFile(chatID int64, filePath string, title string) error {
	if maxBytes := telegramUploadMaxBytes(); maxBytes > 0 {
		if info, err := os.Stat(filePath); err == nil {
			if info.Size() > maxBytes {
				mb := float64(info.Size()) / 1024 / 1024
				return fmt.Errorf("file too large for Telegram upload (%.1f MB)", mb)
			}
		}
	}

	video := tgbotapi.NewVideo(chatID, tgbotapi.FilePath(filePath))
	video.Caption = fmt.Sprintf("🎬 %s", title)
	video.SupportsStreaming = true

	if _, err := b.api.Send(video); err == nil {
		return nil
	}

	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
	doc.Caption = fmt.Sprintf("🎬 %s", title)
	_, err := b.api.Send(doc)
	return err
}

func telegramUploadMaxBytes() int64 {
	// If set, allows you to pre-check and skip uploads that will likely fail.
	// If not set, we won't block uploads and will let Telegram decide.
	if v := strings.TrimSpace(os.Getenv("TELEGRAM_UPLOAD_MAX_MB")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n * 1024 * 1024
		}
	}
	return 0
}

// updateWatchHistory records a movie view in watch history
func (b *Bot) updateWatchHistory(userID int64, contentID string, contentType string) {
	collection, err := b.pb.FindCollectionByNameOrId("watch_history")
	if err != nil {
		log.Printf("Watch history collection not found: %v", err)
		return
	}

	record := core.NewRecord(collection)

	// Get user record ID
	user, err := b.getUser(userID)
	if err != nil || user == nil {
		return
	}

	record.Set("user", user.ID)
	if contentType == "movie" {
		record.Set("movie", contentID)
	} else {
		record.Set("channel", contentID)
	}

	if err := b.pb.Save(record); err != nil {
		log.Printf("Failed to save watch history: %v", err)
	}
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatRuntimeSeconds(seconds int) string {
	if seconds <= 0 {
		return "N/A"
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "N/A"
}

func runeLen(s string) int {
	return len([]rune(s))
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
