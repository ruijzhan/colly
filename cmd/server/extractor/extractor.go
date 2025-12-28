package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
)

const (
	apiUserAgent  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	htmlUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
)

var (
	redditURLRE = regexp.MustCompile(`/r/([^/]+)/comments/([a-z0-9]+)/`)
	scoreLikeRE = regexp.MustCompile(`^\d+\.?[\d]*[kK]?$`)
)

// Comment represents a Reddit comment with nested replies.
type Comment struct {
	Body    string    `json:"body"`
	Replies []Comment `json:"replies,omitempty"`
}

// RedditPost represents extracted information from a Reddit post.
type RedditPost struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	PublishedTime string   `json:"published_time"`
	Score         string   `json:"score"`
	CommentCount  string   `json:"comment_count"`
	Content       string   `json:"content"`
	Images        []string `json:"images"`
	Comments      []Comment `json:"comments"`
}

// RedditAPIResponse represents the structure of Reddit's JSON API response.
type RedditAPIResponse []struct {
	Kind string `json:"kind"`
	Data struct {
		Children []struct {
			Kind string `json:"kind"`
			Data struct {
				Title         string  `json:"title"`
				Author        string  `json:"author"`
				CreatedUTC    float64 `json:"created_utc"`
				Score         int     `json:"score"`
				NumComments   int     `json:"num_comments"`
				Selftext      string  `json:"selftext"`
				IsGallery     bool    `json:"is_gallery"`
				URL           string  `json:"url"`
				MediaMetadata map[string]struct {
					Status string `json:"status"`
					E      string `json:"e"`
					M      string `json:"m"`
					S      struct {
						U string `json:"u"`
					} `json:"s"`
				} `json:"media_metadata"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// ValidateRedditURL returns an error if the URL is empty or malformed.
func ValidateRedditURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("url is required")
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid url")
	}
	return nil
}

// ExtractRedditPost extracts post data from Reddit by trying JSON API first,
// falling back to HTML scraping if needed.
func ExtractRedditPost(ctx context.Context, redditURL string) (*RedditPost, error) {
	if err := ValidateRedditURL(redditURL); err != nil {
		return nil, err
	}
	post, err := extractRedditPostFromAPI(ctx, redditURL)
	if err != nil || post == nil || post.Title == "" {
		post, err = extractRedditPostFromHTML(ctx, redditURL)
		if err != nil {
			return nil, err
		}
	}
	return post, nil
}

func parseRedditURL(redditURL string) (string, string, bool) {
	matches := redditURLRE.FindStringSubmatch(redditURL)
	if len(matches) < 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}

func isRedditImageURL(url string) bool {
	return strings.Contains(url, "preview.redd.it") ||
		strings.Contains(url, "i.redd.it")
}

func setIfEmpty(target *string, value string) {
	if *target == "" && value != "" {
		*target = value
	}
}

func extractRedditPostFromAPI(ctx context.Context, redditURL string) (*RedditPost, error) {
	subreddit, postID, ok := parseRedditURL(redditURL)
	if !ok {
		return nil, fmt.Errorf("invalid reddit post url")
	}

	jsonURL := fmt.Sprintf("https://www.reddit.com/r/%s/comments/%s/.json", subreddit, postID)

	req, err := http.NewRequestWithContext(ctx, "GET", jsonURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", apiUserAgent)

	client := &http.Client{
		Timeout: 12 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	// Read the entire response body first to enable multiple parsing passes
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResponse RedditAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResponse); err != nil {
		return nil, err
	}

	post := &RedditPost{}

	// First pass: Extract post data from the first element (t3 post)
	for _, item := range apiResponse {
		if item.Kind != "Listing" {
			continue
		}
		for _, child := range item.Data.Children {
			if child.Kind != "t3" {
				continue
			}
			post.Title = child.Data.Title
			post.Author = child.Data.Author
			post.Score = fmt.Sprintf("%d", child.Data.Score)
			post.CommentCount = fmt.Sprintf("%d", child.Data.NumComments)
			post.Content = child.Data.Selftext

			if child.Data.CreatedUTC > 0 {
				post.PublishedTime = time.Unix(int64(child.Data.CreatedUTC), 0).Format("2006-01-02 15:04:05")
			}

			if child.Data.IsGallery && child.Data.MediaMetadata != nil {
				for _, media := range child.Data.MediaMetadata {
					if media.Status == "valid" && media.E == "Image" && media.S.U != "" {
						imageURL := strings.ReplaceAll(media.S.U, "&amp;", "&")
						post.Images = append(post.Images, imageURL)
					}
				}
			} else if isRedditImageURL(child.Data.URL) {
				post.Images = append(post.Images, child.Data.URL)
			}
		}
	}

	// Second pass: Extract comments from the second element (t1 comments)
	if len(apiResponse) >= 2 {
		var rawResponse []json.RawMessage
		if err := json.Unmarshal(bodyBytes, &rawResponse); err == nil && len(rawResponse) >= 2 {
			var commentsListing struct {
				Kind string `json:"kind"`
				Data struct {
					Children []json.RawMessage `json:"children"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rawResponse[1], &commentsListing); err == nil {
				post.Comments = parseCommentListings(commentsListing.Data.Children)
			}
		}
	}

	return post, nil
}

// parseCommentListings parses comment listings from raw JSON messages.
func parseCommentListings(children []json.RawMessage) []Comment {
	comments := make([]Comment, 0, len(children))
	for _, childRaw := range children {
		var child struct {
			Kind string `json:"kind"`
			Data struct {
				Body    string          `json:"body"`
				Replies json.RawMessage `json:"replies"`
			} `json:"data"`
		}
		if err := json.Unmarshal(childRaw, &child); err != nil {
			continue
		}
		if child.Kind != "t1" {
			continue
		}

		comment := Comment{
			Body: child.Data.Body,
		}

		// Parse nested replies
		if len(child.Data.Replies) > 0 && string(child.Data.Replies) != `""` && string(child.Data.Replies) != "" {
			var replies struct {
				Kind string `json:"kind"`
				Data struct {
					Children []json.RawMessage `json:"children"`
				} `json:"data"`
			}
			if err := json.Unmarshal(child.Data.Replies, &replies); err == nil {
				if replies.Kind == "Listing" && len(replies.Data.Children) > 0 {
					comment.Replies = parseCommentListings(replies.Data.Children)
				}
			}
		}

		comments = append(comments, comment)
	}
	return comments
}

func extractRedditPostFromHTML(ctx context.Context, redditURL string) (*RedditPost, error) {
	c := colly.NewCollector()
	c.UserAgent = htmlUserAgent
	c.SetRequestTimeout(12 * time.Second)

	post := &RedditPost{
		Images: []string{},
	}

	c.OnHTML(`h1`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Title, strings.TrimSpace(e.Text))
	})

	c.OnHTML(`faceplate-hovercard faceplate-tracker a`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Author, strings.TrimSpace(e.Text))
	})

	c.OnHTML(`faceplate-timeago time`, func(e *colly.HTMLElement) {
		if post.PublishedTime != "" {
			return
		}
		setIfEmpty(&post.PublishedTime, strings.TrimSpace(e.Attr("datetime")))
		if post.PublishedTime == "" {
			setIfEmpty(&post.PublishedTime, strings.TrimSpace(e.Text))
		}
	})

	c.OnHTML(`shreddit-post div[class*="flex"] span span span faceplate-number`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Score, strings.TrimSpace(e.Text))
	})

	c.OnHTML(`faceplate-number`, func(e *colly.HTMLElement) {
		if post.Score == "" {
			scoreText := strings.TrimSpace(e.Text)
			if scoreLikeRE.MatchString(scoreText) {
				post.Score = scoreText
			}
		}
	})

	c.OnHTML(`shreddit-post div[3] button span span[2] faceplate-number`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.CommentCount, strings.TrimSpace(e.Text))
	})

	c.OnHTML(`button span span faceplate-number`, func(e *colly.HTMLElement) {
		if post.CommentCount != "" {
			return
		}
		commentsText := strings.TrimSpace(e.Text)
		if !scoreLikeRE.MatchString(commentsText) {
			return
		}
		parent := e.DOM.Parent()
		buttonParent := parent.Parent()
		if buttonParent.Is("button") {
			post.CommentCount = commentsText
		}
	})

	c.OnHTML(`shreddit-post-text-body p`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Content, strings.TrimSpace(e.Text))
	})

	c.OnHTML(`shreddit-post-text-body div`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Content, strings.TrimSpace(e.Text))
	})

	c.OnHTML(`img[src*="preview.redd.it"]`, func(e *colly.HTMLElement) {
		src := e.Attr("src")
		if src != "" && !strings.Contains(src, "avatar") {
			post.Images = append(post.Images, src)
		}
	})

	c.OnError(func(r *colly.Response, err error) {
		_ = r
	})

	if err := c.Visit(redditURL); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	c.Wait()
	return post, nil
}
