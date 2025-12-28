package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
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

// RedditPost represents extracted information from a Reddit post
type RedditPost struct {
	Title         string   `json:"title"`
	Author        string   `json:"author"`
	PublishedTime string   `json:"published_time"`
	Score         string   `json:"score"`
	Comments      string   `json:"comments"`
	Content       string   `json:"content"`
	Images        []string `json:"images"`
}

// RedditAPIResponse represents the structure of Reddit's JSON API response
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

// parseRedditURL extracts subreddit and post ID from the URL.
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

func printPostJSON(post *RedditPost) error {
	b, err := json.MarshalIndent(post, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

// extractRedditPostFromAPI fetches post data from Reddit's JSON API.
func extractRedditPostFromAPI(redditURL string) (*RedditPost, error) {
	subreddit, postID, ok := parseRedditURL(redditURL)
	if !ok {
		return nil, fmt.Errorf("invalid reddit post url")
	}

	// Construct JSON API URL
	jsonURL := fmt.Sprintf("https://www.reddit.com/r/%s/comments/%s/.json", subreddit, postID)

	// Create HTTP request with user agent
	req, err := http.NewRequest("GET", jsonURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", apiUserAgent)

	// Make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	// Parse JSON response
	var apiResponse RedditAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, err
	}

	post := &RedditPost{}

	// Extract images and title
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
			post.Comments = fmt.Sprintf("%d", child.Data.NumComments)
			post.Content = child.Data.Selftext

			// Convert Unix timestamp to readable format
			if child.Data.CreatedUTC > 0 {
				post.PublishedTime = time.Unix(int64(child.Data.CreatedUTC), 0).Format("2006-01-02 15:04:05")
			}

			// Check if it's a gallery
			if child.Data.IsGallery && child.Data.MediaMetadata != nil {
				for _, media := range child.Data.MediaMetadata {
					if media.Status == "valid" && media.E == "Image" && media.S.U != "" {
						// Decode HTML entities
						imageURL := strings.ReplaceAll(media.S.U, "&amp;", "&")
						post.Images = append(post.Images, imageURL)
					}
				}
			} else if isRedditImageURL(child.Data.URL) {
				post.Images = append(post.Images, child.Data.URL)
			}

			return post, nil
		}
	}

	return post, nil
}

func extractRedditPostFromHTML(redditURL string) (*RedditPost, error) {
	// Create collector
	c := colly.NewCollector()

	// Set a realistic user agent
	c.UserAgent = htmlUserAgent

	// Initialize the RedditPost structure
	post := &RedditPost{
		Images: []string{},
	}

	// Extract the post title
	c.OnHTML(`h1`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Title, strings.TrimSpace(e.Text))
	})

	// Extract the post author
	c.OnHTML(`faceplate-hovercard faceplate-tracker a`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Author, strings.TrimSpace(e.Text))
	})

	// Extract the post published time
	c.OnHTML(`faceplate-timeago time`, func(e *colly.HTMLElement) {
		if post.PublishedTime != "" {
			return
		}
		setIfEmpty(&post.PublishedTime, strings.TrimSpace(e.Attr("datetime")))
		if post.PublishedTime == "" {
			setIfEmpty(&post.PublishedTime, strings.TrimSpace(e.Text))
		}
	})

	// Extract the post score (upvotes) - use the specific XPath path
	c.OnHTML(`shreddit-post div[class*="flex"] span span span faceplate-number`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Score, strings.TrimSpace(e.Text))
	})

	// Fallback: try to find faceplate-number elements that might contain the score
	c.OnHTML(`faceplate-number`, func(e *colly.HTMLElement) {
		if post.Score == "" {
			scoreText := strings.TrimSpace(e.Text)
			if scoreLikeRE.MatchString(scoreText) {
				post.Score = scoreText
			}
		}
	})

	// Extract the post comments count - use the specific XPath path
	c.OnHTML(`shreddit-post div[3] button span span[2] faceplate-number`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Comments, strings.TrimSpace(e.Text))
	})

	// Fallback: try to find comment count in button elements with faceplate-number
	c.OnHTML(`button span span faceplate-number`, func(e *colly.HTMLElement) {
		if post.Comments != "" {
			return
		}
		commentsText := strings.TrimSpace(e.Text)
		if !scoreLikeRE.MatchString(commentsText) {
			return
		}
		parent := e.DOM.Parent()
		buttonParent := parent.Parent()
		if buttonParent.Is("button") {
			post.Comments = commentsText
		}
	})

	// Extract the post content - try multiple selectors
	c.OnHTML(`shreddit-post-text-body p`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Content, strings.TrimSpace(e.Text))
	})

	// Fallback: try any div within post text body
	c.OnHTML(`shreddit-post-text-body div`, func(e *colly.HTMLElement) {
		setIfEmpty(&post.Content, strings.TrimSpace(e.Text))
	})

	// Extract from all preview.redd.it images
	c.OnHTML(`img[src*="preview.redd.it"]`, func(e *colly.HTMLElement) {
		src := e.Attr("src")
		if src != "" && !strings.Contains(src, "avatar") {
			post.Images = append(post.Images, src)
		}
	})

	// Handle errors
	c.OnError(func(r *colly.Response, err error) {
		fmt.Printf("Error while scraping: %v\n", err)
	})

	if err := c.Visit(redditURL); err != nil {
		return nil, err
	}

	// Wait for requests to complete
	c.Wait()

	return post, nil
}

func printPost(post *RedditPost) {
	fmt.Printf("Title: %s\n", post.Title)
	fmt.Printf("Author: %s\n", post.Author)
	fmt.Printf("Published Time: %s\n", post.PublishedTime)
	if post.Score != "" {
		fmt.Printf("Score: %s\n", post.Score)
	}
	if post.Comments != "" {
		fmt.Printf("Comments: %s\n", post.Comments)
	}
	if post.Content != "" {
		fmt.Printf("Content: %s\n", post.Content)
	}

	if len(post.Images) > 0 {
		fmt.Println("Images:")
		for i, img := range post.Images {
			fmt.Printf("  %d. %s\n", i+1, img)
		}
	} else {
		fmt.Println("No images found")
	}
}

func main() {
	var redditURL string
	flag.StringVar(&redditURL, "url", "", "Reddit post URL to extract information from")
	flag.Parse()

	if redditURL == "" {
		log.Println("Reddit post URL required")
		log.Println("Usage: go run main.go -url https://www.reddit.com/r/subreddit/comments/post_id/title/")
		os.Exit(1)
	}

	// Try to extract using Reddit JSON API first
	fmt.Printf("Extracting information from: %s\n\n", redditURL)

	post, err := extractRedditPostFromAPI(redditURL)
	if err != nil || post == nil || post.Title == "" {
		// Fallback to HTML scraping if JSON API fails
		fmt.Println("JSON API extraction failed, falling back to HTML scraping...")
		post, err = extractRedditPostFromHTML(redditURL)
		if err != nil {
			fmt.Printf("Failed to visit URL: %v\n", err)
			os.Exit(1)
		}
	}

	if err := printPostJSON(post); err != nil {
		fmt.Printf("Failed to marshal post as JSON: %v\n", err)
		os.Exit(1)
	}
}
