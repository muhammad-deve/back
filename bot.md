# Telegram Movie & TV Series Bot - Requirements

## Tech Stack
- Language: Go
- Environment: BOT_TOKEN, GEMINI_API_KEY (.env file)

## Core Features

### User Registration & Verification
- `/start` command checks if user exists in PocketBase
- New users must share Telegram contact (mandatory, block access if refused)
- Store: all necessary user data to USERS table

### Mandatory Channel Subscription
- After registration, check subscription to all active channels from PocketBase (collection: mandatory_channels)
- Display channels as inline URL buttons + "✅ Check Subscription" button (inline)
- Verify subscription via Telegram API - block access if not subscribed to all
- **Admins skip subscription check entirely**

### Main Menu
After verification passes:
- 🎬 Movies | 📺 TV Series (row 1)
- 🔍 Search TV Channels (row 2)

## Content Delivery

### Movies
- User provides: IMDb ID, TMDB ID, or movie name
- Query PocketBase for metadata
- Download video from VidSrc using provided scraping logic (iframe extraction → browser automation with chromedp → stream capture → ffmpeg download)
- **Send actual video file to user via Telegram** with metadata (poster, title, description, year, genre, rating)
- Add inline buttons:
  - **Regular users:** "🌐 Watch on Website" (opens your website with this movie)
  - **Admins:** "🌐 Watch on Website" + "📥 Watch Without Ads" (downloads video, sends file directly)
- Update watch counter and history in PocketBase

### TV Series
- User provides: IMDb ID, TMDB ID, or series name
- Display series info → "Select Season ⬇️" button
- Show season list → show episode list → "◀️ Back to Seasons"
- When episode selected: same video delivery as movies (send file + buttons)
- Update watch counter and history

### Video Processing Pipeline
- Extract iframe from VidSrc embed page
- Use headless Chrome (chromedp) to capture video stream URLs (.m3u8, .mp4)
- Filter out ads/trackers
- Download with ffmpeg (retry logic, proper headers)
- Send video file directly to user via Telegram

## Admin System

### Access
- `/admin` command only for users in PocketBase admins collection
- Non-admins ignored or get "⛔ Unauthorized"

### Admin Panel
Four options:
1. **📢 Mandatory Channels** - Add/remove/edit/toggle channels
2. **📤 Broadcast Message** - Send text or image+caption to all active users (rate limit: 50ms delay, show progress)
3. **👥 Users Management** - Export users (.xlsx), get user by ID, block/unblock, send direct message
4. **📊 Statistics** - Total users, movies watched, episodes watched, today's activity, top watchers

## Technical Implementation

### Video Download
Convert provided Python scraping code to Go:
- Parse HTML for iframe
- Browser automation with chromedp
- Network traffic capture
- Stream download with ffmpeg
- Save to ./movies/ directory

### Error Handling
- Invalid ID format
- Content not found in database
- Download failures
- Subscription check failures
- PocketBase API failures (retry 3x)
- Rate limiting (max 10 requests/user/hour)
- Disk space alerts

### Security
- Input validation
- Path traversal prevention
- Admin verification for sensitive operations
- Proper PocketBase authentication
- No sensitive data in logs

### File Management
- Downloaded videos stored locally
- Clean up old files as needed (implementation flexible)

## Implementation Notes
- Use structured logging
- Modular functions
- Goroutines for concurrent operations
- Follow Go best practices
- Chrome/ChromeDriver required
- FFmpeg required
- All persistence via PocketBase API

## Future Enhancements (Optional)
- Favorites/watchlist
- Watch history
- Multiple quality options
- Subtitles
- Download limits
- Premium tiers

================ This is the code that downloads the vieo from VidSrc ====================
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func main() {
	embedURL := "https://vidsrc-embed.ru/embed/tt0460649/3-4"
	if len(os.Args) > 1 {
		embedURL = os.Args[1]
	}

	os.MkdirAll("movies", 0755)

	if !downloadVideo(embedURL) {
		os.Exit(1)
	}
}

func downloadVideo(embedURL string) bool {
	// STEP 1/3
	fmt.Println("\n[STEP 1/3] Extracting iframe...")
	iframeURL, err := getIframeSrc(embedURL)
	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return false
	}
	fmt.Println("✓ Iframe found")

	// STEP 2/3
	fmt.Println("\n[STEP 2/3] Finding video streams...")
	streams := extractStreams(iframeURL)

	if len(streams) == 0 {
		fmt.Println("❌ ERROR: No streams found")
		return false
	}
	fmt.Printf("✓ Found %d stream(s)\n", len(streams))

	streams = prioritizeStreams(streams)

	// STEP 3/3
	fmt.Println("\n[STEP 3/3] Downloading video...")
	fmt.Println("Starting download...\n")
	
	for _, stream := range streams {
		if downloadWithFFmpeg(stream, iframeURL) {
			return true
		}
	}

	fmt.Println("\n❌ ERROR: Download failed")
	return false
}

func getIframeSrc(embedURL string) (string, error) {
	req, _ := http.NewRequest("GET", embedURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	iframe := doc.Find("iframe#player_iframe").First()
	if iframe.Length() == 0 {
		return "", fmt.Errorf("iframe not found")
	}

	src, _ := iframe.Attr("src")
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	} else if strings.HasPrefix(src, "/") {
		parts := strings.Split(embedURL, "//")
		domain := strings.Split(parts[1], "/")[0]
		src = fmt.Sprintf("https://%s%s", domain, src)
	}

	return src, nil
}

func extractStreams(iframeURL string) []string {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// Keep 60s timeout for proper stream detection
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	streams := make(map[string]bool)

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			url := ev.Request.URL
			if isVideoURL(url) && !isAdURL(url) {
				streams[url] = true
			}
		}
	})

	tasks := chromedp.Tasks{
		network.Enable(),
		chromedp.Navigate(iframeURL),
		chromedp.Sleep(8 * time.Second),
	}

	// Click play buttons
	playSelectors := []string{
		`document.querySelector('button[aria-label*="play" i]')`,
		`document.querySelector('.play-button')`,
		`document.querySelector('#pl_but')`,
		`document.querySelector('.vjs-big-play-button')`,
		`document.querySelector('button')`,
		`document.querySelector('[class*="play" i]')`,
	}

	for _, selector := range playSelectors {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			var exists bool
			chromedp.Evaluate(fmt.Sprintf(`!!(%s)`, selector), &exists).Do(ctx)
			if exists {
				js := fmt.Sprintf(`
					try {
						const btn = %s;
						if (btn) {
							btn.click();
							console.log('Clicked play button');
						}
					} catch(e) { console.log(e); }
				`, selector)
				chromedp.Evaluate(js, nil).Do(ctx)
				time.Sleep(3 * time.Second)
			}
			return nil
		}))
	}

	// Handle nested iframes
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		var iframes []*cdp.Node
		chromedp.Nodes("iframe", &iframes, chromedp.ByQueryAll).Do(ctx)

		for i, iframe := range iframes {
			if i > 2 {
				break
			}

			frameCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			chromedp.Run(frameCtx,
				chromedp.Navigate(iframe.AttributeValue("src")),
				chromedp.Sleep(5*time.Second),
				chromedp.Evaluate(`
					try {
						const video = document.querySelector('video');
						if (video && video.src) console.log('VIDEO:', video.src);
						
						const playBtn = document.querySelector('button, .play-button, [class*="play"]');
						if (playBtn) playBtn.click();
					} catch(e) {}
				`, nil),
			)
		}
		return nil
	}))

	// Check video elements
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		var videoSrcs []string
		js := `Array.from(document.querySelectorAll('video')).map(v => v.src || v.currentSrc).filter(Boolean)`
		chromedp.Evaluate(js, &videoSrcs).Do(ctx)

		for _, src := range videoSrcs {
			if strings.HasPrefix(src, "http") && !streams[src] {
				streams[src] = true
			}
		}
		return nil
	}))

	tasks = append(tasks, chromedp.Sleep(10*time.Second))

	if err := chromedp.Run(ctx, tasks); err != nil {
		fmt.Printf("(browser warning: %v)\n", err)
	}

	result := []string{}
	for url := range streams {
		result = append(result, url)
	}

	return result
}

func downloadWithFFmpeg(streamURL, referer string) bool {
	if !commandExists("ffmpeg") {
		fmt.Println("❌ ERROR: ffmpeg not installed")
		return false
	}

	timestamp := time.Now().Unix()
	outputFile := filepath.Join("movies", fmt.Sprintf("video_%d.mp4", timestamp))

	args := []string{
		"-loglevel", "info",
		"-headers", fmt.Sprintf("Referer: %s\r\n", referer),
		"-i", streamURL,
		"-c", "copy",
		"-progress", "pipe:2",
		"-y",
		outputFile,
	}

	cmd := exec.Command("ffmpeg", args...)
	
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return false
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("❌ ERROR: %v\n", err)
		return false
	}

	scanner := bufio.NewScanner(stderr)
	
	var totalDuration float64
	startTime := time.Now()
	lastUpdate := time.Now()

	timeRegex := regexp.MustCompile(`time=(\d+):(\d+):(\d+\.\d+)`)
	durationRegex := regexp.MustCompile(`Duration: (\d+):(\d+):(\d+\.\d+)`)
	speedRegex := regexp.MustCompile(`speed=\s*(\d+\.?\d*)x`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := durationRegex.FindStringSubmatch(line); len(matches) == 4 {
			h, _ := strconv.ParseFloat(matches[1], 64)
			m, _ := strconv.ParseFloat(matches[2], 64)
			s, _ := strconv.ParseFloat(matches[3], 64)
			totalDuration = h*3600 + m*60 + s
		}

		if matches := timeRegex.FindStringSubmatch(line); len(matches) == 4 {
			if time.Since(lastUpdate) < 500*time.Millisecond {
				continue
			}
			lastUpdate = time.Now()

			h, _ := strconv.ParseFloat(matches[1], 64)
			m, _ := strconv.ParseFloat(matches[2], 64)
			s, _ := strconv.ParseFloat(matches[3], 64)
			currentTime := h*3600 + m*60 + s

			if totalDuration > 0 {
				percentage := (currentTime / totalDuration) * 100
				
				elapsed := time.Since(startTime).Seconds()
				var eta string
				if percentage > 1 {
					totalEstimated := elapsed * 100 / percentage
					remaining := totalEstimated - elapsed
					eta = formatDuration(remaining)
				} else {
					eta = "calculating..."
				}

				speed := "N/A"
				if speedMatches := speedRegex.FindStringSubmatch(line); len(speedMatches) == 2 {
					speed = speedMatches[1] + "x"
				}

				fmt.Printf("\r📥 %.1f%% | ETA: %s | Speed: %s    ", percentage, eta, speed)
			}
		}
	}

	err = cmd.Wait()
	fmt.Println()

	if err != nil {
		fmt.Println("\nRetrying with re-encode...")
		return downloadReencode(streamURL, referer, outputFile)
	}

	if fileExists(outputFile) && getFileSize(outputFile) > 500000 {
		size := float64(getFileSize(outputFile)) / 1024 / 1024
		fmt.Printf("\n✅ SUCCESS! %s (%.1f MB)\n", filepath.Base(outputFile), size)
		fmt.Println(strings.Repeat("=", 50))
		return true
	}

	return false
}

func downloadReencode(streamURL, referer, outputFile string) bool {
	args := []string{
		"-loglevel", "info",
		"-headers", fmt.Sprintf("Referer: %s\r\n", referer),
		"-i", streamURL,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "23",
		"-c:a", "copy",
		"-progress", "pipe:2",
		"-y",
		outputFile,
	}

	cmd := exec.Command("ffmpeg", args...)
	stderr, _ := cmd.StderrPipe()
	cmd.Start()

	scanner := bufio.NewScanner(stderr)
	
	var totalDuration float64
	startTime := time.Now()
	lastUpdate := time.Now()

	timeRegex := regexp.MustCompile(`time=(\d+):(\d+):(\d+\.\d+)`)
	durationRegex := regexp.MustCompile(`Duration: (\d+):(\d+):(\d+\.\d+)`)
	speedRegex := regexp.MustCompile(`speed=\s*(\d+\.?\d*)x`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := durationRegex.FindStringSubmatch(line); len(matches) == 4 {
			h, _ := strconv.ParseFloat(matches[1], 64)
			m, _ := strconv.ParseFloat(matches[2], 64)
			s, _ := strconv.ParseFloat(matches[3], 64)
			totalDuration = h*3600 + m*60 + s
		}

		if matches := timeRegex.FindStringSubmatch(line); len(matches) == 4 {
			if time.Since(lastUpdate) < 500*time.Millisecond {
				continue
			}
			lastUpdate = time.Now()

			h, _ := strconv.ParseFloat(matches[1], 64)
			m, _ := strconv.ParseFloat(matches[2], 64)
			s, _ := strconv.ParseFloat(matches[3], 64)
			currentTime := h*3600 + m*60 + s

			if totalDuration > 0 {
				percentage := (currentTime / totalDuration) * 100
				
				elapsed := time.Since(startTime).Seconds()
				var eta string
				if percentage > 1 {
					totalEstimated := elapsed * 100 / percentage
					remaining := totalEstimated - elapsed
					eta = formatDuration(remaining)
				} else {
					eta = "calculating..."
				}

				speed := "N/A"
				if speedMatches := speedRegex.FindStringSubmatch(line); len(speedMatches) == 2 {
					speed = speedMatches[1] + "x"
				}

				fmt.Printf("\r📥 %.1f%% | ETA: %s | Speed: %s    ", percentage, eta, speed)
			}
		}
	}

	err := cmd.Wait()
	fmt.Println()

	if err != nil {
		fmt.Println("❌ ERROR: Download failed")
		return false
	}

	if fileExists(outputFile) && getFileSize(outputFile) > 500000 {
		size := float64(getFileSize(outputFile)) / 1024 / 1024
		fmt.Printf("\n✅ SUCCESS! %s (%.1f MB)\n", filepath.Base(outputFile), size)
		fmt.Println(strings.Repeat("=", 50))
		return true
	}

	return false
}

func formatDuration(seconds float64) string {
	if seconds < 0 {
		return "00:00"
	}
	
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

func isVideoURL(url string) bool {
	lower := strings.ToLower(url)
	return strings.Contains(lower, ".m3u8") ||
		strings.Contains(lower, ".mp4") ||
		strings.Contains(lower, ".ts") ||
		strings.Contains(lower, "/manifest") ||
		strings.Contains(lower, "/playlist") ||
		strings.Contains(lower, "master.m3u") ||
		strings.Contains(lower, "index.m3u")
}

func isAdURL(url string) bool {
	lower := strings.ToLower(url)
	badDomains := []string{"google", "doubleclick", "facebook", "twitter", "analytics", "tracking", "ads", "ad.", "/ad/", "histats", "statcounter", "adservice", "adnxs", "adsystem"}
	for _, bad := range badDomains {
		if strings.Contains(lower, bad) {
			return true
		}
	}
	return false
}

func prioritizeStreams(streams []string) []string {
	m3u8 := []string{}
	other := []string{}

	for _, s := range streams {
		if strings.Contains(s, ".m3u8") {
			m3u8 = append(m3u8, s)
		} else {
			other = append(other, s)
		}
	}

	return append(m3u8, other...)
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}




======================= this is the code that streams downloaded videos =======================
package main // Make streamable

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	moviesDir = "./movies"
	hlsDir    = "./hls"
	port      = ":8888"
)

func main() {
	if err := os.MkdirAll(hlsDir, 0755); err != nil {
		log.Fatal("Failed to create HLS directory:", err)
	}

	convertAllVideos()

	http.HandleFunc("/", playerHandler)
	
	http.HandleFunc("/hls/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-cache")
		path := strings.TrimPrefix(r.URL.Path, "/hls/")
		fullPath := filepath.Join(hlsDir, path)
		http.ServeFile(w, r, fullPath)
	})

	fmt.Printf("\n🎬 Server running on http://localhost%s\n", port)
	fmt.Println("Press Ctrl+C to stop")
	log.Fatal(http.ListenAndServe(port, nil))
}

func convertAllVideos() {
	files, err := filepath.Glob(filepath.Join(moviesDir, "*.mp4"))
	if err != nil {
		log.Println("Error:", err)
		return
	}

	if len(files) == 0 {
		log.Println("No MP4 files found")
		return
	}

	fmt.Println("\n╔════════════════════════════════════════╗")
	fmt.Println("║         VIDEO CONVERSION               ║")
	fmt.Println("╚════════════════════════════════════════╝")

	totalStart := time.Now()
	completedCount := 0
	var totalConversionTime time.Duration

	for i, file := range files {
		baseName := filepath.Base(file)
		
		outputName := strings.TrimSuffix(baseName, filepath.Ext(baseName))
		playlistPath := filepath.Join(hlsDir, outputName+".m3u8")
		
		if _, err := os.Stat(playlistPath); err == nil {
			fmt.Printf("\n[%d/%d] %s\n", i+1, len(files), baseName)
			fmt.Println("✓ Already converted - SKIPPED")
			continue
		}

		fmt.Printf("\n[%d/%d] %s\n", i+1, len(files), baseName)
		
		if completedCount > 0 {
			avgTime := totalConversionTime / time.Duration(completedCount)
			remainingFiles := len(files) - i
			remaining := avgTime * time.Duration(remainingFiles)
			etaStr := formatDuration(remaining)
			fmt.Printf("⏱  ETA: %s remaining\n", etaStr)
		}
		
		fileStart := time.Now()
		if err := convertToHLS(file); err != nil {
			fmt.Printf("\n❌ ERROR: %v\n", err)
		} else {
			elapsed := time.Since(fileStart)
			totalConversionTime += elapsed
			completedCount++
			fmt.Printf("\n✓ Completed in %s\n", formatDuration(elapsed))
		}
	}
	
	totalTime := time.Since(totalStart)
	fmt.Println("\n╔════════════════════════════════════════╗")
	fmt.Printf("║ TOTAL TIME: %-26s ║\n", formatDuration(totalTime))
	fmt.Printf("║ CONVERTED: %-27d ║\n", completedCount)
	fmt.Println("╚════════════════════════════════════════╝\n")
}

func formatDuration(d time.Duration) string {
	totalSeconds := int(d.Seconds())
	
	if totalSeconds < 60 {
		return fmt.Sprintf("%d seconds", totalSeconds)
	}
	
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	
	if minutes < 60 {
		return fmt.Sprintf("%d minutes %d seconds", minutes, seconds)
	}
	
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%d hours %d minutes %d seconds", hours, minutes, seconds)
}

func convertToHLS(inputFile string) error {
	baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	outputDir := filepath.Join(hlsDir, baseName)
	
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	playlistPath := filepath.Join(hlsDir, baseName+".m3u8")
	segmentPath := filepath.Join(outputDir, "segment_%03d.ts")

	duration, err := getVideoDuration(inputFile)
	if err != nil {
		duration = 0
	}

	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "28",
		"-c:a", "copy",
		"-hls_time", "6",
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPath,
		"-hls_base_url", baseName+"/",
		"-progress", "pipe:1",
		"-loglevel", "error",
		"-y",
		playlistPath,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	startTime := time.Now()
	
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_ms=") {
			timeStr := strings.TrimPrefix(line, "out_time_ms=")
			timeMicro, err := strconv.ParseInt(timeStr, 10, 64)
			if err == nil && duration > 0 {
				currentTime := float64(timeMicro) / 1000000.0
				progress := (currentTime / duration) * 100
				if progress > 100 {
					progress = 100
				}
				
				elapsed := time.Since(startTime)
				if progress > 1 {
					totalEstimated := elapsed.Seconds() / (progress / 100.0)
					remaining := totalEstimated - elapsed.Seconds()
					if remaining > 0 {
						printProgressWithETA(int(progress), int(remaining))
					} else {
						printProgress(int(progress))
					}
				} else {
					printProgress(int(progress))
				}
			}
		}
	}

	return cmd.Wait()
}

func getVideoDuration(inputFile string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputFile,
	)
	
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, err
	}
	
	return duration, nil
}

func printProgress(percent int) {
	barWidth := 40
	filled := (percent * barWidth) / 100
	if filled > barWidth {
		filled = barWidth
	}
	
	fmt.Print("\r╠")
	for i := 0; i < barWidth; i++ {
		if i < filled {
			fmt.Print("█")
		} else {
			fmt.Print("░")
		}
	}
	fmt.Printf("╣ %3d%%", percent)
}

func printProgressWithETA(percent int, remainingSeconds int) {
	barWidth := 40
	filled := (percent * barWidth) / 100
	if filled > barWidth {
		filled = barWidth
	}
	
	var etaStr string
	if remainingSeconds < 60 {
		etaStr = fmt.Sprintf("%ds", remainingSeconds)
	} else {
		mins := remainingSeconds / 60
		secs := remainingSeconds % 60
		etaStr = fmt.Sprintf("%dm %ds", mins, secs)
	}
	
	fmt.Print("\r╠")
	for i := 0; i < barWidth; i++ {
		if i < filled {
			fmt.Print("█")
		} else {
			fmt.Print("░")
		}
	}
	fmt.Printf("╣ %3d%% | ETA: %s   ", percent, etaStr)
}

func playerHandler(w http.ResponseWriter, r *http.Request) {
	files, _ := filepath.Glob(filepath.Join(hlsDir, "*.m3u8"))
	
	if len(files) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><body style="background:#000;color:#fff;display:flex;align-items:center;justify-content:center;height:100vh;font-family:Arial;"><h1>No videos available</h1></body></html>`)
		return
	}
	
	videoName := strings.TrimSuffix(filepath.Base(files[0]), ".m3u8")
	displayName := strings.ReplaceAll(videoName, "_", " ")
	m3u8URL := "/hls/" + videoName + ".m3u8"

	html := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>` + displayName + `</title>
	<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
	<style>
		body {
			margin: 0;
			background: #000;
			display: flex;
			flex-direction: column;
			min-height: 100vh;
		}
		.header {
			background: #1a1a1a;
			padding: 20px;
			text-align: center;
			color: white;
		}
		.video-container {
			flex: 1;
			display: flex;
			align-items: center;
			justify-content: center;
			padding: 20px;
		}
		video {
			width: 100%;
			max-width: 1400px;
			max-height: 85vh;
		}
	</style>
</head>
<body>
	<div class="header">
		<h1>` + displayName + `</h1>
	</div>
	<div class="video-container">
		<video id="video" controls autoplay></video>
	</div>

	<script>
		const video = document.getElementById('video');
		const videoSrc = '` + m3u8URL + `';
		
		if (Hls.isSupported()) {
			const hls = new Hls({
				debug: false,
				maxBufferLength: 30,
				maxMaxBufferLength: 60,
			});
			
			hls.loadSource(videoSrc);
			hls.attachMedia(video);
			
			hls.on(Hls.Events.MANIFEST_PARSED, function() {
				video.play();
			});
			
			hls.on(Hls.Events.ERROR, function(event, data) {
				if (data.fatal) {
					switch(data.type) {
						case Hls.ErrorTypes.NETWORK_ERROR:
							hls.startLoad();
							break;
						case Hls.ErrorTypes.MEDIA_ERROR:
							hls.recoverMediaError();
							break;
						default:
							hls.destroy();
							break;
					}
				}
			});
		} else if (video.canPlayType('application/vnd.apple.mpegurl')) {
			video.src = videoSrc;
		}
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}