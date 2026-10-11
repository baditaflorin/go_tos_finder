package main

// Env-gated record/replay validation harness. It never runs under a plain
// `go test` (it needs TOS_VALIDATE=1) and the default test run never touches
// the network.
//
//	TOS_VALIDATE=1 TOS_DOMAINS=file.txt TOS_CACHE=dir TOS_OUT=out.jsonl \
//	  [TOS_CONC=4] [TOS_OFFLINE=1] go test -run TestValidateHarness -count=1 \
//	  [-cpuprofile cpu.out] .
//
// Every outbound GET made by the handler is recorded under TOS_CACHE the first
// time it is seen (record-through) and replayed byte-for-byte afterwards, so
// the old and the new detector can be compared on identical bytes. With
// TOS_OFFLINE=1 a cache miss is answered with a synthetic "replay miss" error
// instead of going live (use it for the CPU profile so only compute is timed).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type recordedResp struct {
	Status   int         `json:"status"`
	Header   http.Header `json:"header,omitempty"`
	Body     []byte      `json:"body,omitempty"`
	FinalURL string      `json:"final_url,omitempty"`
	Err      string      `json:"err,omitempty"`
	Millis   int64       `json:"ms"`
}

// directRT performs a real, direct (cache-less) fetch with the service's own
// SSRF-safe fallback client, following redirects exactly as the production
// fallback path does. Unlike the production fleetfetch transport it never
// reports "lookup go_infrastructure_fetch_cache: no such host" for a healthy
// origin, so the error strings recorded here classify the way a reachable
// fetch cache would let them.
type directRT struct{ c *http.Client }

func (d directRT) RoundTrip(req *http.Request) (*http.Response, error) { return d.c.Do(req) }

type recordingTransport struct {
	dir     string
	offline bool
	next    http.RoundTripper
}

func (t *recordingTransport) path(u *url.URL) string {
	h := sha1.Sum([]byte(u.String()))
	host := strings.NewReplacer(":", "_", "/", "_").Replace(u.Hostname())
	return filepath.Join(t.dir, host, hex.EncodeToString(h[:8])+".json")
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p := t.path(req.URL)
	if b, err := os.ReadFile(p); err == nil {
		var rr recordedResp
		if json.Unmarshal(b, &rr) == nil {
			return t.replay(req, rr)
		}
	}
	if t.offline {
		return nil, errors.New("replay miss: " + req.URL.String())
	}
	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	rr := recordedResp{Millis: time.Since(start).Milliseconds()}
	if err != nil {
		rr.Err = err.Error()
	} else {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
		_ = resp.Body.Close()
		rr.Status, rr.Header, rr.Body = resp.StatusCode, resp.Header, body
		if resp.Request != nil && resp.Request.URL != nil {
			rr.FinalURL = resp.Request.URL.String()
		}
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if b, e := json.Marshal(rr); e == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
	return t.replay(req, rr)
}

func (t *recordingTransport) replay(req *http.Request, rr recordedResp) (*http.Response, error) {
	if rr.Err != "" {
		return nil, errors.New(rr.Err)
	}
	final := req.URL
	if rr.FinalURL != "" {
		if u, err := url.Parse(rr.FinalURL); err == nil {
			final = u
		}
	}
	rq := req.Clone(req.Context())
	rq.URL = final
	h := rr.Header
	if h == nil {
		h = http.Header{}
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", rr.Status, http.StatusText(rr.Status)),
		StatusCode:    rr.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h.Clone(),
		Body:          io.NopCloser(bytes.NewReader(rr.Body)),
		ContentLength: int64(len(rr.Body)),
		Request:       rq,
	}, nil
}

func cpuNow() time.Duration {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

type harnessRow struct {
	Domain   string          `json:"domain"`
	HTTP     int             `json:"http"`
	WallMS   int64           `json:"wall_ms"`
	CPUMS    float64         `json:"cpu_ms,omitempty"`
	Response json.RawMessage `json:"response"`
}

func TestValidateHarness(t *testing.T) {
	if os.Getenv("TOS_VALIDATE") != "1" {
		t.Skip("set TOS_VALIDATE=1 (+TOS_DOMAINS/TOS_CACHE/TOS_OUT) to run the live/replay harness")
	}
	domainsFile, cacheDir, outFile := os.Getenv("TOS_DOMAINS"), os.Getenv("TOS_CACHE"), os.Getenv("TOS_OUT")
	if domainsFile == "" || cacheDir == "" || outFile == "" {
		t.Fatal("TOS_DOMAINS, TOS_CACHE and TOS_OUT are required")
	}
	conc, _ := strconv.Atoi(os.Getenv("TOS_CONC"))
	if conc <= 0 {
		conc = 1
	}
	offline := os.Getenv("TOS_OFFLINE") == "1"
	reps := 1
	if r, _ := strconv.Atoi(os.Getenv("TOS_REPS")); r > 1 {
		reps = r
	}

	f, err := os.Open(domainsFile)
	if err != nil {
		t.Fatal(err)
	}
	var domains []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if d := strings.TrimSpace(sc.Text()); d != "" {
			domains = append(domains, d)
		}
	}
	_ = f.Close()

	prev := clientHook
	clientHook = func(c *http.Client) {
		c.Transport = &recordingTransport{dir: cacheDir, offline: offline, next: directRT{c: fallbackClient()}}
	}
	resetSharedClient()
	t.Cleanup(func() { clientHook = prev; resetSharedClient() })

	out, err := os.Create(outFile)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var mu sync.Mutex
	enc := json.NewEncoder(out)

	run := func(d string) harnessRow {
		req := httptest.NewRequest(http.MethodGet, "/?api_key=default_token&url="+url.QueryEscape(d), nil)
		ctx, cancel := context.WithTimeout(req.Context(), 45*time.Second)
		defer cancel()
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		start := time.Now()
		c0 := cpuNow()
		Handler(rec, req)
		cpu := cpuNow() - c0
		body := rec.Body.Bytes()
		if !json.Valid(body) {
			body, _ = json.Marshal(map[string]string{"raw": string(body)})
		}
		return harnessRow{Domain: d, HTTP: rec.Code, WallMS: time.Since(start).Milliseconds(), CPUMS: float64(cpu.Microseconds()) / 1000, Response: body}
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, conc)
	for _, d := range domains {
		wg.Add(1)
		sem <- struct{}{}
		go func(d string) {
			defer wg.Done()
			defer func() { <-sem }()
			var row harnessRow
			for i := 0; i < reps; i++ {
				row = run(d)
			}
			mu.Lock()
			_ = enc.Encode(row)
			mu.Unlock()
		}(d)
	}
	wg.Wait()
}
