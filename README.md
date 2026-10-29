# 🌿 Hikma

![Go](https://img.shields.io/badge/Go-1.25.5%2B-00ADD8?logo=go&logoColor=white)
![JavaScript](https://img.shields.io/badge/JavaScript-Vanilla-F7DF1E?logo=javascript&logoColor=black)
![Gemini API](https://img.shields.io/badge/AI-Google_Gemini-4285F4)

Hikma is an AI-powered application that generates original Arabic wisdom, meaningful thoughts, and life lessons to inspire reflection, personal growth, and positivity. Google Gemini creates short reflections that users can read on the website or receive through scheduled Web Push notifications.

All wisdom is clearly labeled as AI-generated and is not attributed to any specific person.

## ✨ Features

- **Fresh Arabic wisdom:** AI-generated reflections shared across users, with one cached selection per calendar day by default and automatic page refresh at rotation time.
- **Thoughtful themes:** Short, meaningful reflections about patience, kindness, morality, knowledge, personal growth, and everyday life, generated through Gemini.
- **Safe failure handling:** Keeps the last valid AI reflection during outages; returns a temporary-unavailable error and skips notifications when none is available.
- **Shared caching:** Website and notification requests reuse the same selection; a disk cache preserves it across restarts when retained.
- **Web Push notifications:** VAPID-authenticated delivery through a service worker, a welcome notification, and selectable 6-, 12-, or 24-hour intervals. A one-minute option is available for testing.
- **Arabic interface:** Responsive layout, right-to-left styling, browser subscription controls, and a notification countdown.

The application validates AI responses before displaying or sending them. Its reflections are generated content, not sourced quotations.

## 🛠️ Tech Stack

- **Backend:** Go 1.25.5+, standard-library HTTP server, goroutines, and JSON APIs.
- **AI:** Google Gemini REST API.
- **Notifications:** [`webpush-go`](https://github.com/SherClockHolmes/webpush-go), Web Push, Notifications API, and service workers.
- **Frontend:** HTML5, CSS3, and vanilla JavaScript; no Node.js build step.
- **Storage:** In-memory subscriptions and a file-backed cache of validated AI reflections.

**License:** MIT, as declared in the original project documentation.

## 🚀 Getting Started

Install Go 1.25.5 or newer and use a browser that supports Web Push. From the repository root:

```sh
cd backend
go mod download
test -f .env || cp .env.example .env
```

For first-time push setup, generate VAPID keys:

```sh
go run tools/generate_vapid_keys.go
```

Copy the generated public and private values into `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` in `backend/.env`. Keep existing keys if already configured. The generator also writes an ignored `vapid_keys.json`; the server reads keys from environment variables, not that file. Without configured VAPID keys, it generates temporary keys on startup.

Add your Gemini key as described below. Without a configured key, only an existing valid AI cache can be served. Load the environment and start the server from `backend`:

```sh
set -a
source .env
set +a
go run .
```

Open **[localhost:8080](http://localhost:8080)**, select a notification frequency, and allow notifications when subscribing. The server serves `../frontend`, so keep `backend` as the working directory. `.env` is not loaded automatically; reload it and restart after changing settings. `PORT` defaults to `8080`.

Run checks from `backend`:

```sh
go test -race ./...
go vet ./...
```

With the server running, check the API from another terminal:

```sh
curl -fsS http://localhost:8080/api/health
curl -fsS http://localhost:8080/api/hikmah
```

For deployment, build with `go build -o app .` and run `./app` from `backend`, retaining the sibling `frontend` directory. Set environment variables on the host and use HTTPS for Web Push outside localhost. For separate static hosting, publish `frontend` and configure `RENDER_BACKEND_URL` in [frontend/script.js](frontend/script.js) to point to the backend.

Subscriptions are held in memory and are lost on server restart. Keep VAPID keys stable and retain the wisdom cache on durable storage to preserve the current selection. Caching is shared within one backend instance.

## 🤖 AI Configuration

Create a key in [Google AI Studio](https://aistudio.google.com/apikey) and configure the backend using [`.env.example`](backend/.env.example):

```dotenv
GEMINI_API_KEY=your_gemini_api_key
GEMINI_MODEL=gemini-2.5-flash
HIKMAH_TIMEZONE=Asia/Bahrain
HIKMAH_CACHE_FILE=.cache/daily-hikmah.json
```

Keep API and private VAPID keys in the ignored `.env` or host environment, never in frontend code or commits. The default model in the code is `gemini-2.5-flash`. If unavailable to your project, set `GEMINI_MODEL` to an accessible text model. Consult Google's [model availability](https://ai.google.dev/gemini-api/docs/models/gemini-2.5-flash) and [free-tier pricing](https://ai.google.dev/gemini-api/docs/pricing); access and quotas depend on your account.

The first request after a rotation generates and validates a new AI reflection. All page views and push notifications reuse the same cached text. Failed requests use exponential retry backoff from one minute up to one hour, persisted across restarts. The last successful AI reflection is retained and may be served beyond its original day until generation recovers. If there is no valid AI cache, `/api/hikmah` returns HTTP 503 with a generic error and `Retry-After`; the page displays an Arabic unavailable message and notifications are skipped. Provider details and keys are never sent to the frontend.

To test a new key independently of an existing cache or retry delay, use a fresh test cache file:

```sh
HIKMAH_CACHE_FILE=.cache/gemini-test.json go run .
```

Successful messages begin with `تأمل مولّد بالذكاء الاصطناعي:`. Automated tests use mock Gemini responses, require no real key, and cover validation, caching, stale-cache retention, retry backoff, safe HTTP errors, and notification payload selection. They do not prove live Gemini access or delivery through a browser push service.

For one-minute wisdom rotation during testing, set `HIKMAH_REFRESH_INTERVAL=1m` in `.env`, reload it, and restart. This applies to all users of the backend and can use one AI request per minute. It is separate from the notification-frequency selector. Leave the setting unset for calendar-day rotation; if previously exported, also run `unset HIKMAH_REFRESH_INTERVAL` before restarting.
