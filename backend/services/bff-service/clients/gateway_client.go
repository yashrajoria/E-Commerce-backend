package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type GatewayClient struct {
	baseURL string
	client  *http.Client
}

func NewGatewayClient(baseURL string, timeout time.Duration) *GatewayClient {
	return &GatewayClient{
		baseURL: baseURL,
		client: &http.Client{Timeout: timeout},
	}
}

func (g *GatewayClient) Do(ctx context.Context, method, path string, query url.Values, headers http.Header, body io.Reader) (*http.Response, error) {
	u := g.baseURL + path
	if query != nil && len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}

	return g.client.Do(req)
}

func ReadJSONBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func CopyResponse(w http.ResponseWriter, resp *http.Response) error {
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.WriteHeader(resp.StatusCode)

	_, err := io.Copy(w, resp.Body)
	return err
}

func DecodeJSON(resp *http.Response, out interface{}) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upstream error: status=%d body=%s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func BodyFromBytes(b []byte) io.Reader {
	if len(b) == 0 {
		return nil
	}
	return bytes.NewReader(b)
}
