package extractor

import (
	"context"
	"encoding/json"
	"testing"
)

func TestExtractRedditPostWithComments(t *testing.T) {
	// Using the URL provided by the user for testing
	redditURL := "https://www.reddit.com/r/RetroFuturism/comments/1pwtza4/ed_valigursky_ii/"

	ctx := context.Background()
	post, err := ExtractRedditPost(ctx, redditURL)
	if err != nil {
		t.Fatalf("ExtractRedditPost failed: %v", err)
	}

	// Verify basic post data
	if post.Title == "" {
		t.Error("Expected title to be non-empty")
	}
	if post.Author == "" {
		t.Error("Expected author to be non-empty")
	}
	if post.Score == "" {
		t.Error("Expected score to be non-empty")
	}
	if post.CommentCount == "" {
		t.Error("Expected comment count to be non-empty")
	}

	// Verify comments are extracted
	if len(post.Comments) == 0 {
		t.Error("Expected at least one comment to be extracted")
	}

	// Verify first comment has required fields
	for i, comment := range post.Comments {
		if comment.Body == "" {
			t.Errorf("Comment %d: Expected Body to be non-empty", i)
		}

		// Test nested replies
		if len(comment.Replies) > 0 {
			for j, reply := range comment.Replies {
				if reply.Body == "" {
					t.Errorf("Comment %d, Reply %d: Expected Body to be non-empty", i, j)
				}
			}
		}
	}

	// Verify JSON serialization works
	jsonData, err := json.MarshalIndent(post, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal post to JSON: %v", err)
	}

	t.Logf("Successfully extracted post with %d comments and %s comment count. JSON size: %d bytes",
		len(post.Comments), post.CommentCount, len(jsonData))
	t.Logf("Post Title: %s", post.Title)
	t.Logf("Post Author: %s", post.Author)
	t.Logf("Post Score: %s", post.Score)

	if len(post.Comments) > 0 {
		t.Logf("First Comment: %s",
			post.Comments[0].Body)
		if len(post.Comments[0].Replies) > 0 {
			t.Logf("First Reply: %s",
				post.Comments[0].Replies[0].Body)
		}
	}
}
