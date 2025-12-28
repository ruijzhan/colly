# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Colly is a fast and elegant web scraping framework for Go (Gophers). This is version 2 (v2) of the framework, located at `github.com/gocolly/colly/v2`. The framework provides a clean interface for building web crawlers, scrapers, and spiders with features like automatic cookie handling, rate limiting, distributed scraping, and more.

## Development Commands

### Testing
```bash
# Run all tests with race detection and coverage
go test -race -v -coverprofile=coverage.txt -covermode=atomic ./...

# Run tests for a specific package
go test -v ./...

# Run a specific test
go test -v -run TestSpecificFunction
```

### Building and Validation
```bash
# Build the project
go build

# Install dependencies and check for issues
go get -a

# Format code validation (fails if formatting issues exist)
gofmt -l -d ./

# Lint code
golint -set_exit_status

# Vet code for potential issues
go vet -v ./...
```

### Code Generation and Tools
```bash
# Generate a new scraper template
go run cmd/colly/colly.go new --callbacks=html,response,error --hosts=example.com scraper.go

# Install golint for development
go install golang.org/x/lint/golint@latest
```

## Architecture Overview

### Core Components

1. **Collector (`colly.go`)**: The main scraper instance that orchestrates the entire scraping process
   - Manages HTTP requests, callbacks, and configuration
   - Handles rate limiting, parallelization, and request routing
   - Supports async/parallel scraping with `Collector.Wait()`

2. **HTTP Backend (`http_backend.go`)**: Manages HTTP client behavior and transport
   - Handles proxy configuration and connection management
   - Supports Google App Engine integration via `urlfetch`

3. **Callback System**: Event-driven architecture with multiple callback types:
   - `OnHTML`: HTML element parsing and extraction
   - `OnRequest`: Pre-request processing
   - `OnResponse`: Post-response handling
   - `OnError`: Error handling and retry logic
   - `OnScraped`: Completion callbacks

4. **Storage Layer (`storage/`)**: Pluggable storage backend for persistence
   - `InMemoryStorage`: Default in-memory storage for cookies and visited URLs
   - Interface allows custom storage implementations (Redis, databases, etc.)

5. **Extensions (`extensions/`)**: Optional add-on functionality
   - Random user agent rotation
   - Referer management
   - URL length filtering

6. **Debug Support (`debug/`)**: Development and debugging tools
   - Web debugger for real-time visualization
   - Log debugger for detailed logging

### Key Data Structures

- **Collector**: Main orchestrator with configuration options
- **Request**: Represents HTTP requests with metadata
- **Response**: HTTP response wrapper with parsing capabilities
- **HTMLElement**: Parsed HTML element with traversal methods
- **XMLElement**: XML parsing support (similar to HTMLElement)

### Module Organization

- `/`: Core framework files (colly.go, request.go, response.go, etc.)
- `cmd/colly/`: CLI tool for generating scraper templates
- `_examples/`: Comprehensive example implementations
- `extensions/`: Optional functionality extensions
- `storage/`: Storage backend implementations and interfaces
- `debug/`: Debugging and development tools
- `proxy/`: Proxy handling utilities
- `queue/`: Request queue management for distributed scraping

## Development Practices

### Testing Strategy
- Unit tests are co-located with source files (`*_test.go`)
- Integration tests demonstrate real-world scraping scenarios in `_examples/`
- Tests use race detection and coverage reporting
- Example files serve as both documentation and integration tests

### Code Organization
- Public APIs are clearly documented with examples
- Internal helper functions use camelCase naming
- Configuration options use struct fields with functional options pattern
- Callback registration uses variadic methods for flexibility

### Version Management
- This is v2 of the framework with Go module path `github.com/gocolly/colly/v2`
- Requires Go 1.24+ with toolchain go1.24.9
- Uses semantic versioning for releases

### Key Dependencies
- `github.com/PuerkitoBio/goquery`: jQuery-like HTML parsing
- `github.com/antchfx/htmlquery` & `xmlquery`: XPath support for HTML/XML
- `github.com/temoto/robotstxt`: Robots.txt compliance
- `github.com/gobwas/glob`: URL pattern matching
- `golang.org/x/net`: Extended networking capabilities

## Example Usage Patterns

The framework follows a callback-based pattern where developers register callbacks on a Collector and then initiate scraping with `Visit()`. Common patterns include:

1. **Basic Scraping**: Create collector, register HTML callback, visit starting URL
2. **Rate Limiting**: Configure delay and concurrency limits
3. **Domain Filtering**: Set AllowedDomains/DisallowedDomains
4. **Async Scraping**: Enable Async mode and use Wait() for completion
5. **Custom Storage**: Implement Storage interface for persistence

Review the `_examples/` directory for comprehensive usage examples covering real-world scenarios like authentication, file downloads, parallel scraping, and more.