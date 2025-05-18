package main

import "web-search-engine/internal/crawler"

func main() {

	c := crawler.NewCrawler()
	c.Visit("https://pkg.go.dev/github.com/PuerkitoBio/goquery")

}
