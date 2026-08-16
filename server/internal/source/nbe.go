package source

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/melastore/birrwatch/internal/model"
)

// DefaultNBEURL is the National Bank of Ethiopia rates page.
//
// Not /exchange-rate/ — that page renders its figures client-side through a
// charting plugin and its served HTML contains no table at all. /exchange/ is
// the one that ships the rates as markup.
const DefaultNBEURL = "https://nbe.gov.et/exchange/"

// nbeSubjectCN is the Common Name on the certificate NBE presents.
const nbeSubjectCN = "*.nbe.gov.et"

// nbeOrg is the Organization on that certificate.
const nbeOrg = "National Bank of Ethiopia"

// NBE scrapes the National Bank of Ethiopia rates page.
type NBE struct {
	URL    string
	Client *http.Client
}

// NewNBE returns an NBE source pointed at the default page.
func NewNBE() *NBE {
	return &NBE{
		URL:    DefaultNBEURL,
		Client: newNBEClient(pinnedTLS()),
	}
}

// NewNBEStrict returns a source that requires ordinary certificate validation.
// It will fail against NBE as currently configured; it exists so the relaxed
// path is a deliberate choice rather than the only one on offer.
func NewNBEStrict() *NBE {
	return &NBE{URL: DefaultNBEURL, Client: newNBEClient(nil)}
}

func newNBEClient(tlsCfg *tls.Config) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = tlsCfg
	return &http.Client{Timeout: 30 * time.Second, Transport: tr}
}

// pinnedTLS builds a TLS config for a host that cannot pass normal validation.
//
// NBE serves a self-signed certificate with no subjectAltName extension. Go has
// ignored the legacy CommonName field since 1.15, so the handshake fails with
// "certificate is not valid for any names" — and adding their certificate as a
// trusted root does not help, because the failure is the missing SAN, not an
// untrusted chain. There is no configuration that makes standard verification
// succeed against this server.
//
// Rather than switch verification off wholesale, this disables Go's built-in
// check and substitutes a narrower one: the leaf must carry NBE's own subject.
// That is weaker than a real PKI — it proves possession of a certificate
// bearing that name, not that the operator is NBE — so it is not a defence
// against a determined man-in-the-middle. It is enough for public,
// non-sensitive, read-only data, and it still rejects a server presenting some
// unrelated certificate, which blanket InsecureSkipVerify would happily accept.
//
// Every response is archived verbatim, so anything odd that arrives this way
// stays auditable after the fact.
func pinnedTLS() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Go's own name and chain validation is replaced, not merely relaxed:
		// VerifyPeerCertificate below runs in its place and rejects anything
		// that is not NBE's certificate.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("nbe tls: server presented no certificate")
			}
			leaf, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("nbe tls: parse leaf: %w", err)
			}
			if leaf.Subject.CommonName != nbeSubjectCN {
				return fmt.Errorf("nbe tls: unexpected certificate CN %q (want %q)",
					leaf.Subject.CommonName, nbeSubjectCN)
			}
			if !slices.Contains(leaf.Subject.Organization, nbeOrg) {
				return fmt.Errorf("nbe tls: unexpected certificate organization %v (want %q)",
					leaf.Subject.Organization, nbeOrg)
			}
			if now := time.Now(); now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
				return fmt.Errorf("nbe tls: certificate outside validity window (%s to %s)",
					leaf.NotBefore.Format(time.RFC3339), leaf.NotAfter.Format(time.RFC3339))
			}
			return nil
		},
	}
}

// Name implements Source.
func (n *NBE) Name() string { return model.SourceNBE }

// Fetch implements Source. It sets a browser-like User-Agent because the site
// sits behind a filter that resets connections from default Go clients.
func (n *NBE) Fetch(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := n.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", n.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", n.URL, resp.StatusCode)
	}
	// Cap the read so a misbehaving upstream cannot exhaust memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", n.URL, err)
	}
	return body, nil
}

// ErrNoRateTable is returned when no table in the document scores high enough
// to be a rate table. This is the signal that the page layout changed.
var ErrNoRateTable = errors.New("no exchange rate table found in document")

// Parse implements Source.
//
// The page is a WordPress site whose markup changes without notice, so this
// does not hardcode CSS selectors. It extracts every table, scores each one on
// how well its header row matches the columns a rate table must have, and takes
// the winner. A layout change moves the table or renames a wrapper class; it
// rarely renames "Currency", "Buying" and "Selling" all at once.
func (n *NBE) Parse(raw []byte, fetchedAt time.Time) ([]model.Rate, error) {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	tables := collectTables(doc)
	if len(tables) == 0 {
		return nil, ErrNoRateTable
	}

	best, bestScore, bestCols := [][]string(nil), 0, columnMap{}
	for _, t := range tables {
		cols, score := scoreTable(t)
		if score > bestScore {
			best, bestScore, bestCols = t, score, cols
		}
	}
	// Currency plus at least one numeric column is the minimum viable table.
	if bestScore < 2 {
		return nil, ErrNoRateTable
	}

	date := extractDate(doc, fetchedAt)

	var rates []model.Rate
	seen := map[string]bool{}
	for _, row := range best[1:] {
		rate, ok := rowToRate(row, bestCols, date)
		if !ok || seen[rate.Currency] {
			continue
		}
		seen[rate.Currency] = true
		rates = append(rates, rate)
	}
	if len(rates) == 0 {
		return nil, fmt.Errorf("%w: matched a table but no row parsed", ErrNoRateTable)
	}
	return rates, nil
}

// columnMap records which column index holds which field. -1 means absent.
type columnMap struct {
	currency int
	buying   int
	selling  int
}

func newColumnMap() columnMap { return columnMap{currency: -1, buying: -1, selling: -1} }

// scoreTable inspects a table's header row and reports which columns it found.
// The score is the number of recognised columns.
func scoreTable(rows [][]string) (columnMap, int) {
	cols := newColumnMap()
	if len(rows) < 2 {
		return cols, 0
	}
	score := 0
	for i, h := range rows[0] {
		h = strings.ToLower(strings.TrimSpace(h))
		switch {
		case cols.currency == -1 && (strings.Contains(h, "currency") || strings.Contains(h, "curr.")):
			cols.currency, score = i, score+1
		// "cash buying" and "transactional buying" both count; first wins.
		case cols.buying == -1 && strings.Contains(h, "buying"):
			cols.buying, score = i, score+1
		case cols.buying == -1 && strings.Contains(h, "buy"):
			cols.buying, score = i, score+1
		case cols.selling == -1 && strings.Contains(h, "selling"):
			cols.selling, score = i, score+1
		case cols.selling == -1 && strings.Contains(h, "sell"):
			cols.selling, score = i, score+1
		}
	}
	// A table with a currency column and no numbers is not a rate table.
	if cols.currency == -1 || (cols.buying == -1 && cols.selling == -1) {
		return cols, 0
	}
	return cols, score
}

// rowToRate converts one data row into a Rate. It returns false for separator
// rows, footnotes and anything whose currency cannot be identified.
func rowToRate(row []string, cols columnMap, date string) (model.Rate, bool) {
	at := func(i int) string {
		if i < 0 || i >= len(row) {
			return ""
		}
		return row[i]
	}

	code, ok := normalizeCurrency(at(cols.currency))
	if !ok {
		return model.Rate{}, false
	}

	buy, buyOK := parseAmount(at(cols.buying))
	sell, sellOK := parseAmount(at(cols.selling))
	switch {
	case !buyOK && !sellOK:
		return model.Rate{}, false
	case !buyOK:
		buy = sell
	case !sellOK:
		sell = buy
	}

	return model.Rate{
		Source:   model.SourceNBE,
		Currency: code,
		Date:     date,
		Buying:   buy,
		Selling:  sell,
	}, true
}

var amountCleaner = regexp.MustCompile(`[^0-9.]`)

// parseAmount pulls a number out of a cell, tolerating thousands separators,
// stray currency symbols and non-breaking spaces.
func parseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, ",", "")
	s = amountCleaner.ReplaceAllString(s, "")
	// Guard against a cell like "12.34.56" produced by aggressive stripping.
	if strings.Count(s, ".") > 1 || s == "" || s == "." {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// currencyNames maps the long-form names NBE prints to ISO 4217 codes.
var currencyNames = map[string]string{
	"us dollar": "USD", "usdollar": "USD", "u.s. dollar": "USD", "dollar": "USD",
	"pound sterling": "GBP", "sterling pound": "GBP", "british pound": "GBP", "pound": "GBP",
	"euro":         "EUR",
	"japanese yen": "JPY", "yen": "JPY",
	"swiss franc":    "CHF",
	"swedish kroner": "SEK", "swedish krona": "SEK", "swedish kronor": "SEK",
	"norwegian kroner": "NOK", "norwegian krone": "NOK",
	"danish kroner": "DKK", "danish krone": "DKK",
	"canadian dollar":   "CAD",
	"australian dollar": "AUD",
	"saudi riyal":       "SAR", "saudi rial": "SAR",
	"uae dirham": "AED", "u.a.e. dirham": "AED", "arab emirates dirham": "AED",
	"kuwaiti dinar": "KWD",
	"chinese yuan":  "CNY", "chinese yuan renminbi": "CNY", "yuan": "CNY",
	"indian rupee":       "INR",
	"south african rand": "ZAR", "rand": "ZAR",
	"djibouti franc":  "DJF",
	"kenyan shilling": "KES",
	"sudanese pound":  "SDG",
	"qatari riyal":    "QAR", "qatar riyal": "QAR",
	"bahraini dinar": "BHD",
	"omani rial":     "OMR",
	"turkish lira":   "TRY",
	"ethiopian birr": "ETB", "birr": "ETB",
}

var isoCode = regexp.MustCompile(`^[A-Z]{3}$`)

// normalizeCurrency resolves a cell to an ISO 4217 code. It accepts a bare code
// ("USD"), a long name ("US Dollar"), or a name with a parenthesised code
// ("US Dollar (USD)").
func normalizeCurrency(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}

	// A parenthesised ISO code is the most reliable signal when present.
	if i, j := strings.LastIndex(s, "("), strings.LastIndex(s, ")"); i != -1 && j > i+1 {
		if c := strings.ToUpper(strings.TrimSpace(s[i+1 : j])); isoCode.MatchString(c) {
			return c, true
		}
	}

	if c := strings.ToUpper(s); isoCode.MatchString(c) {
		return c, true
	}

	key := strings.ToLower(strings.Join(strings.Fields(s), " "))
	if c, ok := currencyNames[key]; ok {
		return c, true
	}
	// Fall back to a contains match so "US Dollar - Cash" still resolves.
	for name, code := range currencyNames {
		if len(name) >= 5 && strings.Contains(key, name) {
			return code, true
		}
	}
	return "", false
}

// dateFormats are tried in order against text found near the table.
var dateFormats = []string{
	"2 January 2006", "02 January 2006", "January 2, 2006", "January 02, 2006",
	"2 Jan 2006", "02 Jan 2006", "Jan 2, 2006", "2006-01-02", "02/01/2006", "01/02/2006",
}

var dateHint = regexp.MustCompile(`(?i)(\d{4}-\d{2}-\d{2})|(\d{1,2}\s+[A-Za-z]{3,9},?\s+\d{4})|([A-Za-z]{3,9}\s+\d{1,2},?\s+\d{4})|(\d{1,2}/\d{1,2}/\d{4})`)

// extractDate looks for a publication date in the document text, falling back
// to the fetch date. NBE publishes on business days only, so the fallback keeps
// a weekend scrape attributed to the day it was actually taken rather than
// silently backdating it.
func extractDate(doc *html.Node, fetchedAt time.Time) string {
	text := nodeText(doc)
	for _, m := range dateHint.FindAllString(text, 20) {
		m = strings.TrimSpace(strings.ReplaceAll(m, ",", ""))
		for _, f := range dateFormats {
			f = strings.ReplaceAll(f, ",", "")
			if t, err := time.Parse(f, m); err == nil {
				// Reject dates that cannot be a publication date.
				if t.Year() >= 2000 && !t.After(fetchedAt.AddDate(0, 0, 1)) {
					return t.Format("2006-01-02")
				}
			}
		}
	}
	return fetchedAt.Format("2006-01-02")
}

// collectTables walks the document and returns each table as a grid of cell
// text. Rows are taken from tr elements and cells from th/td alike, so a header
// expressed with td still parses.
func collectTables(root *html.Node) [][][]string {
	var tables [][][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" {
			if grid := tableGrid(n); len(grid) > 1 {
				tables = append(tables, grid)
			}
			// Do not descend: nested tables are rare and would double-count.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return tables
}

func tableGrid(table *html.Node) [][]string {
	var grid [][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "tr" {
			var row []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					row = append(row, cellText(c))
				}
			}
			if len(row) > 0 {
				grid = append(grid, row)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)
	return grid
}

// cellText flattens a cell to a single whitespace-collapsed string.
func cellText(n *html.Node) string {
	return strings.Join(strings.Fields(nodeText(n)), " ")
}

// nodeText concatenates all text below n, skipping script and style content.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteString(" ")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
