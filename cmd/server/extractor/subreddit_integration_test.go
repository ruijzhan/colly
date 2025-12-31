package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestExtractSubredditPosts_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}

	loadDotEnvIfPresent()

	subredditURL := getFirstEnv("REDDIT_SUBREDDIT_URL", "SUBREDDIT_URL", "REDDIT_URL")
	if subredditURL == "" {
		t.Skip("missing subreddit URL; set REDDIT_SUBREDDIT_URL (or SUBREDDIT_URL/REDDIT_URL) in env or .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	resp, err := ExtractSubredditPosts(ctx, subredditURL, "hot", "", 5, "")
	if err != nil {
		t.Fatalf("ExtractSubredditPosts failed: %v", err)
	}
	if resp == nil {
		t.Fatalf("ExtractSubredditPosts returned nil response")
	}

	if len(resp.Posts) == 0 {
		t.Fatalf("no posts returned; try a more active subreddit URL")
	}

	for i, post := range resp.Posts {
		if post.Title == "" {
			t.Fatalf("post %d missing title", i)
		}
		if post.PostLink == "" {
			t.Fatalf("post %d missing post link", i)
		}
		// Validate post link format
		if !strings.HasPrefix(post.PostLink, "https://www.reddit.com/") {
			t.Fatalf("post %d has invalid post link: %s", i, post.PostLink)
		}
	}

	out, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		t.Fatalf("marshal output failed: %v", err)
	}

	fmt.Printf("ExtractSubredditPosts output (url=%s):\n%s\n", subredditURL, string(out))
}

func TestExtractSubredditPosts_Integration_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}

	loadDotEnvIfPresent()

	subredditURL := getFirstEnv("REDDIT_SUBREDDIT_URL", "SUBREDDIT_URL", "REDDIT_URL")
	if subredditURL == "" {
		t.Skip("missing subreddit URL; set REDDIT_SUBREDDIT_URL (or SUBREDDIT_URL/REDDIT_URL) in env or .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// First page
	resp1, err := ExtractSubredditPosts(ctx, subredditURL, "hot", "", 3, "")
	if err != nil {
		t.Fatalf("ExtractSubredditPosts (page 1) failed: %v", err)
	}
	if resp1 == nil {
		t.Fatalf("ExtractSubredditPosts (page 1) returned nil response")
	}

	// Test pagination consistency
	if resp1.HasMore && resp1.NextAfter == "" {
		t.Error("has_more=true but next_after is empty")
	}
	if !resp1.HasMore && resp1.NextAfter != "" {
		t.Error("has_more=false but next_after is not empty")
	}

	// If there's a next page, verify pagination works
	if resp1.HasMore && resp1.NextAfter != "" {
		// Reset context timeout for second request
		ctx2, cancel2 := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel2()

		resp2, err := ExtractSubredditPosts(ctx2, subredditURL, "hot", "", 3, resp1.NextAfter)
		if err != nil {
			t.Fatalf("ExtractSubredditPosts (page 2) failed: %v", err)
		}

		// Verify different content (posts should be different)
		if len(resp1.Posts) > 0 && len(resp2.Posts) > 0 {
			// The first post of page 2 should not be the same as page 1
			if resp1.Posts[0].PostLink == resp2.Posts[0].PostLink {
				t.Error("pagination not working correctly - got same posts on different pages")
			}
		}

		fmt.Printf("Pagination test: page1=%d posts, page2=%d posts, cursor=%s\n",
			len(resp1.Posts), len(resp2.Posts), resp1.NextAfter)
	}
}

func TestExtractSubredditPosts_Integration_Sorting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}

	loadDotEnvIfPresent()

	subredditURL := getFirstEnv("REDDIT_SUBREDDIT_URL", "SUBREDDIT_URL", "REDDIT_URL")
	if subredditURL == "" {
		t.Skip("missing subreddit URL; set REDDIT_SUBREDDIT_URL (or SUBREDDIT_URL/REDDIT_URL) in env or .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test different sorting options
	sorts := []string{"hot", "new"}
	for _, sort := range sorts {
		resp, err := ExtractSubredditPosts(ctx, subredditURL, sort, "", 3, "")
		if err != nil {
			t.Fatalf("ExtractSubredditPosts (sort=%s) failed: %v", sort, err)
		}
		if resp == nil {
			t.Fatalf("ExtractSubredditPosts (sort=%s) returned nil response", sort)
		}
		if len(resp.Posts) == 0 {
			t.Logf("Warning: sort=%s returned no posts", sort)
		}
		fmt.Printf("Sort test (sort=%s): %d posts returned\n", sort, len(resp.Posts))
	}
}

func TestExtractSubredditPosts_Integration_Filtering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}

	loadDotEnvIfPresent()

	subredditURL := getFirstEnv("REDDIT_SUBREDDIT_URL", "SUBREDDIT_URL", "REDDIT_URL")
	if subredditURL == "" {
		t.Skip("missing subreddit URL; set REDDIT_SUBREDDIT_URL (or SUBREDDIT_URL/REDDIT_URL) in env or .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Request more posts to increase chance of catching filtered posts
	resp, err := ExtractSubredditPosts(ctx, subredditURL, "hot", "", 25, "")
	if err != nil {
		t.Fatalf("ExtractSubredditPosts failed: %v", err)
	}
	if resp == nil {
		t.Fatalf("ExtractSubredditPosts returned nil response")
	}

	// Verify that returned posts are not deleted/removed
	for i, post := range resp.Posts {
		if strings.TrimSpace(strings.ToLower(post.Title)) == "[deleted]" ||
			strings.TrimSpace(strings.ToLower(post.Title)) == "[removed]" {
			t.Errorf("post %d should have been filtered (deleted/removed): %s", i, post.Title)
		}
	}

	fmt.Printf("Filtering test: %d posts returned after filtering deleted/removed\n", len(resp.Posts))
}

func TestExtractSubredditPosts_Integration_Validation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testCases := []struct {
		name      string
		url       string
		sort      string
		timeRange string
		limit     int
		wantErr   bool
		errType   error
	}{
		{
			name:    "invalid URL - empty",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid URL - malformed",
			url:     "not-a-valid-url",
			wantErr: true,
		},
		{
			name:    "invalid URL - not subreddit",
			url:     "https://example.com",
			wantErr: true,
		},
		{
			name:    "invalid sort",
			url:     "https://www.reddit.com/r/golang/",
			sort:    "invalid_sort",
			wantErr: true,
		},
		{
			name:      "invalid time range",
			url:       "https://www.reddit.com/r/golang/",
			sort:      "top",
			timeRange: "invalid_time",
			wantErr:   true,
		},
		{
			name:    "limit too low",
			url:     "https://www.reddit.com/r/golang/",
			limit:   0,
			wantErr: false, // Should use default
		},
		{
			name:    "limit too high",
			url:     "https://www.reddit.com/r/golang/",
			limit:   101,
			wantErr: true,
		},
		{
			name:    "limit negative",
			url:     "https://www.reddit.com/r/golang/",
			limit:   -1,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := ExtractSubredditPosts(ctx, tc.url, tc.sort, tc.timeRange, tc.limit, "")
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				// Check if it's a ValidationError for validation cases
				if strings.Contains(tc.name, "invalid") || strings.Contains(tc.name, "limit") {
					_, ok := err.(ValidationError)
					if !ok {
						// Check error message contains expected validation keywords
					 errMsg := err.Error()
						if !strings.Contains(errMsg, "invalid") && !strings.Contains(errMsg, "between") && !strings.Contains(errMsg, "required") {
							t.Errorf("expected ValidationError for %s, got: %v", tc.name, err)
						}
					}
				}
			} else if err != nil {
				// For non-error test cases, network errors are acceptable
				if !strings.Contains(err.Error(), "unexpected status") {
					t.Logf("Got error (may be network issue): %v", err)
				}
			}

			if !tc.wantErr && resp == nil {
				t.Error("expected response but got nil")
			}
		})
	}
}

func TestBuildRedditPostLink(t *testing.T) {
	testCases := []struct {
		name     string
		permalink string
		want     string
	}{
		{
			name:     "valid permalink",
			permalink: "/r/golang/comments/abc123/test_post/",
			want:     "https://www.reddit.com/r/golang/comments/abc123/test_post/",
		},
		{
			name:     "full URL",
			permalink: "https://www.reddit.com/r/golang/comments/abc123/",
			want:     "https://www.reddit.com/r/golang/comments/abc123/",
		},
		{
			name:     "empty permalink",
			permalink: "",
			want:     "",
		},
		{
			name:     "path traversal attack",
			permalink: "/r/golang/../../etc/passwd",
			want:     "",
		},
		{
			name:     "invalid format - missing /r/",
			permalink: "/golang/comments/abc123/",
			want:     "",
		},
		{
			name:     "invalid format - too short",
			permalink: "/r/",
			want:     "",
		},
		{
			name:     "whitespace only",
			permalink: "   ",
			want:     "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildRedditPostLink(tc.permalink)
			if got != tc.want {
				t.Errorf("buildRedditPostLink() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateSubredditURL(t *testing.T) {
	testCases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid subreddit URL",
			url:     "https://www.reddit.com/r/golang/",
			wantErr: false,
		},
		{
			name:    "valid subreddit URL without trailing slash",
			url:     "https://www.reddit.com/r/golang",
			wantErr: false,
		},
		{
			name:    "valid subreddit URL with http",
			url:     "http://www.reddit.com/r/golang/",
			wantErr: false,
		},
		{
			name:    "valid subreddit URL with old domain",
			url:     "https://www.reddit.com/r/programming",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid URL - malformed",
			url:     "not-a-url",
			wantErr: true,
		},
		{
			name:    "invalid URL - not subreddit",
			url:     "https://www.reddit.com/user/test",
			wantErr: true,
		},
		{
			name:    "invalid URL - missing /r/",
			url:     "https://www.reddit.com/golang/",
			wantErr: true,
		},
		{
			name:    "invalid URL - empty subreddit name",
			url:     "https://www.reddit.com/r//",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSubredditURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateSubredditURL() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
