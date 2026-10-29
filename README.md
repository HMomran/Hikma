# Daily Hikmah Notifications - حكمة اليوم

A production-ready full-stack web application that sends Arabic wisdom notifications using **Web Push API** and **Service Workers**. Built with Go backend (webpush-go library) and Vanilla JavaScript frontend.

**🚀 [View Production Deployment Guide →](DEPLOYMENT.md)**

## Gemini setup

The backend uses Google's official Gemini `generateContent` REST API. Kimi/Moonshot is no longer used; no new Go dependencies are needed. `GEMINI_API_KEY` stays on the backend and is sent in the `x-goog-api-key` header, never in URLs, frontend code, cache, or logs.

The default model is `gemini-2.5-flash`. Google lists free-tier input/output pricing, but currently restricts 2.5 access to accounts that previously used the model. Your account access cannot be checked without your own Gemini key. If unavailable, select a free-tier text model offered to your project with `GEMINI_MODEL`, for example `gemini-3.5-flash-lite`, which Google also lists with free-tier text generation. Quotas depend on your project. Keep the project on the Free tier for free-tier-only use; the application does not enforce billing settings.

Official references: [2.5 Flash availability](https://ai.google.dev/gemini-api/docs/models/gemini-2.5-flash), [pricing](https://ai.google.dev/gemini-api/docs/pricing), [REST API](https://ai.google.dev/api/generate-content).

### Configure locally

1. Create a Gemini API key in [Google AI Studio](https://aistudio.google.com/apikey).
2. From the project root, run `cd backend`. If `.env` does not exist, run `cp .env.example .env`. Preserve existing VAPID values so push subscriptions keep using the same keys.
3. Edit the ignored `.env`, set `GEMINI_API_KEY=your_key`, and remove obsolete `KIMI_API_KEY`, `KIMI_MODEL`, entries. Optionally configure `GEMINI_MODEL`, `HIKMAH_TIMEZONE` (default `Asia/Bahrain`), and `HIKMAH_CACHE_FILE` (default `.cache/daily-hikmah.json`).
4. Stop the older backend on port 8080, then start:

```sh
set -a
source .env
set +a
go run .
```

The Go server does not load `.env` automatically. Open `http://localhost:8080`. The frontend design, subscription flow, VAPID configuration, and notification frequencies are preserved. On Render, set the same environment variables and build with `go build -o app .` from `backend`.

### Daily wisdom and fallback

The first page view or due notification each calendar day requests one short Arabic reflection about patience, kindness, morality, knowledge, personal growth, or life. All consumers share it until midnight in `HIKMAH_TIMEZONE`. Generation is lazy, on the first request after midnight. AI text is prefixed `تأمل مولّد بالذكاء الاصطناعي:` in both the page and notification payloads.

If the key is missing, the request times out, quota is exhausted, the model is inaccessible, or output is invalid/blocked, the service selects a deterministic daily entry from the unchanged `backend/data/hikmah.json`. Fallback text is prefixed `من المجموعة المحلية (دون نسبة موثّقة):`. This fallback lasts the rest of the day; no automatic retry loop consumes quota. The local JSON is embedded into the binary for reliable deployment.

Both generated and fallback results are cached in memory and saved atomically to the ignored cache file. Retaining that file preserves the day's selection across restarts. Use durable storage for this path in production. If saving fails, memory caching remains active, but restarting may make another API request. Multiple independent backend instances need a shared database and lock to enforce one global daily selection. Changing models or adding a key does not replace an already cached day's selection.

AI reflections are never added to the historical collection. The prompt forbids attribution to Imam Ali (عليه السلام), Ahlulbayt, or religious sources, and validation rejects attribution markers. The existing collection contains no citations and is not claimed to be independently verified. Any verified historical quotations must remain separately curated with their sources; this integration neither verifies nor overwrites them.

### Test locally

From `backend`:

```sh
go test -race ./...
go vet ./...
curl -fsS http://localhost:8080/api/hikmah
```

Automated tests require no real key and use local mock Gemini servers. They cover request format/authentication, Arabic output, attribution rejection, concurrent caching, Bahrain midnight rollover, restart persistence, missing keys, quota/auth/model/service errors, blocked/malformed/truncated output, timeouts, fallback, and matching website/notification content.

For a live Gemini check, stop the test backend, export your real configuration as above, and use a fresh test cache:

```sh
HIKMAH_CACHE_FILE=.cache/gemini-live-test.json PORT=8081 go run .
```

In another terminal run `curl -fsS http://localhost:8081/api/hikmah` twice. With available model access/quota, `message` begins with the AI label; both results match and `nextRotation` is next local midnight. A local label means generation failed: inspect sanitized backend status logs. HTTP 403 indicates access/key restrictions, 404 can indicate model availability, and 429 indicates quota exhaustion. A live call uses your project's quota.

To test local fallback manually without contacting Gemini, stop the test server and run:

```sh
GEMINI_API_KEY= HIKMAH_CACHE_FILE=.cache/fallback-test.json PORT=8081 go run .
```

Request that endpoint twice: both responses should be HTTP 200 with the same local wisdom. To repeat selection on the same day, stop the test server and remove only its test cache file before restarting. Using a fresh filename also avoids reusing an earlier fallback. Live account generation has not been verified without your Gemini key.

---

## ✨ Features

### Push Notifications
✅ **Real Web Push Notifications** - Works even when browser is closed  
✅ **VAPID Authentication** - Secure push notification delivery  
✅ **Service Worker** - Background notification handling with RTL support  
✅ **Frequency Selection** - Users choose 6h, 12h, or 24h intervals  

### Content & UX
✅ **AI-generated Arabic Wisdom** - Generated through the Gemini API
✅ **Cached Wisdom** - Shared across page views and push notifications
✅ **Subscribe/Unsubscribe** - Full subscription management  
✅ **Smart UI** - Auto-hides instructions when subscribed  
✅ **Modern Design** - Responsive gradient UI with RTL support  

### Production Ready
✅ **Environment Variables** - VAPID keys and PORT configuration  
✅ **CORS Support** - Cross-origin requests for Vercel frontend  
✅ **Render Compatible** - Dynamic port assignment  
✅ **Vercel Compatible** - Optimized static frontend deployment  
✅ **Health Monitoring** - `/health` endpoint for status checks  
✅ **Error Handling** - Automatic removal of invalid subscriptions  

---

## 📁 Project Structure

```
Ali/
├── main.go                    # Original backend (local development)
├── main-production.go         # Production backend with env vars & CORS
├── generate_vapid_keys.go     # VAPID key generator utility
├── go.mod                     # Dependencies (webpush-go v1.4.0)
├── go.sum                     # Dependency checksums
│
├── data/
│   └── hikmah.json           # 99 unique Arabic wisdom sayings
│
├── frontend/                  # Vercel-ready frontend
│   ├── index.html            # UI with frequency selection
│   ├── script.js             # Push subscription with configurable API_URL
│   ├── sw.js                 # Service Worker with RTL support
│   ├── style.css             # Modern RTL gradient styling
│   └── vercel.json           # Vercel deployment configuration
│
├── static/                    # Original static files (local dev)
│   ├── index.html
│   ├── script.js
│   ├── sw.js
│   └── style.css
│
├── .env.example              # Environment variable template
├── .gitignore                # Git ignore file (includes vapid_keys.json)
├── README.md                 # This file
└── DEPLOYMENT.md             # Complete deployment guide
```

---

## 🏗️ Architecture

### Backend (Go) - Production Version

**File**: `main-production.go`

**Key Features:**
- ✅ **Environment Variables**: Loads VAPID keys from `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY`
- ✅ **Dynamic Port**: Uses `PORT` environment variable (required by Render)
- ✅ **CORS Middleware**: Allows cross-origin requests from Vercel frontend
- ✅ **Frequency Support**: Each subscriber can choose 6h, 12h, or 24h intervals
- ✅ **Smart Scheduling**: Checks each subscriber individually and sends based on their preference
- ✅ **Auto-cleanup**: Removes invalid subscriptions (410 Gone, 404 Not Found)
- ✅ **Health Endpoint**: `/health` for monitoring subscriber count and status

**API Endpoints:**
```
# Primary endpoints (recommended for production)
POST   /api/subscribe          - Subscribe with frequency preference (6h/12h/24h)
GET    /api/vapid-public-key   - Get VAPID public key for frontend
GET    /api/hikmah             - Get current wisdom saying
GET    /api/health             - Health check endpoint (returns subscriber count)

# Legacy endpoints (backwards compatibility)
POST   /subscribe              - Same as /api/subscribe
GET    /vapid-public-key       - Same as /api/vapid-public-key
GET    /today                  - Same as /api/hikmah
GET    /health                 - Same as /api/health
```

**Data Flow:**
1. Loads VAPID keys from environment or generates new ones
2. Generates Arabic wisdom through the AI API and caches it for the configured interval
3. Runs a scheduler every minute checking each subscriber
4. Sends notification if subscriber's frequency interval has passed
5. Rotates wisdom counter after each notification

### Frontend (JavaScript) - Vercel Ready

**Location**: `frontend/` folder

**Key Features:**
- ✅ **Configurable API URL**: Automatically detects localhost vs production
- ✅ **Frequency Selection UI**: Radio buttons for 6h, 12h, 24h options
- ✅ **Smart Subscription**: Sends both subscription and frequency preference
- ✅ **Subscribe/Unsubscribe**: Full lifecycle management
- ✅ **Auto-hiding UI**: Instructions disappear when subscribed
- ✅ **RTL Support**: Proper Arabic text rendering throughout

### Service Worker (`sw.js`)

Runs in background, independent of browser state:
- 📬 Receives push notifications from browser push service
- 📱 Displays notifications with RTL Arabic text
- 🔔 Handles notification clicks (opens/focuses app)
- 🔄 Manages push subscription changes
- ✅ Works even when browser is closed or tab is inactive

---

## 🚀 Quick Start (Local Development)

### Prerequisites
- Go 1.16+ installed
- Modern web browser (Chrome, Firefox, Edge, Safari 16+)

### Local Development Setup

1. **Clone and navigate**:
```bash
cd /Users/hassanomran/div/Ali
```

2. **Install dependencies**:
```bash
go mod download
```

3. **Run the backend**:
```bash
go run .
```

4. **Open in browser**:
```
http://localhost:8080
```

5. **Test subscription**:
   - Select frequency (6h, 12h, or 24h)
   - Click "اشترك الآن" (Subscribe Now)
   - Allow notifications when prompted
   - Receive immediate welcome notification

---

## 🌐 Production Deployment

For deploying to **Render (backend)** and **Vercel (frontend)**:

### 📖 [Follow the Complete Deployment Guide →](DEPLOYMENT.md)

### Quick Production Checklist

- [ ] Generate VAPID keys: `go run generate_vapid_keys.go`
- [ ] Rename `main-production.go` to `main.go`
- [ ] Push backend to GitHub → Deploy on Render
- [ ] Set `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` on Render
- [ ] Update `API_URL` in `frontend/script.js` with Render URL
- [ ] Push frontend to GitHub → Deploy on Vercel
- [ ] Test notifications on production URL

### Environment Variables (Production)

On Render, set these environment variables:

```bash
VAPID_PUBLIC_KEY=<your_generated_public_key>
VAPID_PRIVATE_KEY=<your_generated_private_key>
PORT=<automatically_set_by_render>
```

---

## 📡 API Endpoints

| Endpoint | Method | Description | Request Body | Response |
|----------|--------|-------------|--------------|----------|
| `/subscribe` | POST | Subscribe with frequency | `{subscription: PushSubscription, frequency: 6\|12\|24}` | `{success: true, message: "...", frequency: 24}` |
| `/vapid-public-key` | GET | Get VAPID public key | - | `{publicKey: "..."}` |
| `/today` | GET | Get current wisdom | - | `{title: "حكمة اليوم", message: "..."}` |
| `/health` | GET | Health check & stats | - | `{status: "ok", subscribers: 10, sayings: 99}` |

### Subscription Request Example

```javascript
POST /subscribe
Content-Type: application/json

{
  "subscription": {
    "endpoint": "https://fcm.googleapis.com/fcm/send/...",
    "keys": {
      "p256dh": "BKq...",
      "auth": "xYz..."
    }
  },
  "frequency": 24  // 6, 12, or 24 hours
}
```

---

## ⚙️ Configuration

### Backend Configuration

**Original version** (`main.go`):
- Port: `8080` (hardcoded)
- VAPID keys: Auto-generated and saved to `vapid_keys.json`
- Broadcast: Fixed interval

**Production version** (`main-production.go`):
- Port: `PORT` environment variable (Render compatible)
- VAPID keys: `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` from environment
- Broadcast: Per-subscriber frequency (6h, 12h, or 24h)
- Scheduler: Checks every minute, sends based on individual preferences

### Frontend Configuration

**Local development**:
- API URL: `http://localhost:8080` (auto-detected)

**Production** (`frontend/script.js`, line 8):
```javascript
const API_URL = window.location.hostname === 'localhost' 
  ? 'http://localhost:8080'
  : 'https://your-app-name.onrender.com';  // ← Update this
```

### Notification Frequencies

Users can choose from:
- **6 hours** - 4 notifications per day
- **12 hours** - 2 notifications per day  
- **24 hours** - 1 notification per day (recommended)

---

## ⚠️ Important Notes

### Production Web Push
✅ Uses `webpush-go` library (v1.4.0) for proper VAPID authentication  
✅ Automatic cleanup of invalid subscriptions (410/404 responses)  
✅ Per-subscriber frequency management with thread-safe storage

### HTTPS Requirement
🔒 Web Push and Service Workers **require HTTPS** in production  
🔒 `localhost` works for local testing (no HTTPS needed)  
🔒 Render and Vercel provide HTTPS automatically  
🔒 Use ngrok for local public HTTPS testing

### Storage Considerations
💾 **Current**: In-memory storage with `sync.RWMutex` (thread-safe)  
💾 **Limitation**: Subscriptions lost on server restart  
💾 **Production**: Consider PostgreSQL, MySQL, or Redis for persistence  
💾 **Trade-off**: Simple deployment vs data persistence

### VAPID Keys Security
🔑 **Development**: Auto-generated on first run  
🔑 **Production**: Load from environment variables (Render)  
🔑 **Important**: Never commit VAPID keys to git (`.gitignore` included)  
🔑 **Changing keys**: Invalidates all existing subscriptions  
🔑 **Solution**: Users must re-subscribe after key changes

### Notification Scheduling
🔄 **Scheduler**: Runs every minute (checks all subscribers)
🔄 **Per-user**: Sends only if their frequency interval has passed  
🔄 **Wisdom rotation**: Sequential counter-based (1→99, then repeats)  
🔄 **Welcome message**: Immediate notification on new subscription

---

## 🧪 Troubleshooting

### Backend Issues

**"VAPID keys not found in environment"**
- ✅ Solution: Set `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` in Render environment variables

**"Failed to load sayings"**
- ✅ Solution: Set `GEMINI_API_KEY` in the backend environment

**"Port already in use"**
- ✅ Solution: Kill process using: `lsof -ti:8080 | xargs kill -9`

### Frontend Issues

**"Failed to get server public key"**
- ✅ Solution: Check that `API_URL` in `script.js` points to correct backend
- ✅ Verify Render backend is running

**CORS errors in browser console**
- ✅ Solution: Backend includes CORS middleware (check Render logs)
- ✅ Ensure backend is accessible and running

**Notifications not working**
- ✅ Ensure HTTPS (Vercel provides this)
- ✅ Check notification permissions in browser settings
- ✅ Try Incognito mode (clears cached permissions)

### Push Notification Issues

**403 Push Service Errors**
- ✅ VAPID keys might have changed
- ✅ Clear browser cache and re-subscribe
- ✅ Check Render logs for detailed error messages

**Service Worker registration failed**
- ✅ Ensure `sw.js` is in the root of frontend deployment
- ✅ Check browser console for specific errors
- ✅ Verify HTTPS is enabled

---

## 💻 Tech Stack

### Backend
- **Go** (standard library for HTTP server)
- **webpush-go** v1.4.0 (VAPID keys & push notifications)
- **jwt** v5.2.1 (dependency of webpush-go)
- **crypto** v0.31.0 (ECDSA key generation)

### Frontend
- **Vanilla JavaScript** (no frameworks)
- **Service Workers API**
- **Push API** (PushManager)
- **Notification API**
- **Responsive CSS** with gradient design and RTL support

### Deployment
- **Render** (Go backend hosting with free tier)
- **Vercel** (Static frontend hosting with free tier)
- **Go modules** (dependency management)

---

## 🌟 Future Improvements

### Backend
- [ ] PostgreSQL/MongoDB for persistent subscription storage
- [ ] Admin API for managing sayings
- [ ] Analytics endpoint (delivery rates, subscriber growth)
- [ ] Email fallback for failed push notifications
- [ ] Multi-language support
- [ ] User authentication (optional)

### Frontend
- [ ] PWA manifest for "Add to Home Screen"
- [ ] Offline support with Service Worker caching
- [ ] Settings panel (change frequency after subscription)
- [ ] Dark mode toggle
- [ ] Multi-language UI
- [ ] Push notification history view

### DevOps
- [ ] Docker containerization
- [ ] CI/CD pipeline (GitHub Actions)
- [ ] Automated testing (unit + integration)
- [ ] Monitoring and alerting (Prometheus/Grafana)
- [ ] Rate limiting for API endpoints

---

## 📊 Browser Compatibility

- ✅ **Chrome 42+** (Full support)
- ✅ **Firefox 44+** (Full support)
- ✅ **Edge 17+** (Full support)
- ✅ **Safari 16+** (macOS 13+, iOS 16.4+)
- ❌ **Internet Explorer** (Not supported - no Service Worker support)

---

## 📄License

MIT

---

## 🤝 Contributing

Contributions are welcome! Feel free to:
- Report bugs
- Suggest features
- Submit pull requests
- Improve documentation

---

**🎉 Built with ❤️ for daily wisdom seekers**

**Need help?** Check the [Deployment Guide](DEPLOYMENT.md) for detailed instructions.

### One-minute wisdom testing

Notification frequency and wisdom rotation are separate settings. To refresh the shared wisdom every minute during testing, set `HIKMAH_REFRESH_INTERVAL=1m` in backend `.env`, re-export it, and restart `go run .`. The page follows the server's `nextRotation` automatically. Both AI and local fallback rotate on minute boundaries, with at most one generation attempt per minute per server. This uses more Gemini quota. Remove the setting (and `unset HIKMAH_REFRESH_INTERVAL` in your shell) to restore the default calendar-day behavior. The override applies to all users; the notification frequency selection still controls push delivery only.
