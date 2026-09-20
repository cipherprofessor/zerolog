package mutil

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fancyPusher implements CloseNotifier, Flusher, Hijacker, ReaderFrom and
// Pusher -- the shape WrapWriter already special-cases as "fancy", plus
// Pusher on top. Each method records that it was called, so tests can
// confirm calls on the wrapper actually reach this underlying writer through
// the extra embedding layer fancyPushWriter adds.
type fancyPusher struct {
	*httptest.ResponseRecorder
	pushed        string
	flushed       bool
	closeNotified bool
	hijacked      bool
	readFrom      bool
}

func (f *fancyPusher) Flush() {
	f.flushed = true
	f.ResponseRecorder.Flush()
}
func (f *fancyPusher) CloseNotify() <-chan bool {
	f.closeNotified = true
	return nil
}
func (f *fancyPusher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	f.hijacked = true
	return nil, nil, errors.New("not implemented")
}
func (f *fancyPusher) ReadFrom(r io.Reader) (int64, error) {
	f.readFrom = true
	return 0, nil
}
func (f *fancyPusher) Push(target string, opts *http.PushOptions) error {
	f.pushed = target
	return nil
}

// flushPusher implements only Flusher and Pusher -- the realistic shape of
// an HTTP/2 ResponseWriter, which does not support Hijacker or ReaderFrom.
type flushPusher struct {
	*httptest.ResponseRecorder
	pushed  string
	flushed bool
}

func (f *flushPusher) Flush() {
	f.flushed = true
	f.ResponseRecorder.Flush()
}
func (f *flushPusher) Push(target string, opts *http.PushOptions) error {
	f.pushed = target
	return nil
}

// fancyNoPusher implements the full "fancy" set but not Pusher.
type fancyNoPusher struct {
	*httptest.ResponseRecorder
}

func (f *fancyNoPusher) CloseNotify() <-chan bool { return nil }
func (f *fancyNoPusher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not implemented")
}
func (f *fancyNoPusher) ReadFrom(r io.Reader) (int64, error) {
	return 0, nil
}

func TestWrapWriterPreservesPusher_Fancy(t *testing.T) {
	underlying := &fancyPusher{ResponseRecorder: httptest.NewRecorder()}
	w := WrapWriter(underlying)

	pusher, ok := w.(http.Pusher)
	if !ok {
		t.Fatalf("expected WrapWriter's result to implement http.Pusher when the underlying ResponseWriter does")
	}
	if err := pusher.Push("/style.css", nil); err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if underlying.pushed != "/style.css" {
		t.Fatalf("expected Push to be forwarded to the underlying ResponseWriter, got pushed=%q", underlying.pushed)
	}

	// fancyPushWriter adds an extra layer of embedding on top of fancyWriter;
	// confirm the other four proxied methods still reach the underlying
	// writer through it, not just the newly-added Push.
	w.(http.Flusher).Flush()
	if !underlying.flushed {
		t.Errorf("expected Flush to be forwarded to the underlying ResponseWriter")
	}
	w.(http.CloseNotifier).CloseNotify()
	if !underlying.closeNotified {
		t.Errorf("expected CloseNotify to be forwarded to the underlying ResponseWriter")
	}
	_, _, _ = w.(http.Hijacker).Hijack() //nolint:errcheck // fake returns a fixed error; only forwarding matters here
	if !underlying.hijacked {
		t.Errorf("expected Hijack to be forwarded to the underlying ResponseWriter")
	}
	_, _ = w.(io.ReaderFrom).ReadFrom(nil)
	if !underlying.readFrom {
		t.Errorf("expected ReadFrom to be forwarded to the underlying ResponseWriter")
	}
}

func TestWrapWriterPreservesPusher_FlushOnly(t *testing.T) {
	underlying := &flushPusher{ResponseRecorder: httptest.NewRecorder()}
	w := WrapWriter(underlying)

	pusher, ok := w.(http.Pusher)
	if !ok {
		t.Fatalf("expected WrapWriter's result to implement http.Pusher when the underlying ResponseWriter does, even without CloseNotifier/Hijacker/ReaderFrom")
	}
	if err := pusher.Push("/style.css", nil); err != nil {
		t.Fatalf("Push returned error: %v", err)
	}
	if underlying.pushed != "/style.css" {
		t.Fatalf("expected Push to be forwarded to the underlying ResponseWriter, got pushed=%q", underlying.pushed)
	}

	w.(http.Flusher).Flush()
	if !underlying.flushed {
		t.Errorf("expected Flush to be forwarded to the underlying ResponseWriter")
	}
}

func TestWrapWriterDoesNotClaimPusherWhenUnsupported(t *testing.T) {
	// httptest.ResponseRecorder alone does not implement http.Pusher.
	underlying := httptest.NewRecorder()
	w := WrapWriter(underlying)

	if _, ok := w.(http.Pusher); ok {
		t.Fatalf("expected WrapWriter's result to NOT implement http.Pusher when the underlying ResponseWriter doesn't")
	}
}

func TestWrapWriterDoesNotClaimPusherForFancyWithoutPusher(t *testing.T) {
	// The full "fancy" set (CloseNotifier+Flusher+Hijacker+ReaderFrom)
	// without Pusher must still not claim Pusher support -- unchanged
	// behavior, but not exercised by any existing test.
	underlying := &fancyNoPusher{ResponseRecorder: httptest.NewRecorder()}
	w := WrapWriter(underlying)

	if _, ok := w.(http.Pusher); ok {
		t.Fatalf("expected WrapWriter's result to NOT implement http.Pusher when the underlying ResponseWriter doesn't, even for the fancy shape")
	}
}
