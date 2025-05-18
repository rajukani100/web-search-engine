package crawler

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/PuerkitoBio/goquery"
	whatwg "github.com/nlnwa/whatwg-url/url"
)

type crawler struct {
	domain        string
	visitedURLs   sync.Map
	remainingURLs chan string
}

func NewCrawler() *crawler {
	return &crawler{
		visitedURLs:   sync.Map{},
		remainingURLs: make(chan string),
	}
}

func (c *crawler) Visit(url string) error {
	whatwgUrl, err := whatwg.Parse(url)
	if err != nil {
		return fmt.Errorf("provide valid url")
	}

	c.domain = whatwgUrl.Host()

	c.scrape(url)

	return nil
}

func (c *crawler) scrape(url string) error {
	whatwgUrl, err := whatwg.Parse(url)
	if err != nil {
		return fmt.Errorf("provide valid url")
	}

	if whatwgUrl.Host() != c.domain {
		return fmt.Errorf("other domain not allowed")
	}

	res, err := http.Get(url)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	c.visitedURLs.Store(url, true)

	if res.StatusCode != 200 {
		return fmt.Errorf("bad status code")
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return err
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		if newURL, exist := s.Attr("href"); exist {
			// if _, isVisisted := c.visitedURLs.Load(newURL); !isVisisted {
			// 	fmt.Println(newURL)
			// }
			fmt.Println(newURL)

		}
	})
	return nil
}
