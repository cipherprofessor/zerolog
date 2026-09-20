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
// Pusher on top.
type fancyPusher struct {
	*httptest.ResponseRecorder
	pushed string
}

func (f *fancyPusher) CloseNotify() <-chan bool { return nil }
func (f *fancyPusher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not implemented")
}
func (f *fancyPusher) ReadFrom(r io.Reader) (int64, error) {
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
	pushed string
}

func (f *flushPusher) Push(target string, opts *http.PushOptions) error {
	f.pushed = target
	return nil
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
}

func TestWrapWriterDoesNotClaimPusherWhenUnsupported(t *testing.T) {
	// httptest.ResponseRecorder alone does not implement http.Pusher.
	underlying := httptest.NewRecorder()
	w := WrapWriter(underlying)

	if _, ok := w.(http.Pusher); ok {
		t.Fatalf("expected WrapWriter's result to NOT implement http.Pusher when the underlying ResponseWriter doesn't")
	}
}
