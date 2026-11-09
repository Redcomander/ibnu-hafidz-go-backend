package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"
)

func NormalizeCountryCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(strings.SplitN(value, ",", 2)[0]))
	if len(value) != 2 {
		return ""
	}
	region, err := language.ParseRegion(value)
	if err != nil || !region.IsCountry() {
		return ""
	}
	return region.String()
}

var nonPublicRanges = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"),
}

func PublicIP(value string) string {
	ip, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		address, parseErr := netip.ParseAddrPort(strings.TrimSpace(value))
		if parseErr != nil {
			return ""
		}
		ip = address.Addr()
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return ""
	}
	for _, prefix := range nonPublicRanges {
		if prefix.Contains(ip) {
			return ""
		}
	}
	return ip.String()
}

type countryCacheEntry struct {
	code    string
	expires time.Time
}

type CountryResolver struct {
	endpoint string
	client   *http.Client
	gate     chan struct{}
	cache    map[string]countryCacheEntry
}

func NewCountryResolver(endpoint string) *CountryResolver {
	if endpoint == "" {
		endpoint = "https://api.country.is/"
	}
	return &CountryResolver{endpoint: endpoint, client: &http.Client{
		Timeout:       1500 * time.Millisecond,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}, gate: make(chan struct{}, 1), cache: make(map[string]countryCacheEntry)}
}

var defaultCountryResolver *CountryResolver
var defaultCountryResolverOnce sync.Once

func LookupIPCountries(ctx context.Context, ips []string) map[string]string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("IP_COUNTRY_LOOKUP_ENABLED")), "false") {
		return map[string]string{}
	}
	// main loads .env after package initialization; read configuration lazily.
	defaultCountryResolverOnce.Do(func() {
		defaultCountryResolver = NewCountryResolver(os.Getenv("IP_COUNTRY_API_URL"))
	})
	return defaultCountryResolver.Resolve(ctx, ips)
}

// Resolve returns only public-IP matches. At most 100 distinct IPs are sent per
// batch. No request path, username, cookie, or token is sent to the provider.
func (r *CountryResolver) Resolve(parent context.Context, values []string) map[string]string {
	ctx, cancel := context.WithTimeout(parent, 1800*time.Millisecond)
	defer cancel()
	result := make(map[string]string)
	select {
	case r.gate <- struct{}{}:
		defer func() { <-r.gate }()
	case <-ctx.Done():
		return result
	}
	now := time.Now()
	missing := []string{}
	seen := make(map[string]bool)
	for _, value := range values {
		ip := PublicIP(value)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		if cached, ok := r.cache[ip]; ok && cached.expires.After(now) {
			if cached.code != "" {
				result[ip] = cached.code
			}
		} else {
			missing = append(missing, ip)
		}
	}
	for start := 0; start < len(missing) && ctx.Err() == nil; start += 100 {
		end := start + 100
		if end > len(missing) {
			end = len(missing)
		}
		batch := missing[start:end]
		body, _ := json.Marshal(batch)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
		if err != nil {
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		matches, success := r.lookup(req)
		// Bound memory even if many unique IPs visit the application.
		if len(r.cache)+len(batch) > 4096 {
			r.cache = make(map[string]countryCacheEntry)
		}
		for _, ip := range batch {
			code := matches[ip]
			ttl := 30 * time.Second
			if success {
				ttl = 5 * time.Minute
			}
			if code != "" {
				ttl = 24 * time.Hour
				result[ip] = code
			}
			r.cache[ip] = countryCacheEntry{code: code, expires: time.Now().Add(ttl)}
		}
	}
	return result
}

func (r *CountryResolver) lookup(req *http.Request) (map[string]string, bool) {
	matches := make(map[string]string)
	response, err := r.client.Do(req)
	if err != nil {
		return matches, false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return matches, false
	}
	var rows []struct {
		IP      string `json:"ip"`
		Country string `json:"country"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&rows); err != nil {
		return matches, false
	}
	for _, row := range rows {
		ip, code := PublicIP(row.IP), NormalizeCountryCode(row.Country)
		if ip != "" && code != "" {
			matches[ip] = code
		}
	}
	return matches, true
}
