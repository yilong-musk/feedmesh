package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/yilong-musk/feedmesh/internal/feed"
)

// gzipPool reuses compressors: a gzip.Writer carries about 800 KB of state,
// and feed readers poll often.
var gzipPool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
	return w
}}

// acceptsGzip reports whether the request allows a gzip-coded response.
func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "gzip" && name != "*" {
			continue
		}
		q := strings.ReplaceAll(strings.ToLower(params), " ", "")
		if q == "q=0" || q == "q=0.0" || q == "q=0.00" || q == "q=0.000" {
			return false
		}
		return true
	}
	return false
}

// gzipWriter compresses the body of a response whose status carries one. The
// decision is made at WriteHeader, the last moment headers can change.
type gzipWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (g *gzipWriter) WriteHeader(code int) {
	if g.wroteHeader {
		g.ResponseWriter.WriteHeader(code)
		return
	}
	g.wroteHeader = true
	h := g.Header()
	if code >= http.StatusOK && code != http.StatusNoContent && code != http.StatusNotModified &&
		h.Get("Content-Encoding") == "" {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(g.ResponseWriter)
		g.gz = gz
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	if !g.wroteHeader {
		// net/http sniffs the type from the first bytes it sees; with the
		// body compressed it would sniff gzip, so do it here.
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(p))
		}
		g.WriteHeader(http.StatusOK)
	}
	if g.gz != nil {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipWriter) close() {
	if g.gz == nil {
		return
	}
	_ = g.gz.Close()
	g.gz.Reset(io.Discard)
	gzipPool.Put(g.gz)
	g.gz = nil
}

// withGzip compresses responses for clients that accept it. RSS and JSON
// shrink several times over, and feed readers re-download the whole document
// on every poll that is not answered with 304.
func (s *Server) withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// writeRSS renders f with a validator and answers a matching conditional
// request with 304 Not Modified, so a polling reader whose copy is current
// downloads nothing. Callers set Content-Type and the cache headers first; a
// 304 repeats them as RFC 9110 asks.
func (s *Server) writeRSS(w http.ResponseWriter, r *http.Request, f *feed.Feed) {
	body := feed.RenderRSS(f)
	etag := rssETag(body)
	w.Header().Set("ETag", etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// rssETag is a weak validator over the rendered document without its
// lastBuildDate. That element carries the read time, which some providers
// stamp on every uncached request even when no item changed; the documents
// are then semantically equivalent, which is exactly what a weak ETag claims.
// It is weak for a second reason: the same validator covers the identity and
// the gzip-coded representation.
func rssETag(body []byte) string {
	h := sha256.New()
	start := bytes.Index(body, []byte("<lastBuildDate>"))
	end := -1
	if start >= 0 {
		if n := bytes.Index(body[start:], []byte("</lastBuildDate>")); n >= 0 {
			end = start + n + len("</lastBuildDate>")
		}
	}
	if end > start && start >= 0 {
		h.Write(body[:start])
		h.Write(body[end:])
	} else {
		h.Write(body)
	}
	return `W/"` + hex.EncodeToString(h.Sum(nil)[:16]) + `"`
}

// etagMatches applies the weak comparison of If-None-Match (RFC 9110 13.1.2).
func etagMatches(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	want := strings.TrimPrefix(etag, "W/")
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimPrefix(strings.TrimSpace(candidate), "W/") == want {
			return true
		}
	}
	return false
}
