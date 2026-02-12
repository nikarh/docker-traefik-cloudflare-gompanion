package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type CloudflareAPI struct {
	httpClient *http.Client
	email      string
	token      string
	logger     *Logger
}

type DNSRecord struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type DNSRecordRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
}

type cfResponse[T any] struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result T `json:"result"`
}

func NewCloudflareAPI(email string, token string, logger *Logger) (*CloudflareAPI, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("missing token")
	}
	return &CloudflareAPI{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		email:      strings.TrimSpace(email),
		token:      strings.TrimSpace(token),
		logger:     logger,
	}, nil
}

func (cf *CloudflareAPI) ListDNSRecords(zoneID string, name string) ([]DNSRecord, error) {
	path := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?name=%s", zoneID, url.QueryEscape(name))
	body, err := cf.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var parsed cfResponse[[]DNSRecord]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if !parsed.Success {
		return nil, fmt.Errorf("cloudflare list failed: %s", cf.formatErrors(parsed.Errors))
	}
	return parsed.Result, nil
}

func (cf *CloudflareAPI) CreateDNSRecord(zoneID string, record DNSRecordRequest) error {
	path := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	body, err := cf.doRequest(http.MethodPost, path, payload)
	if err != nil {
		return err
	}
	var parsed cfResponse[DNSRecord]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if !parsed.Success {
		return fmt.Errorf("cloudflare create failed: %s", cf.formatErrors(parsed.Errors))
	}
	return nil
}

func (cf *CloudflareAPI) UpdateDNSRecord(zoneID string, recordID string, record DNSRecordRequest) error {
	path := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	body, err := cf.doRequest(http.MethodPut, path, payload)
	if err != nil {
		return err
	}
	var parsed cfResponse[DNSRecord]
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if !parsed.Success {
		return fmt.Errorf("cloudflare update failed: %s", cf.formatErrors(parsed.Errors))
	}
	return nil
}

func (cf *CloudflareAPI) doRequest(method string, endpoint string, body []byte) ([]byte, error) {
	cf.logger.Verbosef("Querying Cloudflare API: %s %s", method, endpoint)
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cf.email != "" {
		req.Header.Set("X-Auth-Email", cf.email)
		req.Header.Set("X-Auth-Key", cf.token)
	} else {
		req.Header.Set("Authorization", "Bearer "+cf.token)
	}

	resp, err := cf.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http status %d: %s", resp.StatusCode, string(respBytes))
	}
	cf.logger.Verbosef("Cloudflare API response: %s %s -> %d", method, endpoint, resp.StatusCode)
	return respBytes, nil
}

func (cf *CloudflareAPI) formatErrors(errors []struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}) string {
	parts := make([]string, 0, len(errors))
	for _, e := range errors {
		parts = append(parts, fmt.Sprintf("%d %s", e.Code, e.Message))
	}
	return strings.Join(parts, "; ")
}
