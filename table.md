# Unified Streaming Platform - PocketBase Schema

## Overview
A streaming platform with 100K+ movies and 3K+ TV channels

---

## 📺 TV CHANNELS TABLES

### `countries`
```
├─ id (uuid, primary)
├─ code (text, unique) - "UZ", "US", "MX"
├─ name (text) - "Uzbekistan", "United States"
└─ language (text) - "Uzbek", "English"
```

### `channel_categories`
```
├─ id (uuid, primary)
└─ name (text, unique) - "sports", "news", "entertainment"
```

### `channels`
```
├─ id (uuid, primary)
├─ title (text, required)
├─ website (text)
├─ stream_url (text)
├─ logo_url (text)
├─ quality (text) - "1080p", "720p"
├─ is_url_working (boolean)
├─ is_logo_available (boolean)
├─ country (relation → countries.id)
└─ category (relation → channel_categories.id)
```

---

## 🎬 MOVIES TABLES

### `movie_genres`
```
├─ id (uuid, primary)
└─ name (text, unique) - "Documentary", "Drama", "Sci-Fi"
```

### `movie_types`
```
├─ id (uuid, primary)
└─ name (text, unique) - "movie", "tvMovie", "short", "series"
```

### `movies`
```
├─ id (uuid, primary)
├─ imdb_id (text, unique, indexed)
├─ tmdb_id (text, unique, indexed)
├─ title (text, required, indexed)
├─ plot (text)
├─ type (relation → movie_types.id)
├─ quality (text) - "1080p", "720p", "4K"
├─ start_year (number)
├─ runtime_seconds (number)
├─ rating (number) - Store as decimal (7.9)
├─ vote_count (number)
├─ poster_url (text) - primaryImage.url
├─ poster_width (number)
├─ poster_height (number)
├─ vidsrc_url (text)
├─ vidlink_pro_url (text)
├─ autoembed_url (text)
├─ gomo_url (text)
└─ moviesapi_url (text)
```

### `movie_genres_junction` (Many-to-Many)
```
├─ id (uuid, primary)
├─ movie (relation → movies.id)
└─ genre (relation → movie_genres.id)
```
**Unique constraint:** (movie, genre)

### `movie_countries` (Many-to-Many)
```
├─ id (uuid, primary)
├─ movie (relation → movies.id)
└─ country (relation → countries.id)
```
**Unique constraint:** (movie, country)

### `movie_languages`
```
├─ id (uuid, primary)
├─ code (text, unique) - "eng", "spa", "uzb"
└─ name (text) - "English", "Spanish", "Uzbek"
```

### `movie_spoken_languages` (Many-to-Many)
```
├─ id (uuid, primary)
├─ movie (relation → movies.id)
└─ language (relation → movie_languages.id)
```
**Unique constraint:** (movie, language)

### `people` (Directors, Writers, Stars)
```
├─ id (uuid, primary)
├─ imdb_id (text, unique, indexed) - "nm0220058"
├─ display_name (text, indexed)
├─ profile_image_url (text)
├─ profile_image_width (number)
└─ profile_image_height (number)
```

### `movie_directors` (Many-to-Many)
```
├─ id (uuid, primary)
├─ movie (relation → movies.id)
└─ person (relation → people.id)
```
**Unique constraint:** (movie, person)

### `movie_writers` (Many-to-Many)
```
├─ id (uuid, primary)
├─ movie (relation → movies.id)
└─ person (relation → people.id)
```
**Unique constraint:** (movie, person)

### `movie_stars` (Many-to-Many)
```
├─ id (uuid, primary)
├─ movie (relation → movies.id)
├─ person (relation → people.id)
└─ order (number) - Display order (1, 2, 3, 4)
```
**Unique constraint:** (movie, person)

---

## 👤 USER & PLATFORM TABLES

### `users` (Built-in PocketBase)
```
├─ id (uuid, primary)
├─ email (text, unique)
├─ username (text, unique)
├─ verified (boolean)
└─ avatar (file)
```

### `watchlist`
```
├─ id (uuid, primary)
├─ user (relation → users.id)
├─ movie (relation → movies.id, optional)
├─ channel (relation → channels.id, optional)
└─ added_at (datetime, auto)
```
**Unique constraint:** (user, movie) OR (user, channel)
**Note:** Either movie OR channel must be set, not both

### `watch_history`
```
├─ id (uuid, primary)
├─ user (relation → users.id)
├─ movie (relation → movies.id, optional)
├─ channel (relation → channels.id, optional)
├─ watched_at (datetime, auto)
└─ progress_seconds (number) - For resuming
```

### `favorites`
```
├─ id (uuid, primary)
├─ user (relation → users.id)
├─ movie (relation → movies.id, optional)
└─ channel (relation → channels.id, optional)
```
**Unique constraint:** (user, movie) OR (user, channel)

---

## 📊 DATABASE STATISTICS

### Total Tables: 18
- **Channels:** 3 tables
- **Movies:** 11 tables
- **Users/Platform:** 4 tables

### Expected Data Volume:
- Movies: ~100,000
- Channels: ~3,000
- Genres: ~30
- People: ~50,000+
- Countries: ~200

---

## 🔍 KEY INDEXES TO CREATE

### Movies
```sql
CREATE INDEX idx_movies_imdb_id ON movies(imdb_id);
CREATE INDEX idx_movies_tmdb_id ON movies(tmdb_id);
CREATE INDEX idx_movies_title ON movies(title);
CREATE INDEX idx_movies_start_year ON movies(start_year);
CREATE INDEX idx_movies_rating ON movies(rating);
```

### Channels
```sql
CREATE INDEX idx_channels_title ON channels(title);
CREATE INDEX idx_channels_country ON channels(country);
CREATE INDEX idx_channels_is_working ON channels(is_url_working);
```

### People
```sql
CREATE INDEX idx_people_imdb_id ON people(imdb_id);
CREATE INDEX idx_people_display_name ON people(display_name);
```

### User Activity
```sql
CREATE INDEX idx_watchlist_user ON watchlist(user);
CREATE INDEX idx_watch_history_user ON watch_history(user);
CREATE INDEX idx_favorites_user ON favorites(user);
```

---

## 🎯 SCHEMA BENEFITS

✅ **Normalized:** No data duplication  
✅ **Shared Resources:** Countries table used by both movies and channels  
✅ **Flexible:** Easy to add TV series episodes later  
✅ **Scalable:** Handles 100K+ movies easily  
✅ **User-Friendly:** Simple watchlist, history, favorites  
✅ **Fast Queries:** Proper indexes on all important fields  

---

## 📝 NOTES

1. **Shared `countries` table** - Used by both channels and movies
2. **Separate categories** - `channel_categories` vs `movie_genres` (different purposes)
3. **Multiple video sources** - Movies have 5 different embed URLs
4. **People reuse** - Same person can be director, writer, star across multiple movies
5. **Quality field** - Consistent across movies and channels ("1080p", "720p", etc.)
6. **User activity** - Watchlist/History/Favorites can contain either movies OR channels

---

## 🚀 NEXT STEPS

1. Create collections in PocketBase
2. Set up relations between tables
3. Create indexes for performance
4. Import 100K movies + 3K channels
5. Build API endpoints for:
   - Movie search/filter
   - Channel browsing
   - User watchlist/history
   - Recommendations

---

## 🔗 RELATIONSHIP SUMMARY

```
countries (shared)
  ├─→ channels (one-to-many)
  └─→ movies (many-to-many via movie_countries)

movies
  ├─→ movie_genres (many-to-many)
  ├─→ movie_countries (many-to-many)
  ├─→ movie_spoken_languages (many-to-many)
  ├─→ movie_directors (many-to-many)
  ├─→ movie_writers (many-to-many)
  └─→ movie_stars (many-to-many)

users
  ├─→ watchlist (can contain movies OR channels)
  ├─→ watch_history (can contain movies OR channels)
  └─→ favorites (can contain movies OR channels)
```