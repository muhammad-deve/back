# Telegram Movie & TV Series Bot - Requirements

## Tech Stack
- Language: Go
- Database: PocketBase (all data storage)
- External APIs: Telegram Bot API, Gemini API
- Environment: BOT_TOKEN, GEMINI_API_KEY (.env file)

## Core Features

### User Registration & Verification
- `/start` command checks if user exists in PocketBase
- New users must share Telegram contact (mandatory, block access if refused)
- Store: telegram_id, username, phone_number, registration_date, last_activity, is_blocked
- Returning users skip verification

### Mandatory Channel Subscription
- After registration, check subscription to all active channels from PocketBase (collection: mandatory_channels)
- Display channels as inline URL buttons + "✅ Check Subscription" button
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