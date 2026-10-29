package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"
)

const aiLabel = "تأمل مولّد بالذكاء الاصطناعي: "

var errWisdomUnavailable = errors.New("wisdom is temporarily unavailable")

var modelNamePattern = regexp.MustCompile(`^gemini-[a-zA-Z0-9.-]+$`)

type dailyWisdom struct {
	Day        string    `json:"day"`
	Timezone   string    `json:"timezone"`
	Message    string    `json:"message"`
	RetryAfter time.Time `json:"retryAfter,omitempty"`
	Failures   int       `json:"failures,omitempty"`
}

// One shared result per calendar day, for both the website and push delivery.
// Only validated Gemini text is cached; failed refreshes retain the last success.
type WisdomService struct {
	mu                      sync.Mutex
	client                  *http.Client
	apiKey, model, endpoint string
	location                *time.Location
	refreshInterval         time.Duration
	now                     func() time.Time
	cacheFile               string
	cached                  dailyWisdom
}

func newWisdomService() (*WisdomService, error) {
	model := strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	if model == "" {
		model = "gemini-2.5-flash"
	}
	if !modelNamePattern.MatchString(model) {
		return nil, fmt.Errorf("invalid GEMINI_MODEL name")
	}
	zone := os.Getenv("HIKMAH_TIMEZONE")
	if zone == "" {
		zone = "Asia/Bahrain"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("invalid HIKMAH_TIMEZONE")
	}
	var refreshInterval time.Duration
	if value := strings.TrimSpace(os.Getenv("HIKMAH_REFRESH_INTERVAL")); value != "" {
		refreshInterval, err = time.ParseDuration(value)
		if err != nil || refreshInterval < time.Minute || refreshInterval > 24*time.Hour {
			return nil, fmt.Errorf("HIKMAH_REFRESH_INTERVAL must be between 1m and 24h")
		}
	}
	cacheFile := os.Getenv("HIKMAH_CACHE_FILE")
	if cacheFile == "" {
		cacheFile = ".cache/daily-hikmah.json"
	}
	s := &WisdomService{
		client: &http.Client{Timeout: 30 * time.Second},
		apiKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")), model: model,
		endpoint: "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent",
		location: location, refreshInterval: refreshInterval, now: time.Now, cacheFile: cacheFile,
	}
	// Restore validated AI content and retry state without changing subscription data.
	if data, err := os.ReadFile(cacheFile); err == nil {
		var cached dailyWisdom
		if json.Unmarshal(data, &cached) == nil && cached.Timezone == zone && (validCachedWisdom(cached.Message) || (cached.Message == "" && cached.Day == "")) {
			s.cached = cached
		}
	}
	return s, nil
}

func validCachedWisdom(message string) bool {
	return strings.HasPrefix(message, aiLabel) && validateReflection(strings.TrimPrefix(message, aiLabel)) == nil
}

func (s *WisdomService) Current(ctx context.Context) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().In(s.location)
	day := now.Format("2006-01-02")
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	next := midnight.AddDate(0, 0, 1)
	if s.refreshInterval > 0 {
		slot := now.UnixNano() / int64(s.refreshInterval)
		day = fmt.Sprintf("interval:%d:%d", s.refreshInterval, slot)
		next = time.Unix(0, (slot+1)*int64(s.refreshInterval)).In(s.location)
	}
	if s.cached.Day == day && s.cached.Message != "" {
		return s.cached.Message, next, nil
	}
	if now.Before(s.cached.RetryAfter) {
		if validCachedWisdom(s.cached.Message) {
			return s.cached.Message, s.cached.RetryAfter, nil
		}
		return "", s.cached.RetryAfter, errWisdomUnavailable
	}
	message, err := s.generate(ctx)
	if err == nil && aiLabel+message == s.cached.Message {
		err = fmt.Errorf("Gemini repeated the previous reflection")
	}
	if err != nil {
		// Never replace a successful reflection with failed output. Persist retry
		// state too so restarting does not reset the provider failure backoff.
		s.cached.Failures++
		if s.cached.Failures > 7 {
			s.cached.Failures = 7
		}
		delay := time.Minute * time.Duration(1<<(s.cached.Failures-1))
		if delay > time.Hour {
			delay = time.Hour
		}
		s.cached.RetryAfter = s.now().Add(delay)
		s.cached.Timezone = s.location.String()
		log.Printf("Gemini generation unavailable; retry delayed: %v", err)
		s.persistCache()
		if validCachedWisdom(s.cached.Message) {
			return s.cached.Message, s.cached.RetryAfter, nil
		}
		return "", s.cached.RetryAfter, errWisdomUnavailable
	}
	message = aiLabel + message
	s.cached = dailyWisdom{Day: day, Timezone: s.location.String(), Message: message}
	s.persistCache()
	return message, next, nil
}

func (s *WisdomService) persistCache() {
	if s.cacheFile != "" {
		if err := s.saveCache(); err != nil {
			log.Print("Wisdom cache could not be persisted; retaining in-memory cache")
		}
	}
}

func (s *WisdomService) saveCache() error {
	if err := os.MkdirAll(filepath.Dir(s.cacheFile), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(s.cached)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.cacheFile), ".hikmah-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.cacheFile)
}

func (s *WisdomService) generate(ctx context.Context) (string, error) {
	if s.apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY is not configured")
	}
	config := map[string]interface{}{"maxOutputTokens": 512, "responseMimeType": "text/plain"}
	// Keep the preferred 2.5 model's output budget for the final reflection.
	if strings.HasPrefix(s.model, "gemini-2.5-") {
		config["thinkingConfig"] = map[string]int{"thinkingBudget": 0}
	}
	payload, err := json.Marshal(map[string]interface{}{
		"systemInstruction": map[string]interface{}{"parts": []map[string]string{{"text": "اكتب تأملاً أصلياً قصيراً ذا معنى باللغة العربية الفصحى، مناسباً لجميع الأعمار، عن الصبر أو اللطف أو الأخلاق أو العلم أو تطوير النفس أو الحياة. أعد النص فقط في جملة واحدة من ٥ إلى ٢٥ كلمة، دون عنوان أو شرح أو تنسيق. لا تقتبس نصاً تاريخياً أو دينياً ولا تنسب أي كلام إلى الإمام علي عليه السلام أو أهل البيت أو أي شخص آخر أو القرآن أو الحديث، ولا تخترع مصدراً أو توقيعاً."}}},
		"contents":          []map[string]interface{}{{"role": "user", "parts": []map[string]string{{"text": "قدّم تأملاً عملياً يساعد القارئ على تحسين يومه وأخلاقه."}}}},
		"generationConfig":  config,
	})
	if err != nil {
		return "", fmt.Errorf("could not encode Gemini request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("could not create Gemini request")
	}
	req.Header.Set("x-goog-api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Gemini request failed or timed out")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return "", fmt.Errorf("invalid Gemini response size")
	}
	var result struct {
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					Text    string `json:"text"`
					Thought bool   `json:"thought"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(data, &result) != nil {
		return "", fmt.Errorf("invalid Gemini response")
	}
	if result.PromptFeedback.BlockReason != "" || len(result.Candidates) != 1 || result.Candidates[0].FinishReason != "STOP" {
		return "", fmt.Errorf("Gemini response blocked or incomplete")
	}
	var parts []string
	for _, part := range result.Candidates[0].Content.Parts {
		if !part.Thought {
			parts = append(parts, part.Text)
		}
	}
	message := strings.TrimSpace(strings.Join(parts, ""))
	if err := validateReflection(message); err != nil {
		return "", err
	}
	return message, nil
}

func validateReflection(message string) error {
	words := strings.Fields(message)
	if len(words) < 5 || len(words) > 25 || utf8.RuneCountInString(message) > 500 {
		return fmt.Errorf("Gemini returned empty or oversized reflection")
	}
	arabic, letters := 0, 0
	for _, r := range message {
		if unicode.IsLetter(r) {
			letters++
			if unicode.In(r, unicode.Arabic) {
				arabic++
			}
		}
		if r == '<' || r == '>' || r == '`' || r == '\n' || r == '\r' {
			return fmt.Errorf("Gemini returned unexpected formatting")
		}
	}
	if arabic == 0 || arabic*10 < letters*8 {
		return fmt.Errorf("Gemini reflection is not Arabic")
	}
	normalized := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) || r == 'ـ' {
			return -1
		}
		switch r {
		case 'أ', 'إ', 'آ':
			return 'ا'
		case 'ى':
			return 'ي'
		}
		return unicode.ToLower(r)
	}, message)
	// Reject unverified religious attribution in generated reflections.
	for _, phrase := range []string{"الامام", "عليه السلام", "اهل البيت", "اهلالبيت", "قال علي", "عن علي", "علي بن", "نهج البلاغة", "قال رسول", "قال النبي", "قال الله", "حديث", "قران", "ali", "ahlulbayt"} {
		if strings.Contains(normalized, phrase) {
			return fmt.Errorf("Gemini returned an unverified religious attribution")
		}
	}
	return nil
}
