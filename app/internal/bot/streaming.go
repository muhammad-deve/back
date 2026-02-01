package bot

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type StreamServer struct {
	addr    string
	baseURL string
	hlsDir  string

	mu    sync.RWMutex
	items map[string]time.Time

	once sync.Once
}

const streamTTL = 24 * time.Hour

func NewStreamServer(addr, baseURL, hlsDir string) *StreamServer {
	_ = os.MkdirAll(hlsDir, 0755)
	return &StreamServer{
		addr:    addr,
		baseURL: strings.TrimRight(baseURL, "/"),
		hlsDir:  hlsDir,
		items:   map[string]time.Time{},
	}
}

func (s *StreamServer) Start() {
	s.once.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/hls/", s.handleHLS)
		mux.HandleFunc("/", s.handlePlayer)
		go s.cleanupLoop()
		go func() {
			_ = http.ListenAndServe(s.addr, mux)
		}()
	})
}

func (s *StreamServer) cleanupLoop() {
	t := time.NewTicker(30 * time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-streamTTL)
		var toDelete []string

		s.mu.RLock()
		for id, created := range s.items {
			if created.Before(cutoff) {
				toDelete = append(toDelete, id)
			}
		}
		s.mu.RUnlock()

		if len(toDelete) == 0 {
			continue
		}

		s.mu.Lock()
		for _, id := range toDelete {
			delete(s.items, id)
		}
		s.mu.Unlock()

		for _, id := range toDelete {
			_ = os.RemoveAll(filepath.Join(s.hlsDir, id))
		}
	}
}

func (s *StreamServer) BaseURL() string {
	return s.baseURL
}

func (s *StreamServer) CreateStreamFromMP4(mp4Path string) (string, error) {
	id := uuid.NewString()
	outDir := filepath.Join(s.hlsDir, id)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}

	playlist := filepath.Join(outDir, "index.m3u8")
	segmentPattern := filepath.Join(outDir, "segment_%03d.ts")

	duration := float64(0)
	if commandExists("ffprobe") {
		if d, err := probeDurationSeconds(mp4Path); err == nil {
			duration = d
		}
	}

	args := []string{
		"-i", mp4Path,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "28",
		"-c:a", "copy",
		"-hls_time", "6",
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPattern,
		"-progress", "pipe:1",
		"-loglevel", "error",
		"-y",
		playlist,
	}

	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return "", err
	}

	go func() {
		_, _ = io.Copy(io.Discard, stderr)
	}()

	scan := bufio.NewScanner(stdout)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if duration <= 0 {
			continue
		}
		if strings.HasPrefix(line, "out_time_ms=") {
			n := strings.TrimPrefix(line, "out_time_ms=")
			us, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				continue
			}
			_ = float64(us) / 1000000.0
		}
	}

	if err := cmd.Wait(); err != nil {
		return "", err
	}

	s.mu.Lock()
	s.items[id] = time.Now()
	s.mu.Unlock()

	return id, nil
}

func (s *StreamServer) Has(id string) bool {
	if !uuidRegex.MatchString(id) {
		return false
	}
	s.mu.RLock()
	_, ok := s.items[id]
	s.mu.RUnlock()
	return ok
}

var uuidRegex = regexp.MustCompile(`^[a-f0-9\-]{36}$`)

func (s *StreamServer) handlePlayer(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	path = strings.TrimSpace(path)
	if path == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if !uuidRegex.MatchString(path) || !s.Has(path) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	m3u8URL := "/hls/" + path + "/index.m3u8"
	html := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>Stream</title>
	<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
	<style>
		body{margin:0;background:#000;display:flex;flex-direction:column;min-height:100vh}
		.video{flex:1;display:flex;align-items:center;justify-content:center;padding:20px}
		video{width:100%;max-width:1400px;max-height:85vh}
	</style>
</head>
<body>
	<div class="video">
		<video id="video" controls autoplay></video>
	</div>
	<script>
		const video=document.getElementById('video');
		const src='` + m3u8URL + `';
		if (Hls.isSupported()) {
			const hls=new Hls({debug:false,maxBufferLength:30,maxMaxBufferLength:60});
			hls.loadSource(src);
			hls.attachMedia(video);
			hls.on(Hls.Events.MANIFEST_PARSED, function(){video.play();});
			return;
		}
		if (video.canPlayType('application/vnd.apple.mpegurl')) { video.src=src; }
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *StreamServer) handleHLS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")

	path := strings.TrimPrefix(r.URL.Path, "/hls/")
	clean := filepath.Clean(path)
	full := filepath.Join(s.hlsDir, clean)

	rel, err := filepath.Rel(s.hlsDir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	id := parts[0]
	if !s.Has(id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, full)
}

func probeDurationSeconds(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	v := strings.TrimSpace(string(out))
	return strconv.ParseFloat(v, 64)
}
