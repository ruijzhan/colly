package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gocolly/colly/v2/cmd/server/extractor"
)

type extractRequest struct {
	URL string `json:"url"`
}

type subredditListRequest struct {
	URL       string `json:"url"`
	Sort      string `json:"sort"`
	TimeRange string `json:"time_range"`
	Limit     int    `json:"limit"`
	After     string `json:"after"`
}

type apiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

	router := gin.Default()

	router.POST("/api/reddit/extract", func(c *gin.Context) {
		var req extractRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, apiResponse{
				Success: false,
				Error:   "invalid json body",
			})
			return
		}

		if err := extractor.ValidateRedditURL(req.URL); err != nil {
			c.JSON(http.StatusBadRequest, apiResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		post, err := extractor.ExtractRedditPost(ctx, req.URL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, apiResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, apiResponse{
			Success: true,
			Data:    post,
		})
	})

	router.POST("/api/subreddit/posts", func(c *gin.Context) {
		var req subredditListRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, apiResponse{
				Success: false,
				Error:   "invalid json body",
			})
			return
		}

		if err := extractor.ValidateSubredditURL(req.URL); err != nil {
			c.JSON(http.StatusBadRequest, apiResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()

		resp, err := extractor.ExtractSubredditPosts(ctx, req.URL, req.Sort, req.TimeRange, req.Limit, req.After)
		if err != nil {
			var validationErr extractor.ValidationError
			if errors.As(err, &validationErr) {
				c.JSON(http.StatusBadRequest, apiResponse{
					Success: false,
					Error:   validationErr.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, apiResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, apiResponse{
			Success: true,
			Data:    resp,
		})
	})

	_ = router.Run(fmt.Sprintf(":%d", *port))
}
