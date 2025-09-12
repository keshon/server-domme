package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/playwright-community/playwright-go"
)

// ChatSession holds the browser and context for a user session
type ChatSession struct {
	Browser  playwright.Browser
	Context  playwright.BrowserContext
	Page     playwright.Page
	LastUsed time.Time
	mu       sync.Mutex
}

var (
	session     *ChatSession
	sessionLock sync.Mutex
)

// humanType simulates human typing with randomness
func humanType(locator playwright.Locator, text string) error {
	for _, ch := range text {
		if err := locator.Type(string(ch)); err != nil {
			return err
		}
		time.Sleep(time.Millisecond * time.Duration(50+rand.Intn(200)))
	}
	return nil
}

// getOrCreateSession ensures a live session exists
func getOrCreateSession() (*ChatSession, error) {
	sessionLock.Lock()
	defer sessionLock.Unlock()

	// Reuse session if alive and <10min idle
	if session != nil && time.Since(session.LastUsed) < 10*time.Minute {
		session.mu.Lock()
		session.LastUsed = time.Now()
		session.mu.Unlock()
		return session, nil
	}

	// Initialize Playwright
	if err := playwright.Install(); err != nil {
		return nil, fmt.Errorf("install failed: %v", err)
	}
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("playwright run failed: %v", err)
	}

	// Launch browser
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false), // or "new" if supported
		Args: []string{
			"--no-sandbox",
			"--disable-blink-features=AutomationControlled",
			"--disable-web-security",
			"--disable-features=VizDisplayCompositor",
			"--headless=new",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("browser launch failed: %v", err)
	}

	// New context
	ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"),
		Viewport: &playwright.Size{
			Width:  1920,
			Height: 1080,
		},
		Locale: playwright.String("en-US"),
	})
	if err != nil {
		return nil, fmt.Errorf("context creation failed: %v", err)
	}

	// New page
	page, err := ctx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("page creation failed: %v", err)
	}

	// Navigate to DuckDuckGo AI Chat
	if _, err := page.Goto("https://duckduckgo.com/aichat", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		return nil, fmt.Errorf("goto failed: %v", err)
	}

	// Accept onboarding
	time.Sleep(2 * time.Second)
	page.Locator("div[role='presentation'] button[type='button']").Click()
	time.Sleep(1 * time.Second)

	session = &ChatSession{
		Browser:  browser,
		Context:  ctx,
		Page:     page,
		LastUsed: time.Now(),
	}

	return session, nil
}

func chatHandler(c *gin.Context) {
	var req struct {
		Message string `json:"message"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	sess, err := getOrCreateSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Locate input
	inputLocator := sess.Page.Locator("textarea[name='user-prompt']")
	if err := humanType(inputLocator.First(), req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "typing failed"})
		return
	}

	if err := inputLocator.First().Press("Enter"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "press enter failed"})
		return
	}

	// Wait for response
	responseLocator := sess.Page.Locator("#react-layout div[data-activeresponse='true']")
	if err := responseLocator.Last().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(30000),
		State:   playwright.WaitForSelectorStateVisible,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "response timeout"})
		return
	}

	time.Sleep(3 * time.Second) // allow typing animation

	// Fetch response
	response, err := responseLocator.Last().TextContent()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fetch response failed"})
		return
	}

	sess.LastUsed = time.Now()

	c.JSON(http.StatusOK, gin.H{
		"response": response,
	})
}

func main() {
	rand.Seed(time.Now().UnixNano())

	r := gin.Default()
	r.POST("/chat", chatHandler)
	r.GET("/status", func(c *gin.Context) {
		sessionLock.Lock()
		defer sessionLock.Unlock()
		status := "offline"
		if session != nil {
			status = "online"
		}
		c.JSON(http.StatusOK, gin.H{"status": status})
	})

	fmt.Println("Server running on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
