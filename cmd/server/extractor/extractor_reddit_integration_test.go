package extractor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadDotEnvIfPresent() {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	dir := wd
	for {
		p := filepath.Join(dir, ".env")
		b, err := os.ReadFile(p)
		if err == nil {
			applyDotEnv(string(b))
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func applyDotEnv(content string) {
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			continue
		}
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

func getFirstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func TestExtractRedditPost_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}

	loadDotEnvIfPresent()

	redditURL := getFirstEnv("REDDIT_URL", "REDDIT_POST_URL", "URL")
	if redditURL == "" {
		t.Skip("missing reddit URL; set REDDIT_URL (or REDDIT_POST_URL/URL) in env or .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	post, err := ExtractRedditPost(ctx, redditURL)
	if err != nil {
		t.Fatalf("ExtractRedditPost failed: %v", err)
	}
	if post == nil {
		t.Fatalf("ExtractRedditPost returned nil post")
	}

	out, err := json.MarshalIndent(post, "", "  ")
	if err != nil {
		t.Fatalf("marshal output failed: %v", err)
	}

	fmt.Printf("ExtractRedditPost output (url=%s):\n%s\n", redditURL, string(out))
}
