package bot

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// DownloadManager handles video downloads
type DownloadManager struct {
	outputDir string
	mu        sync.Mutex
	active    map[string]bool
}

type DownloadProgress struct {
	Percent float64
	ETA     string
}

// NewDownloadManager creates a new download manager
func NewDownloadManager(outputDir string) *DownloadManager {
	os.MkdirAll(outputDir, 0755)
	return &DownloadManager{
		outputDir: outputDir,
		active:    make(map[string]bool),
	}
}

func (dm *DownloadManager) isVerbose() bool {
	return false
}

// DownloadVideo downloads a video from VidSrc embed URL
func (dm *DownloadManager) DownloadVideo(embedURL, title string) (string, error) {
	return dm.DownloadVideoWithProgress(embedURL, title, nil)
}

func (dm *DownloadManager) DownloadVideoWithProgress(embedURL, title string, onProgress func(DownloadProgress)) (string, error) {
	dm.mu.Lock()
	if dm.active[embedURL] {
		dm.mu.Unlock()
		return "", fmt.Errorf("download already in progress")
	}
	dm.active[embedURL] = true
	dm.mu.Unlock()

	defer func() {
		dm.mu.Lock()
		delete(dm.active, embedURL)
		dm.mu.Unlock()
	}()

	if dm.isVerbose() {
		log.Printf("[DOWNLOAD] Starting download: %s", title)
	}

	// Step 1: Extract iframe
	if dm.isVerbose() {
		log.Println("[STEP 1/3] Extracting iframe...")
	}
	iframeURL, err := getIframeSrc(embedURL)
	if err != nil {
		return "", fmt.Errorf("failed to extract iframe: %w", err)
	}
	if dm.isVerbose() {
		log.Println("✓ Iframe found")
	}

	// Step 2: Extract streams
	if dm.isVerbose() {
		log.Println("[STEP 2/3] Finding video streams...")
	}
	streams := extractStreams(iframeURL)
	if len(streams) == 0 {
		return "", fmt.Errorf("no video streams found")
	}
	if dm.isVerbose() {
		log.Printf("✓ Found %d stream(s)", len(streams))
	}

	streams = prioritizeStreams(streams)

	// Step 3: Download with ffmpeg
	if dm.isVerbose() {
		log.Println("[STEP 3/3] Downloading video...")
	}

	// Clean title for filename
	safeTitle := sanitizeFilename(title)
	outputFile := filepath.Join(dm.outputDir, fmt.Sprintf("%s_%d.mp4", safeTitle, time.Now().Unix()))

	for _, stream := range streams {
		if downloadWithFFmpeg(stream, iframeURL, outputFile, onProgress, dm.isVerbose()) {
			return outputFile, nil
		}
	}

	return "", fmt.Errorf("all download attempts failed")
}

// getIframeSrc extracts the iframe source URL from the embed page
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

// extractStreams uses headless Chrome to capture video stream URLs
func extractStreams(iframeURL string) []string {
	// Suppress chromedp logging errors (cookie parsing, navigation events)
	// These are harmless and don't affect video extraction
	oldLog := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(oldLog)

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

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

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
		log.Printf("Browser warning: %v", err)
	}

	result := []string{}
	for url := range streams {
		result = append(result, url)
	}

	return result
}

// downloadWithFFmpeg downloads the stream using ffmpeg
func downloadWithFFmpeg(streamURL, referer, outputFile string, onProgress func(DownloadProgress), verbose bool) bool {
	if !commandExists("ffmpeg") {
		log.Println("❌ ffmpeg not installed")
		return false
	}

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
		log.Printf("❌ Error: %v", err)
		return false
	}

	if err := cmd.Start(); err != nil {
		log.Printf("❌ Error: %v", err)
		return false
	}

	scanner := bufio.NewScanner(stderr)

	var totalDuration float64
	startTime := time.Now()
	lastUpdate := time.Now()
	lastPercent := -1.0

	timeRegex := regexp.MustCompile(`time=(\d+):(\d+):(\d+\.\d+)`)
	durationRegex := regexp.MustCompile(`Duration: (\d+):(\d+):(\d+\.\d+)`)

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

				if onProgress != nil {
					if lastPercent < 0 || percentage-lastPercent >= 0.1 {
						lastPercent = percentage
						onProgress(DownloadProgress{Percent: percentage, ETA: eta})
					}
				} else if verbose {
					log.Printf("📥 %.1f%% | ETA: %s", percentage, eta)
				}
			}
		}
	}

	err = cmd.Wait()

	if err != nil {
		if verbose {
			log.Println("Retrying with re-encode...")
		}
		return downloadReencode(streamURL, referer, outputFile)
	}

	if fileExists(outputFile) && getFileSize(outputFile) > 500000 {
		size := float64(getFileSize(outputFile)) / 1024 / 1024
		if verbose {
			log.Printf("✅ SUCCESS! %s (%.1f MB)", filepath.Base(outputFile), size)
		}
		return true
	}

	return false
}

// downloadReencode attempts download with video re-encoding
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
	for scanner.Scan() {
		// Just consume output
	}

	err := cmd.Wait()

	if err != nil {
		log.Println("❌ Download failed")
		return false
	}

	if fileExists(outputFile) && getFileSize(outputFile) > 500000 {
		size := float64(getFileSize(outputFile)) / 1024 / 1024
		log.Printf("✅ SUCCESS! %s (%.1f MB)", filepath.Base(outputFile), size)
		return true
	}

	return false
}

// Helper functions

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

func sanitizeFilename(name string) string {
	// Remove or replace invalid characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}
