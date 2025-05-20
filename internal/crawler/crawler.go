package crawler

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var mediaExtensions = []string{
	".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", // images
	".svg", ".ico", // icons
	".mp4", ".webm", ".ogg", ".avi", ".mov", ".mkv", // videos
	".mp3", ".wav", ".flac", ".aac", // audio
	".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", // documents
	".zip", ".rar", ".tar", ".gz", ".7z", // archives
	".exe", ".bin", ".dmg", ".apk", // binaries
	".css", ".js", // optionally block static assets
}

func isMediaURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	for _, mediaExt := range mediaExtensions {
		if ext == mediaExt {
			return true
		}
	}
	return false
}

type crawler struct {
	domain        string
	startURL      string
	visitedURLs   sync.Map
	remainingURLs chan string
}

func NewCrawler() *crawler {
	return &crawler{
		visitedURLs:   sync.Map{},
		remainingURLs: make(chan string, 50000),
	}
}

func (c *crawler) Visit(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("provide valid url")
	}

	c.domain = parsedURL.Host

	c.startURL = parsedURL.String()
	c.scrape()

	return nil
}

func (c *crawler) scrape() {

	workerCount := 5
	var workerWG sync.WaitGroup
	var taskWG sync.WaitGroup
	var taskCount atomic.Uint32

	taskWG.Add(1)
	c.remainingURLs <- c.startURL

	go func() {
		taskWG.Wait()
		close(c.remainingURLs)
	}()

	for range workerCount {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for url := range c.remainingURLs {
				c.processURL(url, &taskWG, &taskCount)
			}
		}()
	}

	workerWG.Wait()

	fmt.Println("Total Processed URL: ", taskCount.Load())

}

func (c *crawler) processURL(rawUrl string, taskWG *sync.WaitGroup, taskCount *atomic.Uint32) {
	defer taskWG.Done()
	c.visitedURLs.Store(rawUrl, true)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(rawUrl)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		if href, ok := s.Attr("href"); ok {
			var next string

			// Decode href in case it's percent-encoded
			decodedHref, err := url.PathUnescape(href)
			if err != nil {
				decodedHref = href
			}

			// Try parsing decoded href
			parsedHref, err := url.Parse(decodedHref)
			if err != nil {
				return
			}

			var resolvedURL *url.URL

			if parsedHref.IsAbs() && parsedHref.Host == c.domain {
				resolvedURL = parsedHref
			} else {
				// Resolve relative URLs
				baseURL, err := url.Parse(rawUrl) // 'url' is your current page URL
				if err != nil {
					return
				}
				resolvedURL = baseURL.ResolveReference(parsedHref)

				if resolvedURL.Host != c.domain {
					return
				}
			}

			// Strip query and fragment
			resolvedURL.RawQuery = ""

			next = resolvedURL.Scheme + "://" + resolvedURL.Host + resolvedURL.EscapedPath()

			if !isMediaURL(next) {
				if _, loaded := c.visitedURLs.LoadOrStore(next, true); !loaded {
					select {
					case c.remainingURLs <- next:
						taskWG.Add(1)
						taskCount.Add(1)
						fmt.Println(next)
					default:
						c.visitedURLs.Delete(next)
					}
				}
			}
		}
	})

}
