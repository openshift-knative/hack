package maven

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"
)

const (
	RedHatMavenGA = "https://maven.repository.redhat.com/ga"
)

// Metadata represents the structure of the XML data
type Metadata struct {
	XMLName    xml.Name   `xml:"metadata" json:"metadata"`
	GroupID    string     `xml:"groupId" json:"groupId"`
	ArtifactID string     `xml:"artifactId" json:"artifactId"`
	Versioning Versioning `xml:"versioning"`
}

type Versioning struct {
	Latest      string   `xml:"latest"`
	Release     string   `xml:"release"`
	Versions    Versions `xml:"versions"`
	LastUpdated string   `xml:"lastUpdated"`
}

type Versions struct {
	Version []string `xml:"version"`
}

func ScrapRedHatMavenRegistry(url string) ([]Metadata, error) {
	return scrape(url, 0)
}

func list(doc *html.Node) (*html.Node, error) {
	var list *html.Node
	var crawler func(*html.Node)
	crawler = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "ul" {
			for _, a := range node.Attr {
				if a.Key == "id" && a.Val == "contents" {
					list = node
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			crawler(child)
		}
	}
	crawler(doc)
	if list != nil {
		return list, nil
	}
	return nil, errors.New("missing <ul> in the node tree")
}

func scrape(u string, retry int) ([]Metadata, error) {
	var metadata []Metadata
	metadataLock := &sync.Mutex{}
	log.Printf("scraping %s", u)

	time.Sleep(time.Duration(retry) * time.Second)

	if !strings.HasSuffix(u, "/") {
		u += "/"
	}

	resp, err := http.Get(u)
	if err != nil {
		log.Printf("Retrying scape due to error: %s", err.Error())
		if retry < 10 {
			return scrape(u, retry+1)
		}
		return metadata, fmt.Errorf("scraping %s returned: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("%s was not found", u)
		// Ignore not found errors
		return metadata, nil
	}

	if resp.StatusCode >= 400 {
		if resp.StatusCode >= 500 && retry < 10 {
			return scrape(u, retry+1)
		}
		return metadata, fmt.Errorf("GET %s returned %v: %w", u, resp.StatusCode, err)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed parsing %s body: %w", u, err)
	}
	listContent, err := list(doc)
	if err != nil {
		return nil, fmt.Errorf("failed finding list node %s: %w", u, err)
	}

	hasVersion := regexp.MustCompile(".*([0-9]+)\\.([0-9]+)\\.([0-9]+).*")

	g := &errgroup.Group{}
	g.SetLimit(50)

	for li := listContent.FirstChild; li != nil; li = li.NextSibling {
		if li.Type == html.ElementNode && li.Data == "li" {
			for a := li.FirstChild; a != nil; a = a.NextSibling {
				if a.Type == html.ElementNode && a.Data == "a" {

					for _, attr := range a.Attr {
						parsed, _ := url.Parse(u)
						parsed.Path = filepath.Join(parsed.Path, attr.Val)
						nextURL := parsed.String()

						if attr.Key == "href" && attr.Val == "maven-metadata.xml" {
							m, err := parseMavenMetadata(nextURL, 0)
							if err != nil {
								return metadata, fmt.Errorf("failed parsing metadata file %s: %w", nextURL, err)
							}
							metadataLock.Lock()
							metadata = append(metadata, m)
							metadataLock.Unlock()
						}

						if attr.Key == "href" && (!strings.HasSuffix(attr.Val, "/") || attr.Val == "../" || hasVersion.MatchString(attr.Val)) {
							// parent directory or single file
							break
						}

						if attr.Key == "href" {
							g.Go(func() error {
								m, err := scrape(nextURL, 0)
								if err != nil {
									return fmt.Errorf("failed scraping %s: %w", nextURL, err)
								}
								metadataLock.Lock()
								metadata = append(metadata, m...)
								metadataLock.Unlock()
								return nil
							})
							break
						}
					}
				}
			}
		}
	}

	if err := g.Wait(); err != nil {
		return metadata, fmt.Errorf("failed scraping %s: %w", u, err)
	}

	log.Printf("%s finisehd scrape", u)

	return metadata, nil
}

func parseMavenMetadata(u string, retry int) (Metadata, error) {
	time.Sleep(time.Duration(retry) * time.Second)

	log.Printf("parsing metadata file %s", u)

	metadata := Metadata{}

	resp, err := http.Get(u)
	if err != nil {
		log.Printf("Retrying getting metadata due to error: %s", err.Error())
		if retry < 10 {
			return parseMavenMetadata(u, retry+1)
		}
		return metadata, fmt.Errorf("parsing metadate GET %s returned: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return metadata, fmt.Errorf("GET %s returned %v: %w", u, resp.StatusCode, err)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return metadata, fmt.Errorf("failed reading metadata body %s: %w", u, err)
	}

	if err := xml.Unmarshal(bytes, &metadata); err != nil {
		return metadata, fmt.Errorf("failed to unmarshal metadata body %s: %w", u, err)
	}

	return metadata, nil
}
