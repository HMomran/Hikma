package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testReflection = "بالصبر والرفق تنمو قدرتك على تجاوز الصعوبات ومساعدة الآخرين."

func geminiResponse(text, reason string) string {
	data, _ := json.Marshal(map[string]interface{}{"candidates": []interface{}{map[string]interface{}{"finishReason": reason, "content": map[string]interface{}{"parts": []interface{}{map[string]interface{}{"text": text}}}}}})
	return string(data)
}

func testService(t *testing.T, handler http.HandlerFunc) (*WisdomService, *atomic.Int32) {
	t.Helper()
	t.Setenv("HIKMAH_REFRESH_INTERVAL", "")
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_MODEL", "")
	t.Setenv("HIKMAH_TIMEZONE", "Asia/Bahrain")
	t.Setenv("HIKMAH_CACHE_FILE", filepath.Join(t.TempDir(), "daily.json"))
	s, err := newWisdomService()
	if err != nil {
		t.Fatal(err)
	}
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); handler(w, r) }))
	t.Cleanup(server.Close)
	s.client = server.Client()
	s.endpoint = server.URL
	s.now = func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, s.location) }
	return s, calls
}

func TestDailyGeminiCacheAndRestart(t *testing.T) {
	s, calls := testService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("x-goog-api-key") != "test-key" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" {
			t.Error("invalid Gemini authentication or method")
		}
		var payload struct {
			SystemInstruction struct{ Parts []struct{ Text string } } `json:"systemInstruction"`
			Contents          []struct {
				Role  string
				Parts []struct{ Text string }
			} `json:"contents"`
			GenerationConfig struct {
				MaxOutputTokens int `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload.Contents) != 1 || payload.Contents[0].Role != "user" || len(payload.SystemInstruction.Parts) != 1 || payload.GenerationConfig.MaxOutputTokens != 512 {
			t.Error("invalid Gemini request body")
		}
		fmt.Fprint(w, geminiResponse(testReflection, "STOP"))
	})
	var group sync.WaitGroup
	for i := 0; i < 10; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			message, next, err := s.Current(context.Background())
			if err != nil || message != aiLabel+testReflection || next.Hour() != 0 || next.Day() != 30 {
				t.Errorf("unexpected daily result: %s %v %v", message, next, err)
			}
		}()
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent readers generated %d times", calls.Load())
	}
	restarted, err := newWisdomService()
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = s.now
	restarted.endpoint = s.endpoint
	restarted.client = s.client
	message, _, err := restarted.Current(context.Background())
	if err != nil || message != aiLabel+testReflection || calls.Load() != 1 {
		t.Fatal("restart lost daily cache")
	}
	// UTC 21:00 is the next calendar day in Bahrain, not 24 hours after generation.
	s.now = func() time.Time { return time.Date(2026, 10, 29, 21, 0, 0, 0, time.UTC) }
	if _, _, err := s.Current(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("did not regenerate at Bahrain midnight")
	}
}

func TestGeminiFailuresUseDailyLocalFallback(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"quota", 429, `{"error":{"message":"private test-key provider details"}}`},
		{"unavailable", 503, `{}`}, {"auth", 403, `{}`}, {"model unavailable", 404, `{}`},
		{"malformed", 200, `{`}, {"empty", 200, `{"candidates":[]}`},
		{"blocked", 200, `{"promptFeedback":{"blockReason":"SAFETY"}}`},
		{"truncated", 200, geminiResponse(testReflection, "MAX_TOKENS")},
		{"non Arabic", 200, geminiResponse("Be patient and kind to other people every day.", "STOP")},
		{"attributed Ali", 200, geminiResponse("قال الإمام عَلِيّ عليه السلام إن الصبر مفتاح الفرج.", "STOP")},
		{"attributed Ahlulbayt", 200, geminiResponse("قال أهل البيت عليهم السلام إن الصبر مفتاح الفرج.", "STOP")},
		{"too long", 200, geminiResponse(strings.Repeat("الصبر ", 26), "STOP")},
		{"oversized", 200, strings.Repeat("x", (1<<20)+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, calls := testService(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) })
			first, _, err := s.Current(context.Background())
			if err != nil || !strings.HasPrefix(first, localLabel) {
				t.Fatalf("missing fallback: %q %v", first, err)
			}
			for i := 0; i < 3; i++ {
				message, _, err := s.Current(context.Background())
				if err != nil || message != first {
					t.Fatal("fallback changed within a day")
				}
			}
			if calls.Load() != 1 {
				t.Fatal("failure caused repeated API calls")
			}
			restored, err := newWisdomService()
			if err != nil {
				t.Fatal(err)
			}
			restored.now = s.now
			message, _, err := restored.Current(context.Background())
			if err != nil || message != first {
				t.Fatal("fallback was not persisted")
			}
		})
	}
}

func TestGeminiTimeoutAndSafeErrors(t *testing.T) {
	s, _ := testService(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	})
	s.client.Timeout = 20 * time.Millisecond
	message, _, err := s.Current(context.Background())
	if err != nil || !strings.HasPrefix(message, localLabel) {
		t.Fatal("timeout lost fallback")
	}
	s.endpoint = "http://127.0.0.1:1/?key=test-key"
	_, err = s.generate(context.Background())
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("unsafe transport error: %v", err)
	}
}

func TestGeminiConfigurationAndNoKeyFallback(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GEMINI_MODEL", "")
	t.Setenv("HIKMAH_TIMEZONE", "")
	t.Setenv("HIKMAH_CACHE_FILE", filepath.Join(t.TempDir(), "daily.json"))
	s, err := newWisdomService()
	if err != nil {
		t.Fatal(err)
	}
	if s.model != "gemini-2.5-flash" || s.endpoint != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent" || s.location.String() != "Asia/Bahrain" || s.client.Timeout != 30*time.Second {
		t.Fatal("incorrect defaults")
	}
	s.client = &http.Client{Transport: forbidNetwork{t}}
	message, _, err := s.Current(context.Background())
	if err != nil || !strings.HasPrefix(message, localLabel) {
		t.Fatal("no-key fallback failed")
	}
	t.Setenv("GEMINI_MODEL", "../bad?key=secret")
	if _, err := newWisdomService(); err == nil {
		t.Fatal("unsafe model accepted")
	}
	t.Setenv("GEMINI_MODEL", "")
	t.Setenv("HIKMAH_TIMEZONE", "bad zone")
	if _, err := newWisdomService(); err == nil {
		t.Fatal("bad zone accepted")
	}
}

type forbidNetwork struct{ t *testing.T }

func (f forbidNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Error("unexpected network request")
	return nil, fmt.Errorf("network forbidden")
}

func TestTodayHandlerUsesSameDailyWisdom(t *testing.T) {
	s, calls := testService(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, geminiResponse(testReflection, "STOP")) })
	old := wisdomService
	wisdomService = s
	t.Cleanup(func() { wisdomService = old })
	recorder := httptest.NewRecorder()
	todayHandler(recorder, httptest.NewRequest(http.MethodGet, "/api/hikmah", nil))
	var response struct {
		Message      string `json:"message"`
		NextRotation int64  `json:"nextRotation"`
	}
	if recorder.Code != 200 || json.Unmarshal(recorder.Body.Bytes(), &response) != nil || response.Message != aiLabel+testReflection || response.NextRotation == 0 {
		t.Fatal("invalid API response")
	}
	// This is the same method used by welcome and scheduled push notifications.
	message, _, err := s.Current(context.Background())
	if err != nil || message != response.Message || calls.Load() != 1 {
		t.Fatal("page and notifications diverged")
	}
}

func TestMinuteRotation(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			count := 0
			s, calls := testService(t, func(w http.ResponseWriter, r *http.Request) {
				count++
				if fallback {
					w.WriteHeader(429)
					return
				}
				text := testReflection
				if count > 1 {
					text = "باللطف وحسن الاستماع تبني جسور الثقة بينك وبين الآخرين."
				}
				fmt.Fprint(w, geminiResponse(text, "STOP"))
			})
			s.refreshInterval = time.Minute
			now := s.now()
			s.now = func() time.Time { return now }
			first, next, err := s.Current(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			now = now.Add(30 * time.Second)
			same, _, _ := s.Current(context.Background())
			if same != first || calls.Load() != 1 {
				t.Fatal("cache not shared within minute")
			}
			now = next
			second, _, err := s.Current(context.Background())
			if err != nil || second == first || calls.Load() != 2 {
				t.Fatal("wisdom did not change after minute")
			}
		})
	}
}
