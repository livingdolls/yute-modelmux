package service

import (
	"fmt"
	"io"
	"net/http"
)

const maxBufferedUpstreamResponseBytes int64 = 32 << 20

func readBoundedResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	reader := &io.LimitedReader{R: resp.Body, N: maxBufferedUpstreamResponseBytes + 1}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}
	if int64(len(data)) > maxBufferedUpstreamResponseBytes {
		return nil, fmt.Errorf("upstream response exceeds %d-byte buffer limit", maxBufferedUpstreamResponseBytes)
	}
	return data, nil
}
