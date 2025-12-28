package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
)

type RedditPost struct {
	Title     string
	Link      string
	Comments  string
	Score     string
	Author    string
	PostedAt  string
	CrawledAt time.Time
}

func main() {
	fmt.Println("🚀 开始抓取 RetroFuturism subreddit...")

	posts := []RedditPost{}
	maxPages := 3   // 限制最多抓取 3 页
	maxPosts := 100 // 限制最多抓取 100 个帖子
	pageCount := 0

	// 创建 Collector
	c := colly.NewCollector(
		// 限制只访问 old.reddit.com 域名
		colly.AllowedDomains("old.reddit.com"),
		// 启用异步模式
		colly.Async(true),
		// 设置用户代理
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// 启用随机 User-Agent 以避免检测
	extensions.RandomUserAgent(c)

	// 设置速率限制 - 对 Reddit 友好一些
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*.reddit.com",
		Parallelism: 1,               // 降低并行度
		Delay:       2 * time.Second, // 基础延迟
		RandomDelay: 3 * time.Second, // 随机延迟
	})

	// 错误处理
	c.OnError(func(r *colly.Response, err error) {
		log.Printf("❌ 抓取失败 %s: %v", r.Request.URL, err)
	})

	// 请求前的日志
	c.OnRequest(func(r *colly.Request) {
		fmt.Printf("🔍 正在访问: %s\n", r.URL.String())
	})

	// 响应处理
	c.OnResponse(func(r *colly.Response) {
		fmt.Printf("✅ 获取响应: %s (状态码: %d)\n", r.Request.URL, r.StatusCode)
	})

	// 调试：先查看页面结构
	c.OnHTML("div#siteTable", func(e *colly.HTMLElement) {
		fmt.Printf("🔍 找到主要内容区域\n")
	})

	// 尝试多种可能的选择器
	c.OnHTML(".thing", func(e *colly.HTMLElement) {
		post := RedditPost{
			CrawledAt: time.Now(),
		}

		// 调试信息
		fmt.Printf("🎯 找到帖子元素，class: %s\n", e.Attr("class"))

		// 方法1: 使用原始选择器
		titleElement := e.DOM.Find("a[data-event-action=title]")
		if titleElement.Length() > 0 {
			post.Title = titleElement.Text()
			href, exists := titleElement.Attr("href")
			if exists {
				if href[0] == '/' {
					post.Link = "https://old.reddit.com" + href
				} else {
					post.Link = href
				}
			}
		} else {
			// 方法2: 尝试其他选择器
			titleElement = e.DOM.Find("a.title")
			if titleElement.Length() > 0 {
				post.Title = titleElement.Text()
				href, exists := titleElement.Attr("href")
				if exists {
					if href[0] == '/' {
						post.Link = "https://old.reddit.com" + href
					} else {
						post.Link = href
					}
				}
			} else {
				// 方法3: 直接查找所有链接
				allLinks := e.DOM.Find("a")
				if allLinks.Length() > 0 {
					firstLink := allLinks.Eq(0)
					post.Title = firstLink.Text()
					href, exists := firstLink.Attr("href")
					if exists && href != "" {
						if href[0] == '/' {
							post.Link = "https://old.reddit.com" + href
						} else {
							post.Link = href
						}
					}
				}
			}
		}

		// 提取评论链接
		commentsElement := e.DOM.Find("a[data-event-action=comments]")
		if commentsElement.Length() > 0 {
			href, exists := commentsElement.Attr("href")
			if exists {
				if href[0] == '/' {
					post.Comments = "https://old.reddit.com" + href
				} else {
					post.Comments = href
				}
			}
		}

		// 提取分数
		scoreElement := e.DOM.Find("[data-event-action=score]")
		if scoreElement.Length() > 0 {
			post.Score = scoreElement.Text()
		}

		// 提取作者
		authorElement := e.DOM.Find("a[data-author]")
		if authorElement.Length() > 0 {
			post.Author = authorElement.AttrOr("data-author", "")
		}

		// 提取发布时间
		timeElement := e.DOM.Find("time")
		if timeElement.Length() > 0 {
			post.PostedAt = timeElement.AttrOr("title", "")
		}

		fmt.Printf("📋 提取结果 - 标题: '%s', 链接: '%s'\n", post.Title, post.Link)

		// 只有当标题不为空时才添加到结果中
		if post.Title != "" && post.Title != " " {
			posts = append(posts, post)
			fmt.Printf("📝 找到帖子: %s\n", post.Title)

			// 检查是否达到最大帖子数量
			if len(posts) >= maxPosts {
				fmt.Printf("⏹️ 已达到最大帖子数量限制 (%d)\n", maxPosts)
				return
			}
		}
	})

	// 处理分页 - "下一页" 按钮
	c.OnHTML("span.next-button > a", func(e *colly.HTMLElement) {
		pageCount++
		if pageCount >= maxPages {
			fmt.Printf("⏹️ 已达到最大页数限制 (%d)\n", maxPages)
			return
		}

		nextPage := e.Attr("href")
		if nextPage != "" {
			fmt.Printf("📄 翻到下一页 (%d/%d): %s\n", pageCount, maxPages, nextPage)
			e.Request.Visit(nextPage)
		}
	})

	// 访问目标页面
	targetURL := "https://old.reddit.com/r/RetroFuturism/"
	fmt.Printf("🎯 目标URL: %s\n", targetURL)

	err := c.Visit(targetURL)
	if err != nil {
		log.Fatalf("❌ 无法访问目标页面: %v", err)
	}

	// 等待所有请求完成
	c.Wait()

	// 输出结果
	fmt.Printf("\n🎉 抓取完成！共找到 %d 个帖子\n\n", len(posts))

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("📋 RetroFuturism Subreddit 帖子列表")
	fmt.Println(strings.Repeat("=", 60))

	for i, post := range posts {
		fmt.Printf("%d. %s\n", i+1, post.Title)
		fmt.Printf("   🔗 链接: %s\n", post.Link)
		if post.Comments != "" {
			fmt.Printf("   💬 评论: %s\n", post.Comments)
		}
		if post.Score != "" {
			fmt.Printf("   ⭐ 评分: %s\n", post.Score)
		}
		if post.Author != "" {
			fmt.Printf("   👤 作者: %s\n", post.Author)
		}
		if post.PostedAt != "" {
			fmt.Printf("   🕐 发布时间: %s\n", post.PostedAt)
		}
		fmt.Println()
	}

	// 可选：保存到文件
	if len(posts) > 0 {
		fmt.Println("💾 提示：可以将结果保存到 CSV 或 JSON 文件")
	}
}
