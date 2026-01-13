package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	application "gitlab.yurtal.tech/company/pocketbase-app-template/internal/app"
	"gitlab.yurtal.tech/company/pocketbase-app-template/internal/config"
)

type channelJSON struct {
	Channel     string   `json:"channel"`
	Title       string   `json:"title"`
	Website     string   `json:"website"`
	URL         string   `json:"url"`
	Quality     string   `json:"quality"`
	Country     string   `json:"country"`
	CountryCode string   `json:"country_code"`
	Language    string   `json:"language"`
	Categories  []string `json:"categories"`
	IsWorking   bool     `json:"is_working"`
	LogoURL     string   `json:"logo_url"`
}

type pbAuthResp struct {
	Token string `json:"token"`
}

type pbListResp struct {
	Items []struct {
		Id string `json:"id"`
	} `json:"items"`
}

type pbListAnyResp struct {
	Page       int               `json:"page"`
	PerPage    int               `json:"perPage"`
	TotalItems int               `json:"totalItems"`
	TotalPages int               `json:"totalPages"`
	Items      []pbRecordAnyResp `json:"items"`
}

type pbRecordResp struct {
	Id string `json:"id"`
}

type pbRecordAnyResp map[string]any

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func pbURLJoin(base, p string) (string, error) {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return "", errors.New("empty PocketBase url")
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return base + p, nil
}

func pbDoJSON(client *http.Client, method, fullURL, token string, reqBody any, respBody any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return err
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", token)
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("request failed (%s %s): status=%d body=%s", method, fullURL, res.StatusCode, strings.TrimSpace(string(resBytes)))
	}

	if respBody == nil {
		return nil
	}

	if len(resBytes) == 0 {
		return nil
	}

	return json.Unmarshal(resBytes, respBody)
}

func pbAuthSuperuser(client *http.Client, pbBaseURL, email, password string) (string, error) {
	fullURL, err := pbURLJoin(pbBaseURL, "/api/collections/_superusers/auth-with-password")
	if err != nil {
		return "", err
	}

	var resp pbAuthResp
	err = pbDoJSON(client, http.MethodPost, fullURL, "", map[string]string{
		"identity": email,
		"password": password,
	}, &resp)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Token) == "" {
		return "", errors.New("missing auth token in response")
	}
	return resp.Token, nil
}

func pbFindFirstIDByField(client *http.Client, pbBaseURL, token, collection, field, value string) (string, error) {
	filter := fmt.Sprintf("%s=\"%s\"", field, strings.ReplaceAll(value, "\"", "\\\""))
	q := url.Values{}
	q.Set("filter", filter)
	q.Set("perPage", "1")
	q.Set("page", "1")

	fullURL, err := pbURLJoin(pbBaseURL, "/api/collections/"+collection+"/records?"+q.Encode())
	if err != nil {
		return "", err
	}

	var resp pbListResp
	if err := pbDoJSON(client, http.MethodGet, fullURL, token, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Items) == 0 {
		return "", nil
	}
	return resp.Items[0].Id, nil
}

func pbFindFirstIDByFilter(client *http.Client, pbBaseURL, token, collection, filter string) (string, error) {
	q := url.Values{}
	q.Set("filter", filter)
	q.Set("perPage", "1")
	q.Set("page", "1")

	fullURL, err := pbURLJoin(pbBaseURL, "/api/collections/"+collection+"/records?"+q.Encode())
	if err != nil {
		return "", err
	}

	var resp pbListResp
	if err := pbDoJSON(client, http.MethodGet, fullURL, token, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Items) == 0 {
		return "", nil
	}
	return resp.Items[0].Id, nil
}

func pbGetRecord(client *http.Client, pbBaseURL, token, collection, id string) (pbRecordAnyResp, error) {
	fullURL, err := pbURLJoin(pbBaseURL, "/api/collections/"+collection+"/records/"+id)
	if err != nil {
		return nil, err
	}
	var resp pbRecordAnyResp
	if err := pbDoJSON(client, http.MethodGet, fullURL, token, nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func pbCreateRecord(client *http.Client, pbBaseURL, token, collection string, payload any) (string, error) {
	fullURL, err := pbURLJoin(pbBaseURL, "/api/collections/"+collection+"/records")
	if err != nil {
		return "", err
	}
	var resp pbRecordResp
	if err := pbDoJSON(client, http.MethodPost, fullURL, token, payload, &resp); err != nil {
		return "", err
	}
	return resp.Id, nil
}

func pbUpdateRecord(client *http.Client, pbBaseURL, token, collection, id string, payload any) error {
	fullURL, err := pbURLJoin(pbBaseURL, "/api/collections/"+collection+"/records/"+id)
	if err != nil {
		return err
	}
	return pbDoJSON(client, http.MethodPatch, fullURL, token, payload, nil)
}

func renderProgress(done, total int, start time.Time) string {
	if total <= 0 {
		return ""
	}

	width := 30
	if width < 10 {
		width = 10
	}

	percent := float64(done) / float64(total)
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}

	elapsed := time.Since(start)
	eta := time.Duration(0)
	if done > 0 {
		perItem := elapsed / time.Duration(done)
		eta = perItem * time.Duration(total-done)
	}

	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	return fmt.Sprintf("[%s] %3d%% (%d/%d) ETA %s", bar, int(percent*100), done, total, eta.Round(time.Second))
}

type movieVideoJSON struct {
	AutoembedURL  string `json:"autoembed_url"`
	GomoURL       string `json:"gomo_url"`
	MoviesAPIURL  string `json:"moviesapi_url"`
	VidlinkProURL string `json:"vidlink_pro_url"`
	VidsrcURL     string `json:"vidsrc_url"`
}

type movieImageJSON struct {
	Height int    `json:"height"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
}

type movieRatingJSON struct {
	AggregateRating float64 `json:"aggregateRating"`
	VoteCount       int     `json:"voteCount"`
}

type moviePersonJSON struct {
	ID                 string          `json:"id"`
	DisplayName        string          `json:"displayName"`
	PrimaryProfessions []string        `json:"primaryProfessions"`
	PrimaryImage       *movieImageJSON `json:"primaryImage"`
}

type movieCountryJSON struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type movieJSON struct {
	ImdbID          string             `json:"imdb_id"`
	TmdbID          string             `json:"tmdb_id"`
	PrimaryVideo    *movieVideoJSON    `json:"primaryVideo"`
	Quality         string             `json:"quality"`
	Title           string             `json:"title"`
	PrimaryImage    *movieImageJSON    `json:"primaryImage"`
	StartYear       int                `json:"startYear"`
	RuntimeSeconds  int                `json:"runtimeSeconds"`
	Genres          []string           `json:"genres"`
	Rating          *movieRatingJSON   `json:"rating"`
	Plot            string             `json:"plot"`
	Directors       []moviePersonJSON  `json:"directors"`
	Writers         []moviePersonJSON  `json:"writers"`
	Stars           []moviePersonJSON  `json:"stars"`
	OriginCountries []movieCountryJSON `json:"originCountries"`
}

func normalizeGenreName(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

func normalizeProfessionValue(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimPrefix(s, "\\")
	s = strings.TrimSpace(s)
	switch s {
	case "actor", "actress", "director", "producer", "assistant":
		return s
	case "writers", "writer", "screenwriter", "scenarist":
		return "assistant"
	default:
		return ""
	}
}

func normalizeProfessions(role string, primary []string) []string {
	set := map[string]struct{}{}
	for _, p := range primary {
		if v := normalizeProfessionValue(p); v != "" {
			set[v] = struct{}{}
		}
	}

	// Ensure the role from the movie lists is always represented.
	if v := normalizeProfessionValue(role); v != "" {
		set[v] = struct{}{}
	} else if strings.TrimSpace(role) != "" {
		// For unknown roles (ex: "soundtrack"), don't add anything here.
	}

	if len(set) == 0 {
		set["assistant"] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func countJSONArrayItems(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return 0, fmt.Errorf("expected JSON array in %s", path)
	}

	count := 0
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return 0, err
		}
		count++
	}

	_, err = dec.Token()
	if err != nil {
		return 0, err
	}

	return count, nil
}

type movieJob struct {
	Kind    string
	Item    movieJSON
	Attempt int
}

func retry(attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 250 * time.Millisecond
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		d := time.Duration(float64(baseDelay) * math.Pow(2, float64(i)))
		if d > 10*time.Second {
			d = 10 * time.Second
		}
		time.Sleep(d)
	}
	return lastErr
}

func main() {
	log.Print("config initializing")
	cfg := config.GetConfig()

	app := application.NewApp(cfg)

	channelsCmd := &cobra.Command{
		Use:   "channels",
		Short: "Import channels from channels.json into PocketBase",
		RunE: func(cmd *cobra.Command, args []string) error {
			pbBaseURL, _ := cmd.Flags().GetString("pb-url")
			email, _ := cmd.Flags().GetString("email")
			password, _ := cmd.Flags().GetString("password")
			jsonPath, _ := cmd.Flags().GetString("file")

			if strings.TrimSpace(pbBaseURL) == "" {
				return errors.New("missing --pb-url")
			}
			if strings.TrimSpace(email) == "" {
				return errors.New("missing --email")
			}
			if strings.TrimSpace(password) == "" {
				return errors.New("missing --password")
			}
			if strings.TrimSpace(jsonPath) == "" {
				return errors.New("missing --file")
			}

			absPath, err := filepath.Abs(jsonPath)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(absPath)
			if err != nil {
				return err
			}

			var items []channelJSON
			if err := json.Unmarshal(b, &items); err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No channels found in JSON")
				return nil
			}

			client := &http.Client{Timeout: 60 * time.Second}
			token, err := pbAuthSuperuser(client, pbBaseURL, email, password)
			if err != nil {
				return err
			}

			start := time.Now()
			lastRender := time.Time{}
			total := len(items)

			countryByCode := map[string]string{}
			categoryByName := map[string]string{}

			for i, ch := range items {
				code := strings.TrimSpace(ch.CountryCode)
				if code != "" {
					if _, ok := countryByCode[code]; !ok {
						id, err := pbFindFirstIDByField(client, pbBaseURL, token, "countries", "code", code)
						if err != nil {
							return err
						}
						if id == "" {
							id, err = pbCreateRecord(client, pbBaseURL, token, "countries", map[string]any{
								"code":     code,
								"name":     strings.TrimSpace(ch.Country),
								"language": strings.TrimSpace(ch.Language),
							})
							if err != nil {
								return err
							}
						}
						countryByCode[code] = id
					}
				}

				categoryIDs := make([]string, 0, len(ch.Categories))
				for _, cat := range ch.Categories {
					name := strings.TrimSpace(strings.ToLower(cat))
					if name == "" {
						continue
					}
					id, ok := categoryByName[name]
					if !ok {
						id, err = pbFindFirstIDByField(client, pbBaseURL, token, "categories", "name", name)
						if err != nil {
							return err
						}
						if id == "" {
							id, err = pbCreateRecord(client, pbBaseURL, token, "categories", map[string]any{
								"name": name,
							})
							if err != nil {
								return err
							}
						}
						categoryByName[name] = id
					}
					categoryIDs = append(categoryIDs, id)
				}

				wbsite := strings.TrimSpace(ch.Website)
				if wbsite == "" {
					chKey := strings.TrimSpace(ch.Channel)
					if chKey != "" {
						wbsite = "https://" + chKey
					}
				}

				streamURL := strings.TrimSpace(ch.URL)

				payload := map[string]any{
					"title":             strings.TrimSpace(ch.Title),
					"wbsite":            wbsite,
					"stream_url":        streamURL,
					"logo_url":          strings.TrimSpace(ch.LogoURL),
					"quality":           strings.TrimSpace(ch.Quality),
					"is_url_working":    ch.IsWorking,
					"is_logo_available": strings.TrimSpace(ch.LogoURL) != "",
				}
				if code != "" {
					payload["country"] = countryByCode[code]
				}
				if len(categoryIDs) > 0 {
					payload["category"] = categoryIDs
				}

				existingID := ""
				if streamURL != "" {
					existingID, err = pbFindFirstIDByField(client, pbBaseURL, token, "channels", "stream_url", streamURL)
					if err != nil {
						return err
					}
				}
				if existingID == "" && wbsite != "" {
					existingID, err = pbFindFirstIDByField(client, pbBaseURL, token, "channels", "wbsite", wbsite)
					if err != nil {
						return err
					}
				}

				if existingID == "" {
					_, err = pbCreateRecord(client, pbBaseURL, token, "channels", payload)
					if err != nil {
						return err
					}
				} else {
					if err := pbUpdateRecord(client, pbBaseURL, token, "channels", existingID, payload); err != nil {
						return err
					}
				}

				done := i + 1
				now := time.Now()
				if done == total || now.Sub(lastRender) > 200*time.Millisecond {
					lastRender = now
					fmt.Printf("\r%s", renderProgress(done, total, start))
				}
			}

			fmt.Printf("\r%s\n", renderProgress(total, total, start))
			fmt.Println("Done")
			return nil
		},
	}

	channelsCmd.Flags().String("pb-url", envOrDefault("PB_URL", "http://127.0.0.1:8090"), "PocketBase base URL")
	channelsCmd.Flags().String("email", envOrDefault("PB_SUPERUSER_EMAIL", ""), "PocketBase superuser email")
	channelsCmd.Flags().String("password", envOrDefault("PB_SUPERUSER_PASSWORD", ""), "PocketBase superuser password")
	channelsCmd.Flags().String("file", envOrDefault("PB_CHANNELS_FILE", "../pkg/json/channels.json"), "Path to channels.json")

	app.RootCmd.AddCommand(channelsCmd)

	fixCmd := &cobra.Command{
		Use:   "fix",
		Short: "Fix missing series quality and content URLs using corrected serie*.json files",
		RunE: func(cmd *cobra.Command, args []string) error {
			pbBaseURL, _ := cmd.Flags().GetString("pb-url")
			email, _ := cmd.Flags().GetString("email")
			password, _ := cmd.Flags().GetString("password")
			jsonDir, _ := cmd.Flags().GetString("json-dir")
			workers, _ := cmd.Flags().GetInt("workers")
			retries, _ := cmd.Flags().GetInt("retries")
			maxAttempts, _ := cmd.Flags().GetInt("max-attempts")

			if strings.TrimSpace(pbBaseURL) == "" {
				return errors.New("missing --pb-url")
			}
			if strings.TrimSpace(email) == "" {
				return errors.New("missing --email")
			}
			if strings.TrimSpace(password) == "" {
				return errors.New("missing --password")
			}
			if strings.TrimSpace(jsonDir) == "" {
				return errors.New("missing --json-dir")
			}

			absDir, err := filepath.Abs(jsonDir)
			if err != nil {
				return err
			}
			entries, err := os.ReadDir(absDir)
			if err != nil {
				return err
			}

			paths := make([]string, 0, len(entries))
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := strings.ToLower(e.Name())
				if !strings.HasSuffix(name, ".json") {
					continue
				}
				if strings.HasPrefix(name, "serie") || strings.HasPrefix(name, "series") {
					paths = append(paths, filepath.Join(absDir, e.Name()))
				}
			}
			sort.Strings(paths)

			if len(paths) == 0 {
				return fmt.Errorf("no serie*.json files found in %s", absDir)
			}

			total := 0
			for _, p := range paths {
				c, err := countJSONArrayItems(p)
				if err != nil {
					return err
				}
				total += c
			}
			if total == 0 {
				fmt.Println("No items found")
				return nil
			}

			if workers <= 0 {
				workers = runtime.NumCPU()
			}
			if workers > 20 {
				workers = 20
			}
			if retries <= 0 {
				retries = 5
			}
			if maxAttempts < 0 {
				maxAttempts = 0
			}

			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.MaxIdleConns = 200
			transport.MaxIdleConnsPerHost = 200
			transport.IdleConnTimeout = 90 * time.Second
			client := &http.Client{Timeout: 60 * time.Second, Transport: transport}
			token, err := pbAuthSuperuser(client, pbBaseURL, email, password)
			if err != nil {
				return err
			}

			start := time.Now()
			var succeeded int64
			var failed int64

			jobs := make(chan movieJob, workers*8)
			var wg sync.WaitGroup
			var jobWG sync.WaitGroup
			stopProgress := make(chan struct{})

			fixOne := func(job movieJob) error {
				m := job.Item
				imdbID := strings.TrimSpace(m.ImdbID)
				if imdbID == "" {
					return errors.New("missing imdb_id")
				}

				existingMovieID, err := pbFindFirstIDByField(client, pbBaseURL, token, "movies", "imdb_id", imdbID)
				if err != nil {
					return err
				}
				if existingMovieID == "" {
					return nil
				}

				rec, err := pbGetRecord(client, pbBaseURL, token, "movies", existingMovieID)
				if err != nil {
					return err
				}
				contentID := ""
				if v, ok := rec["content_id"]; ok {
					if s, ok := v.(string); ok {
						contentID = s
					}
				}

				moviePatch := map[string]any{}
				if q := strings.TrimSpace(m.Quality); q != "" {
					moviePatch["quality"] = q
				}
				if len(moviePatch) > 0 {
					if err := pbUpdateRecord(client, pbBaseURL, token, "movies", existingMovieID, moviePatch); err != nil {
						return err
					}
				}

				contentPatch := map[string]any{}
				if m.PrimaryImage != nil {
					if u := strings.TrimSpace(m.PrimaryImage.URL); u != "" {
						contentPatch["poster_url"] = u
						contentPatch["poster_width"] = m.PrimaryImage.Width
						contentPatch["poster_height"] = m.PrimaryImage.Height
					}
				}
				if m.PrimaryVideo != nil {
					if u := strings.TrimSpace(m.PrimaryVideo.VidsrcURL); u != "" {
						contentPatch["vidsrc_url"] = u
					}
					if u := strings.TrimSpace(m.PrimaryVideo.VidlinkProURL); u != "" {
						contentPatch["vidlink_url"] = u
					}
					if u := strings.TrimSpace(m.PrimaryVideo.AutoembedURL); u != "" {
						contentPatch["autoembed_url"] = u
					}
					if u := strings.TrimSpace(m.PrimaryVideo.GomoURL); u != "" {
						contentPatch["gomo_url"] = u
					}
					if u := strings.TrimSpace(m.PrimaryVideo.MoviesAPIURL); u != "" {
						contentPatch["moviesapi_url"] = u
					}
				}

				if contentID == "" {
					if len(contentPatch) == 0 {
						return nil
					}
					cid, err := pbCreateRecord(client, pbBaseURL, token, "contents", contentPatch)
					if err != nil {
						return err
					}
					if err := pbUpdateRecord(client, pbBaseURL, token, "movies", existingMovieID, map[string]any{"content_id": cid}); err != nil {
						return err
					}
					return nil
				}

				if len(contentPatch) > 0 {
					if err := pbUpdateRecord(client, pbBaseURL, token, "contents", contentID, contentPatch); err != nil {
						return err
					}
				}
				return nil
			}

			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for job := range jobs {
						err := retry(retries, 250*time.Millisecond, func() error {
							return fixOne(job)
						})
						if err != nil {
							if maxAttempts == 0 || job.Attempt < maxAttempts {
								job.Attempt++
								select {
								case jobs <- job:
								default:
									go func(j movieJob) { jobs <- j }(job)
								}
								continue
							}
							atomic.AddInt64(&failed, 1)
							jobWG.Done()
							continue
						}
						atomic.AddInt64(&succeeded, 1)
						jobWG.Done()
					}
				}()
			}

			go func() {
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stopProgress:
						return
					case <-ticker.C:
						completed := int(atomic.LoadInt64(&succeeded) + atomic.LoadInt64(&failed))
						fmt.Printf("\r%s", renderProgress(completed, total, start))
					}
				}
			}()

			for _, p := range paths {
				f, err := os.Open(p)
				if err != nil {
					close(jobs)
					wg.Wait()
					return err
				}

				dec := json.NewDecoder(f)
				tok, err := dec.Token()
				if err != nil {
					f.Close()
					close(jobs)
					wg.Wait()
					return err
				}
				if d, ok := tok.(json.Delim); !ok || d != '[' {
					f.Close()
					close(jobs)
					wg.Wait()
					return fmt.Errorf("expected JSON array in %s", p)
				}

				for dec.More() {
					var item movieJSON
					if err := dec.Decode(&item); err != nil {
						f.Close()
						close(jobs)
						close(stopProgress)
						wg.Wait()
						return err
					}
					jobWG.Add(1)
					jobs <- movieJob{Kind: "serie", Item: item, Attempt: 1}
				}

				_, err = dec.Token()
				f.Close()
				if err != nil {
					close(jobs)
					close(stopProgress)
					wg.Wait()
					return err
				}
			}

			jobWG.Wait()
			close(jobs)
			wg.Wait()
			close(stopProgress)

			completed := int(atomic.LoadInt64(&succeeded) + atomic.LoadInt64(&failed))
			fmt.Printf("\r%s\n", renderProgress(completed, total, start))
			if f := atomic.LoadInt64(&failed); f > 0 {
				return fmt.Errorf("fix completed with %d failed items (increase --max-attempts or inspect data)", f)
			}
			fmt.Println("Done")
			return nil
		},
	}

	fixCmd.Flags().String("pb-url", envOrDefault("PB_URL", "http://127.0.0.1:8090"), "PocketBase base URL")
	fixCmd.Flags().String("email", envOrDefault("PB_SUPERUSER_EMAIL", ""), "PocketBase superuser email")
	fixCmd.Flags().String("password", envOrDefault("PB_SUPERUSER_PASSWORD", ""), "PocketBase superuser password")
	fixCmd.Flags().String("json-dir", envOrDefault("PB_MOVIES_DIR", "../pkg/json"), "Directory with serie*.json files")
	fixCmd.Flags().Int("workers", 10, "Number of concurrent workers (max 20)")
	fixCmd.Flags().Int("retries", 5, "Retries per item")
	fixCmd.Flags().Int("max-attempts", 50, "Max attempts per item before giving up (0 = infinite)")

	app.RootCmd.AddCommand(fixCmd)

	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Fill missing people profile images with a default placeholder",
		RunE: func(cmd *cobra.Command, args []string) error {
			pbBaseURL, _ := cmd.Flags().GetString("pb-url")
			email, _ := cmd.Flags().GetString("email")
			password, _ := cmd.Flags().GetString("password")
			workers, _ := cmd.Flags().GetInt("workers")
			retries, _ := cmd.Flags().GetInt("retries")
			maxAttempts, _ := cmd.Flags().GetInt("max-attempts")
			perPage, _ := cmd.Flags().GetInt("per-page")

			if strings.TrimSpace(pbBaseURL) == "" {
				return errors.New("missing --pb-url")
			}
			if strings.TrimSpace(email) == "" {
				return errors.New("missing --email")
			}
			if strings.TrimSpace(password) == "" {
				return errors.New("missing --password")
			}

			if workers <= 0 {
				workers = runtime.NumCPU()
			}
			if workers > 20 {
				workers = 20
			}
			if retries <= 0 {
				retries = 5
			}
			if maxAttempts < 0 {
				maxAttempts = 0
			}
			if perPage <= 0 {
				perPage = 200
			}
			if perPage > 500 {
				perPage = 500
			}

			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.MaxIdleConns = 200
			transport.MaxIdleConnsPerHost = 200
			transport.IdleConnTimeout = 90 * time.Second
			client := &http.Client{Timeout: 60 * time.Second, Transport: transport}

			token, err := pbAuthSuperuser(client, pbBaseURL, email, password)
			if err != nil {
				return err
			}

			defaultURL := "https://www.gravatar.com/avatar/?d=mp&s=256"

			listPage := func(page int) (pbListAnyResp, error) {
				q := url.Values{}
				q.Set("page", fmt.Sprintf("%d", page))
				q.Set("perPage", fmt.Sprintf("%d", perPage))
				q.Set("fields", "id,img_url,img_width,img_height")
				fullURL, err := pbURLJoin(pbBaseURL, "/api/collections/people/records?"+q.Encode())
				if err != nil {
					return pbListAnyResp{}, err
				}
				var resp pbListAnyResp
				if err := pbDoJSON(client, http.MethodGet, fullURL, token, nil, &resp); err != nil {
					return pbListAnyResp{}, err
				}
				return resp, nil
			}

			first, err := listPage(1)
			if err != nil {
				return err
			}
			if first.TotalItems == 0 {
				fmt.Println("No people records found")
				return nil
			}
			totalPages := first.TotalPages
			if totalPages <= 0 {
				totalPages = 1
			}

			type profileJob struct {
				ID      string
				Attempt int
			}

			jobs := make(chan profileJob, workers*8)
			var wg sync.WaitGroup
			var jobWG sync.WaitGroup
			stopProgress := make(chan struct{})

			start := time.Now()
			var processed int64
			var updated int64
			var failed int64

			patchOne := func(id string) error {
				payload := map[string]any{
					"img_url":    defaultURL,
					"img_width":  256,
					"img_height": 256,
				}
				return pbUpdateRecord(client, pbBaseURL, token, "people", id, payload)
			}

			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := range jobs {
						err := retry(retries, 250*time.Millisecond, func() error {
							return patchOne(j.ID)
						})
						if err != nil {
							if maxAttempts == 0 || j.Attempt < maxAttempts {
								j.Attempt++
								select {
								case jobs <- j:
								default:
									go func(job profileJob) { jobs <- job }(j)
								}
								continue
							}
							atomic.AddInt64(&failed, 1)
							atomic.AddInt64(&processed, 1)
							jobWG.Done()
							continue
						}
						atomic.AddInt64(&updated, 1)
						atomic.AddInt64(&processed, 1)
						jobWG.Done()
					}
				}()
			}

			go func() {
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stopProgress:
						return
					case <-ticker.C:
						done := int(atomic.LoadInt64(&processed))
						total := first.TotalItems
						if done > total {
							total = done
						}
						fmt.Printf("\r%s", renderProgress(done, total, start))
					}
				}
			}()

			processList := func(resp pbListAnyResp) {
				for _, it := range resp.Items {
					id, _ := it["id"].(string)
					if strings.TrimSpace(id) == "" {
						atomic.AddInt64(&processed, 1)
						continue
					}
					imgURL, _ := it["img_url"].(string)
					if strings.TrimSpace(imgURL) == "" {
						jobWG.Add(1)
						jobs <- profileJob{ID: id, Attempt: 1}
					} else {
						atomic.AddInt64(&processed, 1)
					}
				}
			}

			processList(first)
			for page := 2; page <= totalPages; page++ {
				resp, err := listPage(page)
				if err != nil {
					close(jobs)
					close(stopProgress)
					wg.Wait()
					return err
				}
				processList(resp)
			}

			jobWG.Wait()
			close(jobs)
			wg.Wait()
			close(stopProgress)

			done := int(atomic.LoadInt64(&processed))
			total := first.TotalItems
			if done > total {
				total = done
			}
			fmt.Printf("\r%s\n", renderProgress(done, total, start))
			if f := atomic.LoadInt64(&failed); f > 0 {
				return fmt.Errorf("profile completed with %d failed updates", f)
			}
			fmt.Printf("Updated %d people records\n", atomic.LoadInt64(&updated))
			fmt.Println("Done")
			return nil
		},
	}

	profileCmd.Flags().String("pb-url", envOrDefault("PB_URL", "http://127.0.0.1:8090"), "PocketBase base URL")
	profileCmd.Flags().String("email", envOrDefault("PB_SUPERUSER_EMAIL", ""), "PocketBase superuser email")
	profileCmd.Flags().String("password", envOrDefault("PB_SUPERUSER_PASSWORD", ""), "PocketBase superuser password")
	profileCmd.Flags().Int("workers", 10, "Number of concurrent workers (max 20)")
	profileCmd.Flags().Int("retries", 5, "Retries per item")
	profileCmd.Flags().Int("max-attempts", 50, "Max attempts per item before giving up (0 = infinite)")
	profileCmd.Flags().Int("per-page", 200, "PocketBase list page size")

	app.RootCmd.AddCommand(profileCmd)

	moviesCmd := &cobra.Command{
		Use:   "movies",
		Short: "Import movies and series JSON files into PocketBase",
		RunE: func(cmd *cobra.Command, args []string) error {
			pbBaseURL, _ := cmd.Flags().GetString("pb-url")
			email, _ := cmd.Flags().GetString("email")
			password, _ := cmd.Flags().GetString("password")
			jsonDir, _ := cmd.Flags().GetString("json-dir")
			workers, _ := cmd.Flags().GetInt("workers")
			retries, _ := cmd.Flags().GetInt("retries")
			maxAttempts, _ := cmd.Flags().GetInt("max-attempts")

			if strings.TrimSpace(pbBaseURL) == "" {
				return errors.New("missing --pb-url")
			}
			if strings.TrimSpace(email) == "" {
				return errors.New("missing --email")
			}
			if strings.TrimSpace(password) == "" {
				return errors.New("missing --password")
			}
			if strings.TrimSpace(jsonDir) == "" {
				return errors.New("missing --json-dir")
			}

			absDir, err := filepath.Abs(jsonDir)
			if err != nil {
				return err
			}

			entries, err := os.ReadDir(absDir)
			if err != nil {
				return err
			}

			paths := make([]string, 0, len(entries))
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := strings.ToLower(e.Name())
				if !strings.HasSuffix(name, ".json") {
					continue
				}
				if strings.HasPrefix(name, "movies") || strings.HasPrefix(name, "series") {
					paths = append(paths, filepath.Join(absDir, e.Name()))
				}
			}
			sort.Strings(paths)

			if len(paths) == 0 {
				return fmt.Errorf("no json files found in %s", absDir)
			}

			total := 0
			for _, p := range paths {
				c, err := countJSONArrayItems(p)
				if err != nil {
					return err
				}
				total += c
			}
			if total == 0 {
				fmt.Println("No items found")
				return nil
			}

			if workers <= 0 {
				workers = runtime.NumCPU()
			}
			if workers > 20 {
				workers = 20
			}
			if retries <= 0 {
				retries = 5
			}
			if maxAttempts < 0 {
				maxAttempts = 0
			}

			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.MaxIdleConns = 200
			transport.MaxIdleConnsPerHost = 200
			transport.IdleConnTimeout = 90 * time.Second
			client := &http.Client{Timeout: 60 * time.Second, Transport: transport}
			token, err := pbAuthSuperuser(client, pbBaseURL, email, password)
			if err != nil {
				return err
			}

			start := time.Now()
			var succeeded int64
			var failed int64

			genreCache := sync.Map{}
			countryCache := sync.Map{}

			getOrCreateGenre := func(name string) (string, error) {
				name = normalizeGenreName(name)
				if name == "" {
					return "", nil
				}
				if v, ok := genreCache.Load(name); ok {
					return v.(string), nil
				}
				id, err := pbFindFirstIDByField(client, pbBaseURL, token, "genres", "name", name)
				if err != nil {
					return "", err
				}
				if id == "" {
					id, err = pbCreateRecord(client, pbBaseURL, token, "genres", map[string]any{"name": name})
					if err != nil {
						return "", err
					}
				}
				genreCache.Store(name, id)
				return id, nil
			}

			getOrCreateCountry := func(code, name string) (string, error) {
				code = strings.TrimSpace(strings.ToUpper(code))
				if code == "" {
					return "", nil
				}
				if v, ok := countryCache.Load(code); ok {
					return v.(string), nil
				}
				id, err := pbFindFirstIDByField(client, pbBaseURL, token, "countries", "code", code)
				if err != nil {
					return "", err
				}
				if id == "" {
					id, err = pbCreateRecord(client, pbBaseURL, token, "countries", map[string]any{"code": code, "name": strings.TrimSpace(name), "language": ""})
					if err != nil {
						return "", err
					}
				}
				countryCache.Store(code, id)
				return id, nil
			}

			jobs := make(chan movieJob, workers*8)
			var wg sync.WaitGroup
			var jobWG sync.WaitGroup
			stopProgress := make(chan struct{})

			importOne := func(job movieJob) error {
				m := job.Item
				imdbID := strings.TrimSpace(m.ImdbID)
				tmdbID := strings.TrimSpace(m.TmdbID)
				if imdbID == "" {
					return errors.New("missing imdb_id")
				}

				genreIDs := make([]string, 0, len(m.Genres))
				seenGenre := map[string]struct{}{}
				for _, g := range m.Genres {
					gn := normalizeGenreName(g)
					if gn == "" {
						continue
					}
					if _, ok := seenGenre[gn]; ok {
						continue
					}
					seenGenre[gn] = struct{}{}
					id, err := getOrCreateGenre(gn)
					if err != nil {
						return err
					}
					if id != "" {
						genreIDs = append(genreIDs, id)
					}
				}

				countryIDs := make([]string, 0, len(m.OriginCountries))
				seenCountry := map[string]struct{}{}
				for _, c := range m.OriginCountries {
					cc := strings.TrimSpace(strings.ToUpper(c.Code))
					if cc == "" {
						continue
					}
					if _, ok := seenCountry[cc]; ok {
						continue
					}
					seenCountry[cc] = struct{}{}
					id, err := getOrCreateCountry(cc, c.Name)
					if err != nil {
						return err
					}
					if id != "" {
						countryIDs = append(countryIDs, id)
					}
				}

				movieType := "movie"
				if job.Kind == "serie" {
					movieType = "serie"
				}

				existingMovieID, err := pbFindFirstIDByField(client, pbBaseURL, token, "movies", "imdb_id", imdbID)
				if err != nil {
					return err
				}

				contentID := ""
				if existingMovieID != "" {
					rec, err := pbGetRecord(client, pbBaseURL, token, "movies", existingMovieID)
					if err != nil {
						return err
					}
					if v, ok := rec["content_id"]; ok {
						if s, ok := v.(string); ok {
							contentID = s
						}
					}
				}

				contentPayload := map[string]any{}
				if m.PrimaryImage != nil {
					contentPayload["poster_url"] = strings.TrimSpace(m.PrimaryImage.URL)
					contentPayload["poster_width"] = m.PrimaryImage.Width
					contentPayload["poster_height"] = m.PrimaryImage.Height
				}
				if m.PrimaryVideo != nil {
					contentPayload["vidsrc_url"] = strings.TrimSpace(m.PrimaryVideo.VidsrcURL)
					contentPayload["vidlink_url"] = strings.TrimSpace(m.PrimaryVideo.VidlinkProURL)
					contentPayload["autoembed_url"] = strings.TrimSpace(m.PrimaryVideo.AutoembedURL)
					contentPayload["gomo_url"] = strings.TrimSpace(m.PrimaryVideo.GomoURL)
					contentPayload["moviesapi_url"] = strings.TrimSpace(m.PrimaryVideo.MoviesAPIURL)
				}

				if contentID == "" {
					cid, err := pbCreateRecord(client, pbBaseURL, token, "contents", contentPayload)
					if err != nil {
						return err
					}
					contentID = cid
				} else {
					if err := pbUpdateRecord(client, pbBaseURL, token, "contents", contentID, contentPayload); err != nil {
						return err
					}
				}

				moviePayload := map[string]any{
					"imdb_id":       imdbID,
					"tmdb_id":       tmdbID,
					"content_id":    contentID,
					"title":         strings.TrimSpace(m.Title),
					"plot":          strings.TrimSpace(m.Plot),
					"type":          movieType,
					"quality":       strings.TrimSpace(m.Quality),
					"released_year": m.StartYear,
					"duration":      m.RuntimeSeconds,
				}
				if m.Rating != nil {
					moviePayload["imdb_rating"] = m.Rating.AggregateRating
					moviePayload["vote_count"] = m.Rating.VoteCount
				}
				if len(genreIDs) > 0 {
					moviePayload["genre_id"] = genreIDs
				}
				if len(countryIDs) > 0 {
					moviePayload["country_id"] = countryIDs
				}

				if existingMovieID == "" {
					mid, err := pbCreateRecord(client, pbBaseURL, token, "movies", moviePayload)
					if err != nil {
						return err
					}
					existingMovieID = mid
				} else {
					if err := pbUpdateRecord(client, pbBaseURL, token, "movies", existingMovieID, moviePayload); err != nil {
						return err
					}
				}

				upsertPeople := func(p moviePersonJSON, role string) error {
					pid := strings.TrimSpace(p.ID)
					if pid == "" {
						return nil
					}
					incomingProfessions := normalizeProfessions(role, p.PrimaryProfessions)

					existingID, err := pbFindFirstIDByField(client, pbBaseURL, token, "people", "imdb_id", pid)
					if err != nil {
						return err
					}

					var existingRec pbRecordAnyResp
					if existingID != "" {
						existingRec, err = pbGetRecord(client, pbBaseURL, token, "people", existingID)
						if err != nil {
							return err
						}
					}

					professions := incomingProfessions
					if existingRec != nil {
						v, ok := existingRec["professions"]
						if !ok {
							v, ok = existingRec["profession"]
						}
						if ok {
							set := map[string]struct{}{}
							switch vv := v.(type) {
							case []any:
								for _, it := range vv {
									if s, ok := it.(string); ok {
										if ns := normalizeProfessionValue(s); ns != "" {
											set[ns] = struct{}{}
										}
									}
								}
							case string:
								if strings.TrimSpace(vv) != "" {
									if ns := normalizeProfessionValue(vv); ns != "" {
										set[ns] = struct{}{}
									}
								}
							}
							for _, p := range incomingProfessions {
								if p != "" {
									set[p] = struct{}{}
								}
							}
							if len(set) == 0 {
								set["assistant"] = struct{}{}
							}
							professions = professions[:0]
							for k := range set {
								professions = append(professions, k)
							}
							sort.Strings(professions)
							if len(professions) > 3 {
								professions = professions[:3]
							}
						}
					}

					movieIDsSet := map[string]struct{}{}
					if strings.TrimSpace(existingMovieID) != "" {
						movieIDsSet[existingMovieID] = struct{}{}
					}
					if existingRec != nil {
						if v, ok := existingRec["movie_id"]; ok {
							switch vv := v.(type) {
							case []any:
								for _, it := range vv {
									if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
										movieIDsSet[s] = struct{}{}
									}
								}
							case string:
								if strings.TrimSpace(vv) != "" {
									movieIDsSet[vv] = struct{}{}
								}
							}
						}
					}

					movieIDs := make([]string, 0, len(movieIDsSet))
					for k := range movieIDsSet {
						movieIDs = append(movieIDs, k)
					}
					sort.Strings(movieIDs)

					payload := map[string]any{
						"imdb_id":     pid,
						"movie_id":    movieIDs,
						"name":        strings.TrimSpace(p.DisplayName),
						"professions": professions,
					}
					if p.PrimaryImage != nil {
						payload["img_url"] = strings.TrimSpace(p.PrimaryImage.URL)
						payload["img_width"] = p.PrimaryImage.Width
						payload["img_height"] = p.PrimaryImage.Height
					}

					if existingID == "" {
						_, err := pbCreateRecord(client, pbBaseURL, token, "people", payload)
						if err == nil {
							return nil
						}
						// fallback for schemas where movie_id maxSelect=1
						payload["movie_id"] = existingMovieID
						_, err2 := pbCreateRecord(client, pbBaseURL, token, "people", payload)
						return err2
					}

					if err := pbUpdateRecord(client, pbBaseURL, token, "people", existingID, payload); err == nil {
						return nil
					}
					// fallback for schemas where movie_id maxSelect=1
					payload["movie_id"] = existingMovieID
					return pbUpdateRecord(client, pbBaseURL, token, "people", existingID, payload)
				}

				for _, p := range m.Directors {
					if err := upsertPeople(p, "director"); err != nil {
						return err
					}
				}
				for _, p := range m.Writers {
					if err := upsertPeople(p, "writer"); err != nil {
						return err
					}
				}
				for _, p := range m.Stars {
					if err := upsertPeople(p, "actor"); err != nil {
						return err
					}
				}

				return nil
			}

			for w := 0; w < workers; w++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for job := range jobs {
						err := retry(retries, 250*time.Millisecond, func() error {
							return importOne(job)
						})
						if err != nil {
							if maxAttempts == 0 || job.Attempt < maxAttempts {
								job.Attempt++
								select {
								case jobs <- job:
								default:
									go func(j movieJob) {
										jobs <- j
									}(job)
								}
								continue
							}
							atomic.AddInt64(&failed, 1)
							jobWG.Done()
							continue
						}
						atomic.AddInt64(&succeeded, 1)
						jobWG.Done()
					}
				}()
			}

			go func() {
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-stopProgress:
						return
					case <-ticker.C:
						completed := int(atomic.LoadInt64(&succeeded) + atomic.LoadInt64(&failed))
						fmt.Printf("\r%s", renderProgress(completed, total, start))
					}
				}
			}()

			for _, p := range paths {
				base := strings.ToLower(filepath.Base(p))
				kind := "movie"
				if strings.HasPrefix(base, "series") {
					kind = "serie"
				}

				f, err := os.Open(p)
				if err != nil {
					close(jobs)
					wg.Wait()
					return err
				}

				dec := json.NewDecoder(f)
				tok, err := dec.Token()
				if err != nil {
					f.Close()
					close(jobs)
					wg.Wait()
					return err
				}
				if d, ok := tok.(json.Delim); !ok || d != '[' {
					f.Close()
					close(jobs)
					wg.Wait()
					return fmt.Errorf("expected JSON array in %s", p)
				}

				for dec.More() {
					var item movieJSON
					if err := dec.Decode(&item); err != nil {
						f.Close()
						close(jobs)
						close(stopProgress)
						wg.Wait()
						return err
					}
					jobWG.Add(1)
					jobs <- movieJob{Kind: kind, Item: item, Attempt: 1}
				}

				_, err = dec.Token()
				f.Close()
				if err != nil {
					close(jobs)
					close(stopProgress)
					wg.Wait()
					return err
				}
			}

			jobWG.Wait()
			close(jobs)
			wg.Wait()
			close(stopProgress)

			completed := int(atomic.LoadInt64(&succeeded) + atomic.LoadInt64(&failed))
			fmt.Printf("\r%s\n", renderProgress(completed, total, start))
			if f := atomic.LoadInt64(&failed); f > 0 {
				return fmt.Errorf("movies import completed with %d failed items (increase --max-attempts or inspect data)", f)
			}
			fmt.Println("Done")
			return nil
		},
	}

	moviesCmd.Flags().String("pb-url", envOrDefault("PB_URL", "http://127.0.0.1:8090"), "PocketBase base URL")
	moviesCmd.Flags().String("email", envOrDefault("PB_SUPERUSER_EMAIL", ""), "PocketBase superuser email")
	moviesCmd.Flags().String("password", envOrDefault("PB_SUPERUSER_PASSWORD", ""), "PocketBase superuser password")
	moviesCmd.Flags().String("json-dir", envOrDefault("PB_MOVIES_DIR", "../pkg/json"), "Directory with movies*.json and series*.json")
	moviesCmd.Flags().Int("workers", 10, "Number of concurrent workers (max 20)")
	moviesCmd.Flags().Int("retries", 5, "Retries per item")
	moviesCmd.Flags().Int("max-attempts", 0, "Max attempts per item before giving up (0 = infinite)")

	app.RootCmd.AddCommand(moviesCmd)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
