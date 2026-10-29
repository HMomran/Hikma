package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
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

// Keep the original collection separate from generated reflections and available
// in deployed binaries. These entries have no independently verified citations.
//
//go:embed data/hikmah.json
var localWisdomJSON []byte

const aiLabel = "تأمل مولّد بالذكاء الاصطناعي: "
const localLabel = "من المجموعة المحلية (دون نسبة موثّقة): "

var modelNamePattern = regexp.MustCompile(`^gemini-[a-zA-Z0-9.-]+$`)

type dailyWisdom struct {
	Day      string `json:"day"`
	Timezone string `json:"timezone"`
	Message  string `json:"message"`
}

// One shared result per calendar day, for both the website and push delivery.
// A fallback also lasts the whole day: provider failures never cause retry storms.
type WisdomService struct {
	mu                      sync.Mutex
	client                  *http.Client
	apiKey, model, endpoint string
	location                *time.Location
	refreshInterval         time.Duration
	now                     func() time.Time
	fallback                []string
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
	var fallback []string
	if err := json.Unmarshal(localWisdomJSON, &fallback); err != nil || len(fallback) == 0 {
		return nil, fmt.Errorf("invalid embedded local wisdom collection")
	}
	cacheFile := os.Getenv("HIKMAH_CACHE_FILE")
	if cacheFile == "" {
		cacheFile = ".cache/daily-hikmah.json"
	}
	s := &WisdomService{
		client: &http.Client{Timeout: 30 * time.Second},
		apiKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")), model: model,
		endpoint: "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent",
		location: location, refreshInterval: refreshInterval, now: time.Now, fallback: fallback, cacheFile: cacheFile,
	}
	// A missing key is supported: the local daily fallback works without network.
	if data, err := os.ReadFile(cacheFile); err == nil {
		var cached dailyWisdom
		if json.Unmarshal(data, &cached) == nil && cached.Timezone == zone && validCachedWisdom(cached.Message) {
			s.cached = cached
		}
	}
	return s, nil
}

func validCachedWisdom(message string) bool {
	if strings.HasPrefix(message, aiLabel) {
		return validateReflection(strings.TrimPrefix(message, aiLabel)) == nil
	}
	if strings.HasPrefix(message, localLabel) {
		var collection []string
		if json.Unmarshal(localWisdomJSON, &collection) != nil {
			return false
		}
		for _, item := range collection {
			if message == localLabel+item {
				return true
			}
		}
	}
	return false
}

func (s *WisdomService) Current(ctx context.Context) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().In(s.location)
	day := now.Format("2006-01-02")
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	next := midnight.AddDate(0, 0, 1)
	ordinal := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix() / 86400
	if s.refreshInterval > 0 {
		slot := now.UnixNano() / int64(s.refreshInterval)
		day = fmt.Sprintf("interval:%d:%d", s.refreshInterval, slot)
		next = time.Unix(0, (slot+1)*int64(s.refreshInterval)).In(s.location)
		ordinal = slot
	}
	if s.cached.Day == day && s.cached.Message != "" {
		return s.cached.Message, next, nil
	}
	message, err := s.generate(ctx)
	if err == nil && aiLabel+message == s.cached.Message {
		err = fmt.Errorf("Gemini repeated the previous reflection")
	}
	if err != nil {
		// Errors are constructed locally; raw provider responses and keys stay private.
		log.Printf("Gemini unavailable; using local daily wisdom: %v", err)
		if len(s.fallback) == 0 {
			return "", next, fmt.Errorf("local wisdom collection unavailable")
		}
		index := int((ordinal%int64(len(s.fallback)) + int64(len(s.fallback))) % int64(len(s.fallback)))
		message = localLabel + s.fallback[index]
	} else {
		message = aiLabel + message
	}
	s.cached = dailyWisdom{Day: day, Timezone: s.location.String(), Message: message}
	// Disk cache preserves the daily selection across restarts of a single server.
	if s.cacheFile != "" {
		if err := s.saveCache(); err != nil {
			log.Print("Daily wisdom cache could not be persisted; retaining in-memory cache")
		}
	}
	return message, next, nil
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
	// Reject attribution markers and religious names; generated text is never
	// promoted into the separately maintained historical collection.
	for _, phrase := range []string{"الامام", "عليه السلام", "اهل البيت", "اهلالبيت", "قال علي", "عن علي", "علي بن", "نهج البلاغة", "قال رسول", "قال النبي", "قال الله", "حديث", "قران", "ali", "ahlulbayt"} {
		if strings.Contains(normalized, phrase) {
			return fmt.Errorf("Gemini returned an unverified religious attribution")
		}
	}
	return nil
}
