package mtx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	Base   string
	Client *http.Client
}

func New(base string) *Client {
	return &Client{
		Base:   base,
		Client: &http.Client{Timeout: 10 * time.Second},
	}
}

func PathBody(host, port, format string) (map[string]any, error) {
	switch format {
	case "mpegts", "ts":
		return map[string]any{
			"source":                     fmt.Sprintf("udp+mpegts://%s", net.JoinHostPort(host, port)),
			"sourceOnDemand":             true,
			"sourceOnDemandStartTimeout": "10s",
		}, nil
	case "rtp", "":
		sdp := fmt.Sprintf("v=0\r\no=- 0 0 IN IP4 %s\r\ns=dynamic\r\nc=IN IP4 %s\r\nt=0 0\r\nm=video %s RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\na=fmtp:96 packetization-mode=1\r\n", host, host, port)
		return map[string]any{
			"source":                     fmt.Sprintf("udp+rtp://%s", net.JoinHostPort(host, port)),
			"rtpSDP":                     sdp,
			"sourceOnDemand":             true,
			"sourceOnDemandStartTimeout": "10s",
		}, nil
	default:
		return nil, fmt.Errorf("format must be rtp or mpegts")
	}
}

func (c *Client) Upsert(name string, body map[string]any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	esc := url.PathEscape(name)
	res, err := c.post("/v3/config/paths/add/"+esc, b)
	if err != nil {
		return fmt.Errorf("MediaMTX API: %w (is mediamtx running?)", err)
	}
	defer res.Body.Close()
	msg, _ := io.ReadAll(res.Body)
	if res.StatusCode < 300 {
		return nil
	}
	res2, err := c.post("/v3/config/paths/replace/"+esc, b)
	if err != nil {
		return fmt.Errorf("add failed (%s): %s; replace: %v", res.Status, bytes.TrimSpace(msg), err)
	}
	defer res2.Body.Close()
	if res2.StatusCode >= 300 {
		msg2, _ := io.ReadAll(res2.Body)
		return fmt.Errorf("mtx replace %s: %s", res2.Status, bytes.TrimSpace(msg2))
	}
	return nil
}

func (c *Client) Delete(name string) error {
	req, err := http.NewRequest(http.MethodPost, c.Base+"/v3/config/paths/delete/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode != http.StatusNotFound {
		msg, _ := io.ReadAll(res.Body)
		return fmt.Errorf("mtx delete %s: %s", res.Status, bytes.TrimSpace(msg))
	}
	return nil
}

type PathInfo struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Source any    `json:"source"`
}

func (c *Client) ListPaths() ([]PathInfo, error) {
	res, err := c.Client.Get(c.Base + "/v3/paths/list")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out struct {
		Items []PathInfo `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *Client) GetPath(name string) (*PathInfo, error) {
	res, err := c.Client.Get(c.Base + "/v3/paths/get/" + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if res.StatusCode >= 300 {
		msg, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("mtx get %s: %s", res.Status, bytes.TrimSpace(msg))
	}
	var p PathInfo
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *Client) post(path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, c.Base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.Client.Do(req)
}
