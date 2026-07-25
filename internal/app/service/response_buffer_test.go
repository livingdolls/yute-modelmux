package service

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestReadBoundedResponseBody(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader([]byte("ok")))}
	got, err := readBoundedResponseBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Fatalf("got %q", got)
	}
}

func TestReadBoundedResponseBodyRejectsOversizedBody(t *testing.T) {
	body := io.LimitReader(zeroReader{}, maxBufferedUpstreamResponseBytes+1)
	resp := &http.Response{Body: io.NopCloser(body)}
	if _, err := readBoundedResponseBody(resp); err == nil {
		t.Fatal("expected oversized response error")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
