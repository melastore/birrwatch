package source

import (
	"regexp"
	"strings"
)

// currencyNames maps the long-form names NBE uses to ISO 4217 codes.
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

// normalizeCurrency resolves a label to an ISO 4217 code. It accepts a bare
// code ("USD"), a long name ("US Dollar"), or a name with a parenthesised code
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
