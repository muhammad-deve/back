package bot

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (b *Bot) handleNoAdsMovie(chatID int64, userID int64, movieID string) {
	if !b.isAdmin(userID) {
		b.sendMessage(chatID, "⛔ Unauthorized")
		return
	}

	movie, err := b.findMovieByID(movieID)
	if err != nil || movie == nil {
		b.sendMessage(chatID, "❌ Movie not found.")
		return
	}

	embedURL := movieBestEmbedURL(movie)
	if embedURL == "" {
		b.sendMessage(chatID, "❌ No video source available.")
		return
	}

	title := movie.Title
	cacheKey := strings.TrimSpace(movie.ImdbID)
	if cacheKey == "" {
		cacheKey = movie.ID
	}
	cachedPath := filepath.Join(b.downloads.OutputDir(), fmt.Sprintf("%s.mp4", sanitizeFilename(cacheKey)))
	tempPath := cachedPath + ".part"
	statusMsg, err := b.api.Send(tgbotapi.NewMessage(chatID, downloadProgressText(title, DownloadProgress{Percent: 0, ETA: "starting..."})))
	if err != nil {
		statusMsg.MessageID = 0
	}

	go func() {
		if st, err := os.Stat(cachedPath); err == nil && st.Size() > 0 {
			if statusMsg.MessageID != 0 {
				_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Already downloaded. Preparing stream..."))
			}
			id, err := b.stream.CreateStreamFromMP4Cached(cacheKey, cachedPath)
			if err != nil {
				log.Printf("HLS convert failed: %v", err)
				b.sendMessage(chatID, fmt.Sprintf("❌ Stream conversion failed: %v", err))
				return
			}
			url := fmt.Sprintf("%s/%s", b.stream.BaseURL(), id)
			b.sendMessage(chatID, fmt.Sprintf("✅ Stream ready: %s", url))
			if statusMsg.MessageID != 0 {
				_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Stream ready!"))
			}
			return
		}

		lastEdit := time.Now()
		lastPercent := -1.0
		_ = os.Remove(tempPath)

		videoPath, err := b.downloads.DownloadVideoWithProgressToFile(embedURL, title, tempPath, func(p DownloadProgress) {
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
			if strings.Contains(strings.ToLower(err.Error()), "already in progress") {
				if st, stErr := os.Stat(cachedPath); stErr == nil && st.Size() > 0 {
					id, convErr := b.stream.CreateStreamFromMP4Cached(cacheKey, cachedPath)
					if convErr == nil {
						url := fmt.Sprintf("%s/%s", b.stream.BaseURL(), id)
						b.sendMessage(chatID, fmt.Sprintf("✅ Stream ready: %s", url))
						if statusMsg.MessageID != 0 {
							_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Stream ready!"))
						}
						return
					}
				}
				b.sendMessage(chatID, "⏳ Download already in progress. Please try again in a minute.")
				return
			}
			log.Printf("No-ads download failed: %v", err)
			if statusMsg.MessageID != 0 {
				_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, fmt.Sprintf("❌ Download failed: %v", err)))
				return
			}
			b.sendMessage(chatID, fmt.Sprintf("❌ Download failed: %v", err))
			return
		}

		_ = os.Remove(cachedPath)
		if err := os.Rename(videoPath, cachedPath); err == nil {
			videoPath = cachedPath
		}

		if statusMsg.MessageID != 0 {
			_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "⏳ Converting to stream..."))
		}

		id, err := b.stream.CreateStreamFromMP4Cached(cacheKey, videoPath)
		if err != nil {
			log.Printf("HLS convert failed: %v", err)
			b.sendMessage(chatID, fmt.Sprintf("❌ Stream conversion failed: %v", err))
			return
		}

		url := fmt.Sprintf("%s/%s", b.stream.BaseURL(), id)
		b.sendMessage(chatID, fmt.Sprintf("✅ Stream ready: %s", url))
		if statusMsg.MessageID != 0 {
			_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Stream ready!"))
		}
	}()
}

func (b *Bot) handleNoAdsEpisode(chatID int64, userID int64, seriesID, seasonStr, episodeStr string) {
	if !b.isAdmin(userID) {
		b.sendMessage(chatID, "⛔ Unauthorized")
		return
	}

	season, _ := strconv.Atoi(seasonStr)
	episode, _ := strconv.Atoi(episodeStr)

	series, err := b.findMovieByID(seriesID)
	if err != nil || series == nil {
		b.sendMessage(chatID, "❌ Series not found.")
		return
	}

	embedURL := fmt.Sprintf("https://vidsrc-embed.ru/embed/%s/%d-%d", series.ImdbID, season, episode)
	title := fmt.Sprintf("%s S%02dE%02d", series.Title, season, episode)
	cacheKey := fmt.Sprintf("%s_S%02dE%02d", strings.TrimSpace(series.ImdbID), season, episode)
	if strings.TrimSpace(series.ImdbID) == "" {
		cacheKey = fmt.Sprintf("%s_S%02dE%02d", series.ID, season, episode)
	}
	cachedPath := filepath.Join(b.downloads.OutputDir(), fmt.Sprintf("%s.mp4", sanitizeFilename(cacheKey)))
	tempPath := cachedPath + ".part"

	statusMsg, err := b.api.Send(tgbotapi.NewMessage(chatID, downloadProgressText(title, DownloadProgress{Percent: 0, ETA: "starting..."})))
	if err != nil {
		statusMsg.MessageID = 0
	}

	go func() {
		if st, err := os.Stat(cachedPath); err == nil && st.Size() > 0 {
			if statusMsg.MessageID != 0 {
				_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Already downloaded. Preparing stream..."))
			}
			id, err := b.stream.CreateStreamFromMP4Cached(cacheKey, cachedPath)
			if err != nil {
				log.Printf("HLS convert failed: %v", err)
				b.sendMessage(chatID, fmt.Sprintf("❌ Stream conversion failed: %v", err))
				return
			}
			url := fmt.Sprintf("%s/%s", b.stream.BaseURL(), id)
			b.sendMessage(chatID, fmt.Sprintf("✅ Stream ready: %s", url))
			if statusMsg.MessageID != 0 {
				_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Stream ready!"))
			}
			return
		}

		lastEdit := time.Now()
		lastPercent := -1.0
		_ = os.Remove(tempPath)

		videoPath, err := b.downloads.DownloadVideoWithProgressToFile(embedURL, title, tempPath, func(p DownloadProgress) {
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
			if strings.Contains(strings.ToLower(err.Error()), "already in progress") {
				if st, stErr := os.Stat(cachedPath); stErr == nil && st.Size() > 0 {
					id, convErr := b.stream.CreateStreamFromMP4Cached(cacheKey, cachedPath)
					if convErr == nil {
						url := fmt.Sprintf("%s/%s", b.stream.BaseURL(), id)
						b.sendMessage(chatID, fmt.Sprintf("✅ Stream ready: %s", url))
						if statusMsg.MessageID != 0 {
							_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Stream ready!"))
						}
						return
					}
				}
				b.sendMessage(chatID, "⏳ Download already in progress. Please try again in a minute.")
				return
			}
			log.Printf("No-ads episode download failed: %v", err)
			if statusMsg.MessageID != 0 {
				_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, fmt.Sprintf("❌ Download failed: %v", err)))
				return
			}
			b.sendMessage(chatID, fmt.Sprintf("❌ Download failed: %v", err))
			return
		}

		_ = os.Remove(cachedPath)
		if err := os.Rename(videoPath, cachedPath); err == nil {
			videoPath = cachedPath
		}

		if statusMsg.MessageID != 0 {
			_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "⏳ Converting to stream..."))
		}

		id, err := b.stream.CreateStreamFromMP4Cached(cacheKey, videoPath)
		if err != nil {
			log.Printf("HLS convert failed: %v", err)
			b.sendMessage(chatID, fmt.Sprintf("❌ Stream conversion failed: %v", err))
			return
		}

		url := fmt.Sprintf("%s/%s", b.stream.BaseURL(), id)
		b.sendMessage(chatID, fmt.Sprintf("✅ Stream ready: %s", url))
		if statusMsg.MessageID != 0 {
			_, _ = b.api.Send(tgbotapi.NewEditMessageText(chatID, statusMsg.MessageID, "✅ Stream ready!"))
		}
	}()
}
