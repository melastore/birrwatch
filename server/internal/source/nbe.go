package source

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/melastore/birrwatch/internal/model"
)

// DefaultNBEAPI is the endpoint behind the rates table on nbe.gov.et.
//
// It is undocumented. The published page at /exchange/ ships an empty
// <tbody id="exchangeData2q1"> and fills it client-side, and the script that
// does so calls this endpoint with a date parameter. Reading the JSON directly
// is both simpler and sturdier than driving a browser to watch it build a
// table — and because the date is a parameter, history is reachable rather
// than only today.
const DefaultNBEAPI = "https://api.nbe.gov.et/api/filter-exchange-rates"

// nbeOrg is the Organization on the certificates NBE serves.
const nbeOrg = "National Bank of Ethiopia"

// NBE reads daily indicative rates from the National Bank of Ethiopia API.
type NBE struct {
	URL    string
	Client *http.Client

	// Date selects which day to request. Zero means today.
	Date time.Time
}

// NewNBE returns an NBE source pointed at the default endpoint.
func NewNBE() *NBE {
	return &NBE{URL: DefaultNBEAPI, Client: newNBEClient(pinnedTLS())}
}

// NewNBEStrict requires ordinary certificate validation. It will fail against
// NBE as currently configured; it exists so the relaxed path below is a
// deliberate choice rather than the only one on offer.
func NewNBEStrict() *NBE {
	return &NBE{URL: DefaultNBEAPI, Client: newNBEClient(nil)}
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
// "certificate is not valid for any names" — and trusting their certificate as
// a root does not help, because the problem is the missing SAN, not an
// untrusted chain. No configuration makes standard verification succeed here.
//
// Rather than switch verification off wholesale, this replaces Go's check with
// a narrower one: the leaf must name NBE. That is weaker than real PKI — it
// proves possession of a certificate bearing that name, not that the operator
// is NBE — so it is no defence against a determined man-in-the-middle. It is
// proportionate for public, non-sensitive, read-only data, and it still
// rejects a server presenting some unrelated certificate, which a blanket
// InsecureSkipVerify would accept without complaint.
//
// Every response is archived verbatim, so anything odd arriving this way stays
// auditable after the fact.
func pinnedTLS() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Go's name and chain validation is replaced, not merely relaxed:
		// VerifyPeerCertificate runs in its place.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("nbe tls: server presented no certificate")
			}
			leaf, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("nbe tls: parse leaf: %w", err)
			}

			named := strings.Contains(leaf.Subject.CommonName, "nbe.gov.et")
			for _, o := range leaf.Subject.Organization {
				if o == nbeOrg {
					named = true
				}
			}
			for _, d := range leaf.DNSNames {
				if strings.Contains(d, "nbe.gov.et") {
					named = true
				}
			}
			if !named {
				return fmt.Errorf("nbe tls: certificate does not name NBE (CN=%q O=%v SAN=%v)",
					leaf.Subject.CommonName, leaf.Subject.Organization, leaf.DNSNames)
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

// Fetch implements Source.
func (n *NBE) Fetch(ctx context.Context) ([]byte, error) {
	day := n.Date
	if day.IsZero() {
		day = time.Now().UTC()
	}
	url := fmt.Sprintf("%s?date=%s", n.URL, day.Format("2006-01-02"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "birrwatch/1.0 (+https://github.com/melastore/birrwatch)")

	resp, err := n.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return body, nil
}

// ErrNoRates is returned when a response carries no usable rows. A day with no
// publication (weekend, holiday) is reported this way rather than as success
// with nothing written.
var ErrNoRates = errors.New("no exchange rates in response")

// apiResponse mirrors the endpoint's shape.
//
// weighted_average arrives as a string in some responses and a number in
// others, so it is decoded as json.Number and parsed explicitly.
type apiResponse struct {
	Data []struct {
		Date     string `json:"date"`
		Currency struct {
			Name string `json:"name"`
			Code string `json:"code"`
		} `json:"currency"`
		WeightedAverage json.Number `json:"weighted_average"`
	} `json:"data"`
}

// Parse implements Source.
//
// NBE publishes one figure per currency — a weighted average of the previous
// day's interbank transactions — not a buy/sell pair. Both legs therefore carry
// the same value; the Rate type keeps two so that sources which do quote a
// spread fit the same shape.
func (n *NBE) Parse(raw []byte, fetchedAt time.Time) ([]model.Rate, error) {
	var resp apiResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, ErrNoRates
	}

	fallback := fetchedAt.UTC().Format("2006-01-02")
	var rates []model.Rate
	seen := map[string]bool{}

	for _, row := range resp.Data {
		code, ok := normalizeCurrency(row.Currency.Code)
		if !ok {
			// Fall back to the long name when the code field is blank.
			if code, ok = normalizeCurrency(row.Currency.Name); !ok {
				continue
			}
		}
		if seen[code] {
			continue
		}

		value, err := strconv.ParseFloat(strings.TrimSpace(row.WeightedAverage.String()), 64)
		if err != nil || value <= 0 {
			continue
		}

		date := fallback
		if d := parseAPIDate(row.Date); d != "" {
			date = d
		}

		seen[code] = true
		rates = append(rates, model.Rate{
			Source:   model.SourceNBE,
			Currency: code,
			Date:     date,
			Buying:   value,
			Selling:  value,
		})
	}

	if len(rates) == 0 {
		return nil, fmt.Errorf("%w: %d rows, none usable", ErrNoRates, len(resp.Data))
	}
	return rates, nil
}

// apiDateFormats covers the shapes the endpoint has been seen to return.
var apiDateFormats = []string{
	"2006-01-02",
	time.RFC3339,
	"2006-01-02T15:04:05.000000Z",
	"2006-01-02 15:04:05",
}

func parseAPIDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, f := range apiDateFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}
