package utils

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCountryCodeNormalization(t *testing.T) {
	for input, want := range map[string]string{" id ": "ID", "sg,proxy": "SG", "NA": "NA", "UNKNOWN": "", "XX": "", "ZZ": "", "N/A": "", "Indonesia": "", "US": "US"} {
		require.Equal(t, want, NormalizeCountryCode(input), input)
	}
}

func TestCountryLookupBatchCacheAndNonPublicIPs(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		var ips []string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&ips))
		require.ElementsMatch(t, []string{"8.8.8.8", "1.1.1.1"}, ips)
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"ip":"8.8.8.8","country":"ID"},{"ip":"1.1.1.1","country":"SG"},{"ip":"9.9.9.9","country":"US"}]`))
	}))
	defer server.Close()
	resolver := NewCountryResolver(server.URL)
	values := []string{"8.8.8.8", "8.8.8.8:443", "::ffff:8.8.8.8", "1.1.1.1", "127.0.0.1", "192.168.1.1", "10.0.0.1", "100.64.1.1", "203.0.113.1", "2001:db8::1", "::1", "garbage"}
	for i := 0; i < 2; i++ {
		require.Equal(t, map[string]string{"8.8.8.8": "ID", "1.1.1.1": "SG"}, resolver.Resolve(context.Background(), values))
	}
	require.Equal(t, 1, calls)
}

func TestCountryLookupFailureAndCancellation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer server.Close()
	resolver := NewCountryResolver(server.URL)
	require.Empty(t, resolver.Resolve(context.Background(), []string{"8.8.8.8"}))
	require.Empty(t, resolver.Resolve(context.Background(), []string{"8.8.8.8"}))
	require.Equal(t, 1, calls, "temporarily cache failures to avoid retry storms")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	require.Empty(t, resolver.Resolve(ctx, []string{"1.1.1.1"}))
	require.Less(t, time.Since(start), time.Second)
}
