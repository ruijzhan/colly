package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultSubredditSort  = "hot"
	defaultSubredditLimit = 20
	maxSubredditLimit     = 100
)

// ValidationError represents a client-side validation error.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

// SubredditPost represents a single post from a subreddit listing.
type SubredditPost struct {
	Title        string   `json:"title"`
	ImageURLs    []string `json:"image_urls,omitempty"`
	PostLink     string   `json:"post_link"`
	Score        int      `json:"score,omitempty"`
	Comments     int      `json:"comments,omitempty"`
	ExternalLink string   `json:"external_link,omitempty"`
}

// SubredditListResponse represents a subreddit listing response.
type SubredditListResponse struct {
	Posts     []SubredditPost `json:"posts"`
	NextAfter string          `json:"next_after,omitempty"`
	HasMore   bool            `json:"has_more"`
}

type redditListingResponse struct {
	Kind string `json:"kind"`
	Data struct {
		After    string `json:"after"`
		Children []struct {
			Kind string                `json:"kind"`
			Data redditListingPostData `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type redditListingPostData struct {
	Title             string `json:"title"`
	Author            string `json:"author"`
	Score             int    `json:"score"`
	NumComments       int    `json:"num_comments"`
	Selftext          string `json:"selftext"`
	Permalink         string `json:"permalink"`
	URL               string `json:"url"`
	IsSelf            bool   `json:"is_self"`
	PostHint          string `json:"post_hint"`
	IsGallery         bool   `json:"is_gallery"`
	IsVideo           bool   `json:"is_video"`
	RemovedByCategory string `json:"removed_by_category"`
	Preview           struct {
		Images []struct {
			Source struct {
				URL string `json:"url"`
			} `json:"source"`
		} `json:"images"`
	} `json:"preview"`
	MediaMetadata map[string]struct {
		Status string `json:"status"`
		E      string `json:"e"`
		M      string `json:"m"`
		S      struct {
			U string `json:"u"`
		} `json:"s"`
	} `json:"media_metadata"`
}

// ExtractSubredditPosts fetches a subreddit listing using Reddit JSON API.
func ExtractSubredditPosts(ctx context.Context, subredditURL, sort, timeRange string, limit int, after string) (*SubredditListResponse, error) {
	// Initialize logger for stderr output
	logger := log.New(os.Stderr, "[subreddit] ", log.LstdFlags|log.Lmsgprefix)

	subreddit, err := parseSubredditURL(subredditURL)
	if err != nil {
		logger.Printf("validation error: url=%s, err=%v", subredditURL, err)
		return nil, ValidationError{Message: err.Error()}
	}

	normalizedSort := normalizeSubredditSort(sort)
	if strings.TrimSpace(sort) != "" && normalizedSort == "" {
		logger.Printf("invalid sort parameter: sort=%s, subreddit=%s", sort, subreddit)
		return nil, ValidationError{Message: "invalid sort"}
	}
	if normalizedSort == "" {
		normalizedSort = defaultSubredditSort
	}
	if limit == 0 {
		limit = defaultSubredditLimit
	}
	if limit < 1 || limit > maxSubredditLimit {
		logger.Printf("invalid limit parameter: limit=%d, subreddit=%s", limit, subreddit)
		return nil, ValidationError{Message: fmt.Sprintf("limit must be between 1 and %d", maxSubredditLimit)}
	}
	if timeRange != "" && normalizedSort == "top" && !isValidTimeRange(timeRange) {
		logger.Printf("invalid time_range parameter: time_range=%s, subreddit=%s", timeRange, subreddit)
		return nil, ValidationError{Message: "invalid time_range"}
	}

	apiURL := fmt.Sprintf("https://www.reddit.com/r/%s/%s.json", subreddit, normalizedSort)
	query := url.Values{}
	query.Set("limit", fmt.Sprintf("%d", limit))
	if after != "" {
		query.Set("after", after)
	}
	if normalizedSort == "top" && timeRange != "" {
		query.Set("t", timeRange)
	}
	apiURL = apiURL + "?" + query.Encode()

	logger.Printf("fetching: subreddit=%s, sort=%s, limit=%d, after=%s", subreddit, normalizedSort, limit, after)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		logger.Printf("request creation failed: %v", err)
		return nil, err
	}
	req.Header.Set("User-Agent", apiUserAgent)

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Printf("request failed: subreddit=%s, err=%v", subreddit, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
			logger.Printf("subreddit unavailable: subreddit=%s, status=%d", subreddit, resp.StatusCode)
			return &SubredditListResponse{
				Posts:   []SubredditPost{},
				HasMore: false,
			}, nil
		}
		logger.Printf("unexpected response: subreddit=%s, status=%d", subreddit, resp.StatusCode)
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Printf("body read failed: subreddit=%s, err=%v", subreddit, err)
		return nil, err
	}

	var listing redditListingResponse
	if err := json.Unmarshal(bodyBytes, &listing); err != nil {
		logger.Printf("json unmarshal failed: subreddit=%s, err=%v", subreddit, err)
		return nil, err
	}

	posts := make([]SubredditPost, 0, len(listing.Data.Children))
	filteredCount := 0
	for _, child := range listing.Data.Children {
		if child.Kind != "t3" {
			continue
		}
		data := child.Data
		if isRemovedPost(data.Title, data.Selftext, data.RemovedByCategory) {
			filteredCount++
			continue
		}
		postLink := buildRedditPostLink(data.Permalink)
		if postLink == "" {
			logger.Printf("invalid permalink filtered: title=%s, permalink=%s", data.Title, data.Permalink)
			continue
		}

		images := collectPostImages(data)
		externalLink := ""
		if isExternalLinkURL(data.URL) {
			externalLink = data.URL
		}

		posts = append(posts, SubredditPost{
			Title:        data.Title,
			ImageURLs:    images,
			PostLink:     postLink,
			Score:        data.Score,
			Comments:     data.NumComments,
			ExternalLink: externalLink,
		})
	}

	nextAfter := strings.TrimSpace(listing.Data.After)
	logger.Printf("success: subreddit=%s, returned=%d, filtered=%d, has_more=%v, next_after=%s",
		subreddit, len(posts), filteredCount, nextAfter != "", nextAfter)

	return &SubredditListResponse{
		Posts:     posts,
		NextAfter: nextAfter,
		HasMore:   nextAfter != "",
	}, nil
}

func normalizeSubredditSort(sort string) string {
	sort = strings.ToLower(strings.TrimSpace(sort))
	switch sort {
	case "", "hot", "new", "top", "rising":
		return sort
	default:
		return ""
	}
}

func isValidTimeRange(timeRange string) bool {
	switch strings.ToLower(strings.TrimSpace(timeRange)) {
	case "day", "week", "month", "year", "all":
		return true
	default:
		return false
	}
}

// ValidateSubredditURL returns an error if the URL is empty or not a subreddit URL.
func ValidateSubredditURL(rawURL string) error {
	_, err := parseSubredditURL(rawURL)
	if err != nil {
		return ValidationError{Message: err.Error()}
	}
	return nil
}

func parseSubredditURL(rawURL string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", fmt.Errorf("url is required")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url")
	}
	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i < len(pathParts)-1; i++ {
		if pathParts[i] == "r" && pathParts[i+1] != "" {
			return pathParts[i+1], nil
		}
	}
	return "", fmt.Errorf("invalid subreddit url")
}

func buildRedditPostLink(permalink string) string {
	permalink = strings.TrimSpace(permalink)
	if permalink == "" {
		return ""
	}
	// If already a full URL, return as-is
	if strings.HasPrefix(permalink, "http://") || strings.HasPrefix(permalink, "https://") {
		return permalink
	}
	// Validate path format - must start with /r/
	if !strings.HasPrefix(permalink, "/r/") {
		return ""
	}
	// Ensure no path traversal attempts
	if strings.Contains(permalink, "..") {
		return ""
	}
	// Validate path segments and ensure reasonable structure
	parts := strings.Split(strings.Trim(permalink, "/"), "/")
	if len(parts) < 3 { // At least /r/subreddit should exist
		return ""
	}
	// First part must be "r"
	if parts[0] != "r" {
		return ""
	}
	// Second part should be the subreddit name (non-empty)
	if parts[1] == "" {
		return ""
	}
	return "https://www.reddit.com" + permalink
}

func isRemovedPost(title, selftext, removedCategory string) bool {
	removedCategory = strings.TrimSpace(removedCategory)
	if removedCategory != "" {
		return true
	}
	title = strings.TrimSpace(strings.ToLower(title))
	selftext = strings.TrimSpace(strings.ToLower(selftext))
	if title == "[deleted]" || title == "[removed]" {
		return true
	}
	if selftext == "[deleted]" || selftext == "[removed]" {
		return true
	}
	return false
}

func collectPostImages(data redditListingPostData) []string {
	var images []string

	if data.IsVideo {
		return images
	}

	if data.IsGallery && data.MediaMetadata != nil {
		for _, media := range data.MediaMetadata {
			if media.Status == "valid" && strings.EqualFold(media.E, "Image") && media.S.U != "" {
				imageURL := strings.ReplaceAll(media.S.U, "&amp;", "&")
				images = append(images, imageURL)
			}
		}
		if len(images) > 0 {
			return images
		}
	}

	if data.PostHint == "image" || isRedditImageURL(data.URL) {
		if strings.TrimSpace(data.URL) != "" {
			images = append(images, data.URL)
		}
	}

	if len(images) == 0 && len(data.Preview.Images) > 0 {
		for _, img := range data.Preview.Images {
			if img.Source.URL == "" {
				continue
			}
			images = append(images, strings.ReplaceAll(img.Source.URL, "&amp;", "&"))
		}
	}

	return images
}

func isExternalLinkURL(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if strings.HasSuffix(host, "reddit.com") || strings.HasSuffix(host, "redd.it") {
		return false
	}
	if strings.HasSuffix(host, "i.redd.it") || strings.HasSuffix(host, "preview.redd.it") || strings.HasSuffix(host, "v.redd.it") {
		return false
	}
	return true
}
