package confluence

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

// Provider fetches pages from Confluence Cloud (v1 REST API) and yields them as scanner.Documents.
type Provider struct {
	baseURL  string // e.g. https://mycompany.atlassian.net/wiki
	email    string
	apiToken string
	spaces   []string // space keys to index, empty = all
	client   *http.Client
}

func New(baseURL, email, apiToken string, spaces []string) *Provider {
	return &Provider{
		baseURL:  strings.TrimRight(baseURL, "/"),
		email:    email,
		apiToken: apiToken,
		spaces:   spaces,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// v1 API response types

type v1Page struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Title  string `json:"title"`
	Space  struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"space"`
	Body struct {
		Storage struct {
			Value string `json:"value"`
		} `json:"storage"`
	} `json:"body"`
	Version struct {
		Number int    `json:"number"`
		When   string `json:"when"`
		By     struct {
			DisplayName string `json:"displayName"`
			Email       string `json:"email"`
		} `json:"by"`
	} `json:"version"`
	Links struct {
		WebUI string `json:"webui"`
		Self  string `json:"self"`
	} `json:"_links"`
}

type v1PageListResponse struct {
	Results []v1Page `json:"results"`
	Start   int      `json:"start"`
	Limit   int      `json:"limit"`
	Size    int      `json:"size"`
	Links   struct {
		Next string `json:"next"`
	} `json:"_links"`
}

type v1Space struct {
	ID     int    `json:"id"`
	Key    string `json:"key"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type v1SpaceListResponse struct {
	Results []v1Space `json:"results"`
	Start   int       `json:"start"`
	Limit   int       `json:"limit"`
	Size    int       `json:"size"`
	Links   struct {
		Next string `json:"next"`
	} `json:"_links"`
}

// Scan fetches all pages from configured spaces and returns them as Documents.
func (p *Provider) Scan(ctx context.Context) (<-chan scanner.Document, <-chan error) {
	docs := make(chan scanner.Document, 50)
	errs := make(chan error, 10)

	go func() {
		defer close(docs)
		defer close(errs)

		spaces, err := p.resolveSpaces(ctx)
		if err != nil {
			errs <- fmt.Errorf("listing spaces: %w", err)
			return
		}

		for _, sp := range spaces {
			if err := p.scanSpace(ctx, sp, docs, errs); err != nil {
				select {
				case errs <- fmt.Errorf("scanning space %s: %w", sp.Key, err):
				default:
				}
			}
		}
	}()

	return docs, errs
}

func (p *Provider) resolveSpaces(ctx context.Context) ([]v1Space, error) {
	var allSpaces []v1Space
	endpoint := "/rest/api/space?limit=50"

	for endpoint != "" {
		var resp v1SpaceListResponse
		if err := p.get(ctx, endpoint, &resp); err != nil {
			return nil, err
		}
		allSpaces = append(allSpaces, resp.Results...)

		if resp.Links.Next != "" {
			endpoint = resp.Links.Next
		} else {
			endpoint = ""
		}
	}

	if len(p.spaces) == 0 {
		return allSpaces, nil
	}

	// Filter to configured space keys
	wantKeys := make(map[string]bool)
	for _, k := range p.spaces {
		wantKeys[strings.ToUpper(k)] = true
	}

	var filtered []v1Space
	for _, sp := range allSpaces {
		if wantKeys[strings.ToUpper(sp.Key)] {
			filtered = append(filtered, sp)
		}
	}
	return filtered, nil
}

func (p *Provider) scanSpace(ctx context.Context, sp v1Space, docs chan<- scanner.Document, errs chan<- error) error {
	endpoint := fmt.Sprintf("/rest/api/content?spaceKey=%s&type=page&limit=50&expand=body.storage,version,space", sp.Key)

	for endpoint != "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var resp v1PageListResponse
		if err := p.get(ctx, endpoint, &resp); err != nil {
			return err
		}

		for _, pg := range resp.Results {
			if pg.Status != "current" {
				continue
			}

			doc := p.pageToDocument(pg, sp)

			select {
			case docs <- doc:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if resp.Links.Next != "" {
			endpoint = resp.Links.Next
		} else {
			endpoint = ""
		}
	}

	return nil
}

func (p *Provider) pageToDocument(pg v1Page, sp v1Space) scanner.Document {
	// Convert storage format HTML to plain text with markdown-ish headings
	content := htmlToText(pg.Body.Storage.Value)

	// Prepend title as heading
	fullContent := fmt.Sprintf("# %s\n\n%s", pg.Title, content)

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(fullContent)))
	relPath := fmt.Sprintf("confluence/%s/%s.wiki", sp.Key, sanitisePath(pg.Title))
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("confluence:%s:%s", sp.Key, pg.ID))))

	webURL := p.baseURL + pg.Links.WebUI

	modTime := time.Now()
	if pg.Version.When != "" {
		if t, err := time.Parse(time.RFC3339, pg.Version.When); err == nil {
			modTime = t
		}
	}

	return scanner.Document{
		ID:          id,
		Path:        webURL,
		RelPath:     relPath,
		Repo:        fmt.Sprintf("confluence/%s", sp.Key),
		RepoRoot:    "",
		Category:    scanner.CategoryDocs,
		Language:    "confluence",
		FileType:    scanner.FileTypeWiki,
		SourceType:  scanner.SourceTypeConfluence,
		Content:     fullContent,
		ContentHash: hash,
		ModTime:     modTime,
		Metadata: map[string]string{
			"source":     "confluence",
			"space_key":  sp.Key,
			"space_name": sp.Name,
			"page_id":    pg.ID,
			"page_title": pg.Title,
			"version":    fmt.Sprintf("%d", pg.Version.Number),
			"web_url":    webURL,
			"author":     pg.Version.By.DisplayName,
		},
	}
}

func (p *Provider) get(ctx context.Context, endpoint string, result interface{}) error {
	var reqURL string
	if strings.HasPrefix(endpoint, "http") {
		reqURL = endpoint
	} else {
		reqURL = p.baseURL + endpoint
	}

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.SetBasicAuth(p.email, p.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("confluence API %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// htmlToText strips HTML tags and converts common Confluence storage format
// elements to readable plain text with markdown-style headings.
func htmlToText(html string) string {
	if html == "" {
		return ""
	}

	s := html

	// Convert headings to markdown
	for i := 6; i >= 1; i-- {
		prefix := strings.Repeat("#", i)
		re := regexp.MustCompile(fmt.Sprintf(`(?i)<h%d[^>]*>(.*?)</h%d>`, i, i))
		s = re.ReplaceAllString(s, "\n"+prefix+" $1\n")
	}

	// Convert lists
	s = regexp.MustCompile(`(?i)<li[^>]*>`).ReplaceAllString(s, "\n- ")
	s = regexp.MustCompile(`(?i)</li>`).ReplaceAllString(s, "")

	// Convert paragraphs and line breaks
	s = regexp.MustCompile(`(?i)<br\s*/?>|<p[^>]*>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?i)</p>`).ReplaceAllString(s, "\n")

	// Convert code blocks (Confluence macro format)
	s = regexp.MustCompile(`(?is)<ac:structured-macro[^>]*ac:name="code"[^>]*>.*?<ac:plain-text-body>\s*<!\[CDATA\[(.*?)\]\]>\s*</ac:plain-text-body>\s*</ac:structured-macro>`).ReplaceAllString(s, "\n```\n$1\n```\n")

	// Convert tables to simple text
	s = regexp.MustCompile(`(?i)<t[dh][^>]*>`).ReplaceAllString(s, " | ")
	s = regexp.MustCompile(`(?i)</tr>`).ReplaceAllString(s, "\n")

	// Convert links - extract href text
	s = regexp.MustCompile(`(?i)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(s, "$2 ($1)")

	// Convert bold/italic/strong
	s = regexp.MustCompile(`(?i)<(strong|b)[^>]*>(.*?)</(strong|b)>`).ReplaceAllString(s, "**$2**")
	s = regexp.MustCompile(`(?i)<(em|i)[^>]*>(.*?)</(em|i)>`).ReplaceAllString(s, "*$2*")

	// Convert inline code
	s = regexp.MustCompile(`(?i)<code[^>]*>(.*?)</code>`).ReplaceAllString(s, "`$1`")

	// Strip Confluence macros (info panels, expand, etc.) but keep their body text
	s = regexp.MustCompile(`(?is)<ac:structured-macro[^>]*>.*?</ac:structured-macro>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)<ac:[^>]*>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)</ac:[^>]*>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)<ri:[^>]*>`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`(?i)</ri:[^>]*>`).ReplaceAllString(s, "")

	// Strip remaining tags
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")

	// Decode common HTML entities
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&rarr;", "->")
	s = strings.ReplaceAll(s, "&larr;", "<-")
	s = strings.ReplaceAll(s, "&rsquo;", "'")
	s = strings.ReplaceAll(s, "&lsquo;", "'")
	s = strings.ReplaceAll(s, "&rdquo;", "\"")
	s = strings.ReplaceAll(s, "&ldquo;", "\"")
	s = strings.ReplaceAll(s, "&mdash;", " -- ")
	s = strings.ReplaceAll(s, "&ndash;", "-")
	s = strings.ReplaceAll(s, "&hellip;", "...")

	// Clean up excessive whitespace
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)

	return s
}

// sanitisePath converts a page title to a safe filesystem-like path.
func sanitisePath(title string) string {
	s := strings.ToLower(title)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 100 {
		s = s[:100]
	}
	return url.PathEscape(s)
}
