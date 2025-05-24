package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

	workerCount := 50
	var workerWG sync.WaitGroup
	var taskWG sync.WaitGroup
	var taskCount atomic.Uint32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel() // cancel all workers
	}()

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
			for {
				select {
				case <-ctx.Done():
					return
				case url, ok := <-c.remainingURLs:
					if !ok {
						return
					}
					c.processURL(ctx, url, &taskWG, &taskCount)
				}
			}
		}()
	}

	workerWG.Wait()

	fmt.Println("Total Processed URL: ", taskCount.Load())

}

func (c *crawler) processURL(ctx context.Context, rawUrl string, taskWG *sync.WaitGroup, taskCount *atomic.Uint32) {
	defer taskWG.Done()

	select {
	case <-ctx.Done():
		return
	default:
	}
	c.visitedURLs.Store(rawUrl, true)
	fmt.Println("Visiting: ", rawUrl)
	client := http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", rawUrl, nil)
	if err != nil {
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.3;; en-US) AppleWebKit/602.45 (KHTML, like Gecko) Chrome/52.0.3750.323 Safari/536.7 Edge/10.62018")

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	taskCount.Add(1)

	if resp.StatusCode != http.StatusOK {
		return
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return
	}

	// remove these tags, not usefull
	doc.Find(`script, style, noscript, head, link, meta, title, 
	iframe, svg, canvas, img, video, audio, 
	map, area, object, embed, source, track, 
	template, picture, param`).Each(func(i int, s *goquery.Selection) {
		s.Remove()
	})

	// it stores content from page
	var content strings.Builder

	doc.Each(func(i int, s *goquery.Selection) {
		content.Write([]byte(strings.ToLower(s.Text())))
	})

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
					default:
						c.visitedURLs.Delete(next)
					}
				}
			}
		}
	})

}
