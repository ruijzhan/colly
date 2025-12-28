package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gocolly/colly/v2/cmd/server/extractor"
)

type extractRequest struct {
	URL string `json:"url"`
}

type apiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func main() {
	router := gin.Default()

	router.POST("/extract", func(c *gin.Context) {
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

	_ = router.Run(":8080")
}
