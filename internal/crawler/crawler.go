package crawler

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	whatwg "github.com/nlnwa/whatwg-url/url"
)

type crawler struct {
	domain        string
	startURL      string
	visitedURLs   sync.Map
	remainingURLs chan string
}

func NewCrawler() *crawler {
	return &crawler{
		visitedURLs:   sync.Map{},
		remainingURLs: make(chan string, 500),
	}
}

func (c *crawler) Visit(url string) error {
	whatwgUrl, err := whatwg.Parse(url)
	if err != nil {
		return fmt.Errorf("provide valid url")
	}

	c.domain = whatwgUrl.Host()

	c.startURL = whatwgUrl.Href(false)
	c.scrape()

	return nil
}

func (c *crawler) scrape() {

	workerCount := 200
	var workerWG sync.WaitGroup
	var taskWG sync.WaitGroup

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
				c.processURL(url, &taskWG)
			}
		}()
	}

	workerWG.Wait()

}

func (c *crawler) processURL(url string, taskWG *sync.WaitGroup) {
	defer taskWG.Done()
	c.visitedURLs.Store(url, true)

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
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
		if href, ok := s.Attr("href"); ok && strings.HasPrefix(href, "http") {
			if u, err := whatwg.Parse(href); err == nil && u.Host() == c.domain {
				next := u.Href(true)
				if _, loaded := c.visitedURLs.LoadOrStore(next, true); !loaded {
					select {
					case c.remainingURLs <- next:
						taskWG.Add(1)
						fmt.Println(next)
					default:
						c.visitedURLs.Delete(next)
					}
				}
			}
		}
	})

}
