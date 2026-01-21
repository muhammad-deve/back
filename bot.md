# Telegram Movie & TV Series Bot - Requirements

• Build a Telegram bot in Go that allows users to stream movies and TV series
• Bot language: English only
• Use Telegram Bot API with inline keyboards
• All data stored in PocketBase (users, movies, series, channels, admins, watch history, temporary files)
• Environment variables: BOT_TOKEN, GEMINI_API_KEY (from .env file)

## User Registration & Verification

• When user sends /start command, check if they exist in PocketBase
• New users must share their Telegram contact (mandatory step)
• If user refuses contact sharing, block all access
• Store: telegram_id, username, phone_number, registration_date, last_activity, is_blocked status
• Returning users skip contact verification

## Mandatory Channel Subscription

• After contact verification, retrieve active mandatory channels from PocketBase
• Display each channel as inline URL button
• Add "✅ Check Subscription" button at bottom
• When clicked, verify user is subscribed to ALL channels using Telegram API
• If not subscribed to all channels, display error listing missing subscriptions and block access
• If all verified, proceed to main menu
• Admins completely skip subscription check and go straight to main menu
• PocketBase collection: mandatory_channels (fields: channel_id, channel_name, channel_url, is_active, created_at)

## Main Menu

• Display three buttons after verification passes:
• Row 1: "🎬 Movies" and "📺 TV Series" (side by side, equal width)
• Row 2: "🔍 Search TV Channels" (full width)

## Movies Feature

• When "🎬 Movies" clicked, ask user to provide: IMDb ID (tt1234567), TMDB ID (12345), or Movie Name
• Parse input to determine type
• Query PocketBase for movie metadata using the provided ID/name
• If not found, display error and return to main menu
• If found, construct VidSrc embed URL based on IMDb/TMDB ID
• Execute video download pipeline using the provided Python code (convert to Go):
  - Extract iframe URL from embed page by parsing HTML for <iframe id="player_iframe">
  - Use headless Chrome (chromedp library) to navigate to iframe URL
  - Wait for video player, click play button if present
  - Capture network traffic to extract .m3u8 or .mp4 stream URLs
  - Filter out ads and trackers
  - Download stream using ffmpeg with proper headers (Referer, User-Agent)
  - Implement retry logic with 3 different configurations
  - Save to ./movies/ directory
• Convert downloaded video to HLS format using FFmpeg:
  - Command: ffmpeg -i input.mp4 -c:v libx264 -preset ultrafast -crf 28 -c:a copy -hls_time 6 -hls_list_size 0 -hls_segment_filename ./hls/{video_id}/segment_%03d.ts -hls_base_url {video_id}/ -y ./hls/{video_id}.m3u8
  - Output: hls/{movie_id}.m3u8 and hls/{movie_id}/segment_*.ts files
• Generate temporary streaming URL valid for exactly 5 hours
• URL format: https://your-domain.com/stream/{token}
• Token = base64(encrypt(content_id + expiry_timestamp + random_salt))
• Send message with: movie poster, title, description, release year, genre, rating, streaming link
• Add inline button: "▶️ Watch Now"
• Increment movie watch counter in PocketBase (global and per-user)
• Log to watch_history: user_id, content_type='movie', content_id, watched_at timestamp

## TV Series Feature

• When "📺 TV Series" clicked, ask for: IMDb ID (tt0903747), TMDB ID (1399), or Series Name
• Query PocketBase for series metadata
• If found, send: series poster, title, description
• Display inline button: "Select Season ⬇️"
• When clicked, display all available seasons as inline buttons (Season 1, Season 2, Season 3, etc.)
• When season selected, display all episodes for that season: "S01E01 - Episode Title", "S01E02 - Episode Title", etc.
• Add "◀️ Back to Seasons" button at bottom
• When episode selected:
  - Download episode using same video pipeline as movies
  - Convert to HLS format
  - Generate 5-hour temporary URL
  - Send episode with metadata and streaming link
• Increment episode watch counter in PocketBase (global and per-user)
• Log to watch_history: user_id, content_type='episode', content_id, watched_at

## Admin System

• /admin command only accessible to users in PocketBase admins collection
• Non-admins get "⛔ Unauthorized" or silently ignored
• PocketBase admins collection fields: telegram_id, username, added_at, added_by

## Admin Panel Menu

• Display 4 buttons when /admin pressed:
  - 📢 Mandatory Channels
  - 📤 Send Global Message
  - 👥 Users Management
  - 📊 Statistics

## Mandatory Channels Management

• Add Channel: Request channel_id, channel_name, channel_url → validate bot is admin → save to PocketBase with is_active=true
• View Channels: List all with buttons [❌ Remove] [⚙️ Edit] [🔄 Toggle]
• Remove Channel: Delete from PocketBase or set is_active=false
• Edit Channel: Update name, URL, settings in PocketBase
• Toggle: Enable/disable without deleting
• All changes immediately affect user verification flow

## Send Global Message (Broadcast)

• Support text-only or image+caption
• Request message content, show preview, confirm
• Implementation:
  - Retrieve all active (non-blocked) user IDs from PocketBase
  - Iterate with rate limiting: sleep 50ms between sends (Telegram limit: 30 msg/sec)
  - Handle errors (blocked bot, deleted accounts)
  - Display progress: "Sent: 1234/5000 (24.68%)"
  - Final report: "✅ Sent to 4950 users, ❌ Failed: 50"
  - Update user status in PocketBase if bot was blocked

## Users Management

• Export Users: Generate .xlsx file from PocketBase with columns: User ID, Username, Phone Number, Registration Date, Movies Watched, Episodes Watched, Status
• Send file to admin with caption: "📊 User Database Export - [Date]"
• Get User by ID: /getuser <user_id> retrieves from PocketBase and displays:
  - User profile: ID, username, phone, registration date
  - Activity stats: movies watched, episodes watched, last active
  - Status: Active/Blocked
  - Buttons: [🚫 Block User] [💬 Send Message]
• Block User: Confirm action → update is_blocked=true in PocketBase → user cannot access bot
• Unblock: Set is_blocked=false
• Send Direct Message: Request text → send from bot to user → confirm delivery

## Statistics Dashboard

• Display global stats from PocketBase:
  - Total Users
  - Total Movies Watched
  - Total Episodes Watched
  - Today's Activity (views)
  - Last Updated timestamp
• Top Movie Watchers: List top 10 users by movie watch count
• Top Series Watchers: List top 10 users by episode watch count
• Optional: Top movies by view count, daily/weekly usage stats

## Temporary File Management

• Store in PocketBase temporary_files collection: file_path, content_id, content_type (movie/episode), created_at, expires_at (created_at + 5 hours), is_deleted
• URL validation: decrypt token → check expiry → verify content_id exists → serve HLS playlist
• Cron job runs every 30 minutes:
  - Query PocketBase for expired files (expires_at < now)
  - Delete physical files from ./hls/ directory
  - Update is_deleted=true in PocketBase
  - Log cleanup statistics

## Video Download Pipeline (Python to Go Conversion)

• Input: VidSrc embed URL (e.g., https://vidsrc-embed.ru/embed/tt0460649/1-1)
• Step 1 - Extract iframe: Parse HTML → find <iframe id="player_iframe"> → extract src attribute
• Step 2 - Browser automation using chromedp:
  - Launch headless Chrome
  - Navigate to iframe URL
  - Wait for video player to load (5 seconds)
  - Find and click play button if exists (#pl_but, .play-button, button[aria-label='Play'])
  - Switch to nested iframes if present
  - Wait for streams to load (5 seconds)
  - Capture network logs (performance logging)
  - Extract URLs containing: .m3u8, .mp4, .ts, /manifest, /playlist
  - Filter out: google, doubleclick, facebook, analytics, ads, trackers
  - Also check video elements directly for src attribute
• Step 3 - Download stream using ffmpeg (exec.Command):
  - Try 3 configurations with different headers/settings
  - Config 1: Standard with referer and origin headers
  - Config 2: HLS optimized with hls_prefer_native
  - Config 3: Permissive with nocheckcertificate
  - Use retries=10, fragment_retries=10
  - Save to movies/ directory
• Required libraries: chromedp (browser automation), ffmpeg (stream download)

## HLS Conversion

• Use FFmpeg to convert downloaded video to HLS streaming format
• Command: ffmpeg -i input.mp4 -c:v libx264 -preset ultrafast -crf 28 -c:a copy -hls_time 6 -hls_list_size 0 -hls_segment_filename ./hls/{video_id}/segment_%03d.ts -hls_base_url {video_id}/ -y ./hls/{video_id}.m3u8
• Show progress bar during conversion with percentage and ETA
• Output structure: hls/{video_id}.m3u8 (master playlist) and hls/{video_id}/segment_*.ts (chunks)
• Video format must be playable in all browsers/devices

## Error Handling

• Invalid IMDb/TMDB ID: "❌ Invalid ID format. Please try again."
• Movie not found: "❌ Movie not found in database. Please check the ID or try a different title."
• Video download failed: "❌ Unable to download this content. Please try again later or contact support."
• Not subscribed: "⚠️ You must subscribe to all channels to continue:" + list channels
• PocketBase API failed: Retry 3 times, then notify admin
• FFmpeg conversion failed: Log error, notify admin, send user generic error message
• Telegram API rate limit: Implement exponential backoff
• Disk space full: Alert admin, pause downloads

## Security

• Validate all user inputs before processing
• Validate all file paths to prevent path traversal attacks
• Rate limiting: Max 10 movie requests per user per hour
• Always verify admin status before sensitive operations
• Use proper authentication for all PocketBase API calls
• Never log sensitive data (phone numbers, tokens)

## Implementation Notes

• Use structured logging (logrus or zap)
• All hardcoded values must be configurable
• Keep functions modular and focused
• Use goroutines for concurrent operations (broadcast, file cleanup)
• Follow Go best practices and conventions
• All data persistence through PocketBase API
• Chrome/ChromeDriver must be installed for video extraction
• FFmpeg must be installed for video conversion and download

## Testing

• Unit tests: Input validation, PocketBase API integration, token generation/validation
• Integration tests: Complete user registration flow, movie search and delivery, admin channel management, broadcast messaging
• Load tests: 100 concurrent streaming users, 1000 users receiving broadcast, PocketBase API under load

## Optional Future Enhancements

• Search history per user
• Favorites/watchlist feature
• Resume watching from last position
• Multiple video quality options
• Subtitle support
• Download limit per user (premium vs free tiers)
• Payment integration
• Content recommendations based on watch history
• Multi-language support
• Mobile app integration