package provider

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type fixedProviderClient struct {
	response *http.Response
}

func (c *fixedProviderClient) Forward(context.Context, domain.Provider, domain.Model, domain.APIKey, *http.Request, string) (*http.Response, error) {
	return c.response, nil
}

func (c *fixedProviderClient) TestKey(context.Context, domain.Provider, domain.Model, domain.APIKey) error {
	return nil
}

func TestBoundedProviderClientBuffersNormalResponse(t *testing.T) {
	client := withResponseLimit(&fixedProviderClient{response: &http.Response{
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader([]byte(`{"ok":true}`))),
		ContentLength: -1,
	}})
	resp, err := client.Forward(context.Background(), domain.Provider{}, domain.Model{}, domain.APIKey{}, &http.Request{}, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"ok":true}` {
		t.Fatalf("got %q", data)
	}
}

func TestBoundedProviderClientRejectsOversizedResponse(t *testing.T) {
	client := withResponseLimit(&fixedProviderClient{response: &http.Response{
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(io.LimitReader(zeroReader{}, maxBufferedProviderResponseBytes+1)),
		ContentLength: -1,
	}})
	if _, err := client.Forward(context.Background(), domain.Provider{}, domain.Model{}, domain.APIKey{}, &http.Request{}, ""); err == nil {
		t.Fatal("expected oversized response error")
	}
}

func TestBoundedProviderClientPreservesSSEStreaming(t *testing.T) {
	body := io.NopCloser(bytes.NewReader([]byte("data: ok\n\n")))
	client := withResponseLimit(&fixedProviderClient{response: &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   body,
	}})
	resp, err := client.Forward(context.Background(), domain.Provider{}, domain.Model{}, domain.APIKey{}, &http.Request{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Body != body {
		t.Fatal("streaming body was replaced")
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
