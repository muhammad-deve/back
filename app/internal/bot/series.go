package bot

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pocketbase/pocketbase/core"
)

type imdbSeasonInfo struct {
	Season       int
	EpisodeCount int
}

type imdbSeasonsResponse struct {
	Seasons []struct {
		Season       string `json:"season"`
		EpisodeCount int    `json:"episodeCount"`
	} `json:"seasons"`
}

type cachedSeasons struct {
	items     []imdbSeasonInfo
	fetchedAt time.Time
}

var (
	seasonsCacheMu sync.Mutex
	seasonsCache   = map[string]cachedSeasons{}
)

func fetchImdbSeasons(imdbID string) ([]imdbSeasonInfo, error) {
	imdbID = strings.TrimSpace(imdbID)
	if imdbID == "" {
		return nil, fmt.Errorf("missing imdb id")
	}

	url := fmt.Sprintf("https://api.imdbapi.dev/titles/%s/seasons", imdbID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MoviesBot/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("imdb seasons http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var parsed imdbSeasonsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	out := make([]imdbSeasonInfo, 0, len(parsed.Seasons))
	for _, s := range parsed.Seasons {
		seasonNum, err := strconv.Atoi(strings.TrimSpace(s.Season))
		if err != nil {
			continue
		}
		out = append(out, imdbSeasonInfo{Season: seasonNum, EpisodeCount: s.EpisodeCount})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no seasons returned")
	}

	return out, nil
}

func getImdbSeasonsCached(imdbID string) []imdbSeasonInfo {
	imdbID = strings.TrimSpace(imdbID)
	if imdbID == "" {
		return nil
	}

	seasonsCacheMu.Lock()
	defer seasonsCacheMu.Unlock()

	if c, ok := seasonsCache[imdbID]; ok {
		if time.Since(c.fetchedAt) < 6*time.Hour {
			return c.items
		}
	}

	items, err := fetchImdbSeasons(imdbID)
	if err != nil {
		return nil
	}

	seasonsCache[imdbID] = cachedSeasons{items: items, fetchedAt: time.Now()}
	return items
}

// Series represents a TV series (extends Movie type)
type Series struct {
	Movie
	TotalSeasons int `json:"total_seasons"`
}

// Episode represents a TV series episode
type Episode struct {
	Season  int
	Episode int
	Title   string
}

func (b *Bot) displaySeriesSearchResults(chatID int64, series []*Movie) {
	text := "🔍 <b>Series Results:</b>\n\n"

	var rows [][]tgbotapi.InlineKeyboardButton

	for i, s := range series {
		if i >= 10 {
			text += fmt.Sprintf("... and %d more results", len(series)-10)
			break
		}

		y := s.StartYear
		text += fmt.Sprintf("%d. <b>%s</b> (%d) ⭐%.1f\n", i+1, s.Title, y, s.Rating)
		buttonText := fmt.Sprintf("%s ⭐%.1f", truncateString(s.Title, 25), s.Rating)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(buttonText, fmt.Sprintf("series_info:%s", s.ID)),
		))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

// handleSeriesInfo displays series information
func (b *Bot) handleSeriesInfo(chatID int64, userID int64, seriesID string) {
	movie, err := b.findMovieByID(seriesID)
	if err != nil {
		b.sendMessage(chatID, "❌ Series not found.")
		return
	}

	b.displaySeriesInfo(chatID, userID, movie)
}

// displaySeriesInfo shows series details with season selection
func (b *Bot) displaySeriesInfo(chatID int64, userID int64, series *Movie) {
	// Format runtime if available
	var runtime string
	if series.RuntimeSeconds > 0 {
		minutes := series.RuntimeSeconds / 60
		runtime = fmt.Sprintf("%d min/episode", minutes)
	} else {
		runtime = "N/A"
	}

	// Truncate plot if too long
	plot := series.Plot
	if len(plot) > 250 {
		plot = plot[:247] + "..."
	}

	text := fmt.Sprintf(`📺 <b>%s</b> (%d)

⭐ Rating: %.1f/10 (%d votes)
⏱ Episode Duration: %s
📺 Quality: %s

📝 <b>Plot:</b>
%s

Select a season:`,
		series.Title,
		series.StartYear,
		series.Rating,
		series.VoteCount,
		runtime,
		series.Quality,
		plot,
	)

	seasonsInfo := getImdbSeasonsCached(series.ImdbID)
	totalSeasons := 10
	seasonToEpisodes := map[int]int{}
	if len(seasonsInfo) > 0 {
		maxSeason := 0
		for _, s := range seasonsInfo {
			if s.Season > maxSeason {
				maxSeason = s.Season
			}
			if s.Season > 0 && s.EpisodeCount > 0 {
				seasonToEpisodes[s.Season] = s.EpisodeCount
			}
		}
		if maxSeason > 0 {
			totalSeasons = maxSeason
		}
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	for season := 1; season <= totalSeasons; season++ {
		label := fmt.Sprintf("Season %d", season)
		if c, ok := seasonToEpisodes[season]; ok {
			label = fmt.Sprintf("S%d (%d)", season, c)
		}
		button := tgbotapi.NewInlineKeyboardButtonData(
			label,
			fmt.Sprintf("season:%s:%d", series.ID, season),
		)
		row = append(row, button)

		// 3 buttons per row
		if len(row) == 3 || season == totalSeasons {
			rows = append(rows, row)
			row = nil
		}
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)

	// Send poster if available
	if series.PosterURL != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(series.PosterURL))
		photo.Caption = text
		photo.ParseMode = "HTML"
		photo.ReplyMarkup = keyboard
		if _, err := b.api.Send(photo); err != nil {
			log.Printf("Failed to send photo: %v", err)
			b.sendMessageWithInline(chatID, text, keyboard)
		}
	} else {
		b.sendMessageWithInline(chatID, text, keyboard)
	}
}

// handleSeasonSelection displays episodes for a selected season
func (b *Bot) handleSeasonSelection(chatID int64, userID int64, seriesID string, seasonStr string) {
	season, err := strconv.Atoi(seasonStr)
	if err != nil {
		b.sendMessage(chatID, "❌ Invalid season.")
		return
	}

	series, err := b.findMovieByID(seriesID)
	if err != nil {
		b.sendMessage(chatID, "❌ Series not found.")
		return
	}

	b.displayEpisodeList(chatID, userID, series, season)
}

// displayEpisodeList shows episodes for a season
func (b *Bot) displayEpisodeList(chatID int64, userID int64, series *Movie, season int) {
	text := fmt.Sprintf(`📺 <b>%s</b>
📁 Season %d

Select an episode:`, series.Title, season)

	// Default to 20 episodes per season if API fails
	totalEpisodes := 20
	if seasonsInfo := getImdbSeasonsCached(series.ImdbID); len(seasonsInfo) > 0 {
		for _, s := range seasonsInfo {
			if s.Season == season && s.EpisodeCount > 0 {
				totalEpisodes = s.EpisodeCount
				break
			}
		}
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	for ep := 1; ep <= totalEpisodes; ep++ {
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("E%d", ep),
			fmt.Sprintf("episode:%s:%d:%d", series.ID, season, ep),
		)
		row = append(row, button)

		// 5 buttons per row
		if len(row) == 5 || ep == totalEpisodes {
			rows = append(rows, row)
			row = nil
		}
	}

	// Navigation buttons
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Seasons", fmt.Sprintf("series_info:%s", series.ID)),
		tgbotapi.NewInlineKeyboardButtonData("🏠 Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)
}

// handleEpisodeSelection displays episode info and options
func (b *Bot) handleEpisodeSelection(chatID int64, userID int64, seriesID, seasonStr, episodeStr string) {
	season, _ := strconv.Atoi(seasonStr)
	episode, _ := strconv.Atoi(episodeStr)

	series, err := b.findMovieByID(seriesID)
	if err != nil {
		b.sendMessage(chatID, "❌ Series not found.")
		return
	}

	b.displayEpisodeInfo(chatID, userID, series, season, episode)
}

// displayEpisodeInfo shows episode details with watch/download options
func (b *Bot) displayEpisodeInfo(chatID int64, userID int64, series *Movie, season, episode int) {
	text := fmt.Sprintf(`📺 <b>%s</b>
📁 Season %d, Episode %d

Ready to watch?`, series.Title, season, episode)

	var rows [][]tgbotapi.InlineKeyboardButton

	// Build VidSrc URL for episode
	embedURL := fmt.Sprintf("https://vidsrc-embed.ru/embed/%s/%d-%d", series.ImdbID, season, episode)

	// Watch on website button
	websiteURL := fmt.Sprintf("%s/series/%s/%d/%d", websiteBaseURL(), series.ImdbID, season, episode)
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonURL("🌐 Watch on Website", websiteURL),
	))

	// Download button for admins
	if b.isAdmin(userID) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				"📥 Download Video",
				fmt.Sprintf("watch_episode:%s:%d:%d", series.ID, season, episode),
			),
		))
	}

	// Navigation
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("◀️ Back to Episodes", fmt.Sprintf("season:%s:%d", series.ID, season)),
		tgbotapi.NewInlineKeyboardButtonData("🏠 Menu", "main_menu"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendMessageWithInline(chatID, text, keyboard)

	// Avoid unused variable warning
	_ = embedURL
}

// handleWatchEpisode initiates episode download
func (b *Bot) handleWatchEpisode(chatID int64, userID int64, seriesID, seasonStr, episodeStr string) {
	season, _ := strconv.Atoi(seasonStr)
	episode, _ := strconv.Atoi(episodeStr)

	series, err := b.findMovieByID(seriesID)
	if err != nil {
		b.sendMessage(chatID, "❌ Series not found.")
		return
	}

	// Build VidSrc URL for episode
	embedURL := fmt.Sprintf("https://vidsrc-embed.ru/embed/%s/%d-%d", series.ImdbID, season, episode)
	title := fmt.Sprintf("%s S%02dE%02d", series.Title, season, episode)

	statusMsg, err := b.api.Send(tgbotapi.NewMessage(chatID, downloadProgressText(title, DownloadProgress{Percent: 0, ETA: "calculating..."})))
	if err != nil {
		b.sendMessage(chatID, "⏳ Downloading episode... This may take several minutes.")
		statusMsg.MessageID = 0
	}

	// Start download in background
	go func() {
		lastEdit := time.Now()
		lastPercent := -1.0
		videoPath, err := b.downloads.DownloadVideoWithProgress(embedURL, title, func(p DownloadProgress) {
			if statusMsg.MessageID == 0 {
				return
			}
			if time.Since(lastEdit) < 1200*time.Millisecond && (lastPercent >= 0 && p.Percent-lastPercent < 1.0) {
				return
			}
			lastEdit = time.Now()
			lastPercent = p.Percent
			edit := tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, downloadProgressText(title, p))
			_, _ = b.api.Send(edit)
		})
		if err != nil {
			log.Printf("Download failed: %v", err)
			if statusMsg.MessageID != 0 {
				edit := tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, fmt.Sprintf("❌ Download failed: %v", err))
				_, _ = b.api.Send(edit)
				return
			}
			b.sendMessage(chatID, fmt.Sprintf("❌ Download failed: %v", err))
			return
		}

		if statusMsg.MessageID != 0 {
			edit := tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "📤 Uploading to Telegram...")
			_, _ = b.api.Send(edit)
		}

		if err := b.sendVideoFile(chatID, videoPath, title); err != nil {
			log.Printf("Failed to send video: %v", err)
			watchURL := fmt.Sprintf("%s/series/%s/%d/%d", websiteBaseURL(), series.ImdbID, season, episode)
			b.sendMessage(chatID, fmt.Sprintf("❌ Failed to send video.\n\n🌐 Watch online: %s", watchURL))
			return
		}

		if statusMsg.MessageID != 0 {
			_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Sent!"))
		}

		// Update watch history
		b.updateSeriesWatchHistory(userID, series.ID, season, episode)
	}()
}

// updateSeriesWatchHistory records a series episode view
func (b *Bot) updateSeriesWatchHistory(userID int64, seriesID string, season, episode int) {
	collection, err := b.pb.FindCollectionByNameOrId("watch_history")
	if err != nil {
		log.Printf("Watch history collection not found: %v", err)
		return
	}

	record := core.NewRecord(collection)

	user, err := b.getUser(userID)
	if err != nil || user == nil {
		return
	}

	record.Set("user", user.ID)
	record.Set("movie", seriesID)
	// If watch_history table has season/episode fields:
	// record.Set("season", season)
	// record.Set("episode", episode)

	if err := b.pb.Save(record); err != nil {
		log.Printf("Failed to save watch history: %v", err)
	}
}
