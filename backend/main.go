package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// PushSubscription represents a browser push subscription
type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// SubscriptionRequest includes subscription and frequency preference
type SubscriptionRequest struct {
	Subscription PushSubscription `json:"subscription"`
	Frequency    int              `json:"frequency"` // 6, 12, or 24 hours
}

// Subscriber represents a user subscription with preferences
type Subscriber struct {
	Subscription PushSubscription
	Frequency    int       // Hours between notifications (6, 12, or 24)
	LastSent     time.Time // Last time notification was sent
}

// Notification payload sent to users
type Notification struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// SubscriptionStore manages all push subscriptions with frequency preferences
type SubscriptionStore struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscriber // Key: endpoint URL
}

func (s *SubscriptionStore) Add(subscriber *Subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.subscribers == nil {
		s.subscribers = make(map[string]*Subscriber)
	}

	endpoint := subscriber.Subscription.Endpoint
	if existing, exists := s.subscribers[endpoint]; exists {
		log.Printf("📝 Updating subscription: %s (frequency: %dh)", endpoint[:50], subscriber.Frequency)
		existing.Frequency = subscriber.Frequency
		return
	}

	s.subscribers[endpoint] = subscriber
	log.Printf("✅ New subscription added: %s (frequency: %dh). Total: %d",
		endpoint[:50], subscriber.Frequency, len(s.subscribers))
}

func (s *SubscriptionStore) GetAll() []*Subscriber {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subs := make([]*Subscriber, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		subs = append(subs, sub)
	}
	return subs
}

func (s *SubscriptionStore) Remove(endpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subscribers[endpoint]; exists {
		delete(s.subscribers, endpoint)
		log.Printf("🗑️  Subscription removed: %s. Total: %d", endpoint[:50], len(s.subscribers))
	}
}

func (s *SubscriptionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

func (s *SubscriptionStore) UpdateLastSent(endpoint string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sub, exists := s.subscribers[endpoint]; exists {
		sub.LastSent = t
	}
}

var (
	vapidPublicKey  string
	vapidPrivateKey string
	subStore        = &SubscriptionStore{subscribers: make(map[string]*Subscriber)}
	wisdomService   *WisdomService
)

// getVAPIDKeys loads VAPID keys from environment variables or generates new ones
func getVAPIDKeys() error {
	vapidPublicKey = os.Getenv("VAPID_PUBLIC_KEY")
	vapidPrivateKey = os.Getenv("VAPID_PRIVATE_KEY")

	// If both keys are provided via environment, use them
	if vapidPublicKey != "" && vapidPrivateKey != "" {
		log.Println("✅ Using VAPID keys from environment variables")
		return nil
	}

	// Otherwise, generate new keys (for local development)
	log.Println("⚠️  VAPID keys not found in environment. Generating new ones...")
	log.Println("⚠️  For production, set VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY environment variables")

	privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return fmt.Errorf("failed to generate VAPID keys: %v", err)
	}

	vapidPublicKey = publicKey
	vapidPrivateKey = privateKey

	log.Println("✅ VAPID keys generated successfully")
	log.Printf("   Public Key:  %s", publicKey)
	log.Println("   Private key generated (not logged)")
	log.Println("   ⚠️  Save these keys as environment variables for production!")

	return nil
}

// Both scheduled and welcome notifications use the website's validated AI cache.
func currentWisdomNotification(ctx context.Context) (Notification, error) {
	message, _, err := wisdomService.Current(ctx)
	if err != nil {
		return Notification{}, err
	}
	if !validCachedWisdom(message) {
		return Notification{}, errWisdomUnavailable
	}
	return Notification{Title: "حكمة اليوم", Message: message}, nil
}

// sendPushNotification sends a Web Push notification to a subscriber
func sendPushNotification(sub *Subscriber, notif Notification) error {
	if !validCachedWisdom(notif.Message) {
		return fmt.Errorf("notification requires valid AI-generated wisdom")
	}
	payload, err := json.Marshal(notif)
	if err != nil {
		return err
	}

	// Create subscription object for webpush library
	s := &webpush.Subscription{
		Endpoint: sub.Subscription.Endpoint,
		Keys: webpush.Keys{
			Auth:   sub.Subscription.Keys.Auth,
			P256dh: sub.Subscription.Keys.P256dh,
		},
	}

	// Send push notification
	resp, err := webpush.SendNotification(payload, s, &webpush.Options{
		Subscriber:      "mailto:admin@example.com",
		VAPIDPublicKey:  vapidPublicKey,
		VAPIDPrivateKey: vapidPrivateKey,
		TTL:             30,
	})

	if err != nil {
		return fmt.Errorf("failed to send push: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// If subscription is invalid (410 Gone), remove it
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			log.Printf("🗑️  Removing invalid subscription: %s", sub.Subscription.Endpoint[:50])
			subStore.Remove(sub.Subscription.Endpoint)
		}
		return fmt.Errorf("push service responded with status: %d", resp.StatusCode)
	}

	return nil
}

// notificationScheduler continuously checks subscribers and sends notifications based on their frequency
func notificationScheduler() {
	// Check every minute (allows testing with 1-minute frequency)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Println("🚀 Notification scheduler started (checking every minute)")

	for {
		<-ticker.C

		subscribers := subStore.GetAll()
		if len(subscribers) == 0 {
			log.Println("⏳ No subscribers yet. Waiting...")
			continue
		}

		log.Printf("🔍 Checking %d subscribers for due notifications...", len(subscribers))

		now := time.Now()
		sentCount := 0

		for _, sub := range subscribers {
			// Calculate time since last notification
			timeSince := now.Sub(sub.LastSent)

			// Frequency: 1 = 1 minute (testing), others = hours
			var frequencyDuration time.Duration
			if sub.Frequency == 1 {
				frequencyDuration = 1 * time.Minute // Testing mode
			} else {
				frequencyDuration = time.Duration(sub.Frequency) * time.Hour
			}

			// Check if it's time to send a notification
			if timeSince >= frequencyDuration {
				notif, err := currentWisdomNotification(context.Background())
				if err != nil {
					log.Print("Skipping notification: wisdom temporarily unavailable")
					continue
				}

				if err := sendPushNotification(sub, notif); err != nil {
					log.Printf("❌ Failed to send to %s: %v", sub.Subscription.Endpoint[:50], err)
				} else {
					subStore.UpdateLastSent(sub.Subscription.Endpoint, now)
					log.Printf("📤 Sent to %s (frequency: %dh, last sent: %s ago)",
						sub.Subscription.Endpoint[:50], sub.Frequency, timeSince.Round(time.Minute))
					sentCount++
				}
			}
		}

		if sentCount > 0 {
			log.Printf("✅ Sent %d notifications in this cycle", sentCount)
		}
	}
}

// corsMiddleware adds CORS headers for cross-origin requests
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow requests from any origin (for Vercel frontend)
		// In production, you might want to restrict this to your Vercel domain
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")
		w.Header().Set("Access-Control-Expose-Headers", "Retry-After")

		// Handle preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// Handler for POST /subscribe
func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Invalid subscription data: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate subscription
	if req.Subscription.Endpoint == "" {
		http.Error(w, "Invalid subscription: missing endpoint", http.StatusBadRequest)
		return
	}

	// Validate frequency (1 minute for testing, or 6, 12, 24 hours)
	if req.Frequency != 1 && req.Frequency != 6 && req.Frequency != 12 && req.Frequency != 24 {
		log.Printf("⚠️  Invalid frequency %d, defaulting to 24 hours", req.Frequency)
		req.Frequency = 24
	}

	// Create subscriber
	subscriber := &Subscriber{
		Subscription: req.Subscription,
		Frequency:    req.Frequency,
		LastSent:     time.Time{}, // Zero time means never sent, will send immediately
	}

	subStore.Add(subscriber)

	// Send immediate welcome notification
	go func() {
		notif, err := currentWisdomNotification(context.Background())
		if err != nil {
			log.Print("Skipping welcome notification: wisdom temporarily unavailable")
			return
		}
		if err := sendPushNotification(subscriber, notif); err != nil {
			log.Printf("❌ Failed to send welcome notification: %v", err)
		} else {
			subStore.UpdateLastSent(req.Subscription.Endpoint, time.Now())
			log.Printf("📬 Welcome notification sent to new subscriber")
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "Subscribed successfully",
		"frequency": req.Frequency,
	})
}

// Handler for GET /vapid-public-key
func vapidPublicKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"publicKey": vapidPublicKey,
	})
}

// Handler for GET /today and /api/hikmah
func todayHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wisdom, nextRotation, err := wisdomService.Current(r.Context())
	if err != nil {
		retrySeconds := int(time.Until(nextRotation).Seconds()) + 1
		if retrySeconds < 60 {
			retrySeconds = 60
		}
		w.Header().Set("Retry-After", fmt.Sprint(retrySeconds))
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Wisdom is temporarily unavailable. Please try again shortly.", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"title":        "حكمة اليوم",
		"message":      wisdom,
		"nextRotation": nextRotation.Unix(), // Unix timestamp for frontend
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("❌ Error encoding hikmah response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	log.Printf("📖 Served hikmah to %s", r.RemoteAddr)
}

// Handler for GET /health and /api/health (for Render health checks)
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":      "ok",
		"subscribers": subStore.Count(),
		"source":      "gemini",
		"version":     "1.0.0",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("❌ Error encoding health response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func main() {
	log.Println("🚀 Starting Daily Hikmah Notification Service")

	// Load VAPID keys from environment or generate new ones
	if err := getVAPIDKeys(); err != nil {
		log.Fatalf("❌ VAPID key error: %v", err)
	}

	var err error
	wisdomService, err = newWisdomService()
	if err != nil {
		log.Fatalf("AI configuration error: %v", err)
	}

	// Start notification scheduler in background
	go notificationScheduler()

	// Setup HTTP routes with CORS
	// Support both /api/* and direct /* endpoints for flexibility
	http.HandleFunc("/api/subscribe", corsMiddleware(subscribeHandler))
	http.HandleFunc("/subscribe", corsMiddleware(subscribeHandler)) // Backwards compatibility

	http.HandleFunc("/api/vapid-public-key", corsMiddleware(vapidPublicKeyHandler))
	http.HandleFunc("/vapid-public-key", corsMiddleware(vapidPublicKeyHandler)) // Backwards compatibility

	http.HandleFunc("/api/hikmah", corsMiddleware(todayHandler))
	http.HandleFunc("/today", corsMiddleware(todayHandler)) // Backwards compatibility

	http.HandleFunc("/api/health", corsMiddleware(healthHandler))
	http.HandleFunc("/health", corsMiddleware(healthHandler)) // Backwards compatibility

	// Serve static files (for local development)
	// In production, frontend will be on Vercel
	fs := http.FileServer(http.Dir("../frontend"))
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add CORS headers for static files too
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		fs.ServeHTTP(w, r)
	}))

	// Get port from environment (required for Render)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Println("⚠️  PORT environment variable not set, using default: 8080")
	}

	log.Printf("✅ Server starting on port %s", port)
	log.Printf("📡 API endpoints:")
	log.Printf("   POST   /api/subscribe     (also: /subscribe)")
	log.Printf("   GET    /api/vapid-public-key     (also: /vapid-public-key)")
	log.Printf("   GET    /api/hikmah       (also: /today)")
	log.Printf("   GET    /api/health       (also: /health)")
	log.Println()
	log.Printf("🌐 Backend ready for production deployment")
	log.Printf("💡 Use /api/* endpoints for production, /* for backwards compatibility")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}
