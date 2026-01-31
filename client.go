package mailersdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiPathPrefix  = "/api/v1"
	defaultTimeout = 10 * time.Second
)

// Config holds configuration for the mailer service client
type Config struct {
	BaseURL string        // Mailer service base URL (e.g., "http://localhost:3002")
	Timeout time.Duration // Request timeout (default: 10 seconds)
}

// Client is the mailer service HTTP client (plain HTTP).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// APIError represents an error returned by the mailer service API
type APIError struct {
	StatusCode int    // HTTP status code
	Message    string // Error message from the API response
	Body       string // Raw response body (for debugging)
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("mailer service returned status %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("mailer service returned status %d: %s", e.StatusCode, e.Body)
}

// IsAPIError checks if an error is an APIError and returns it
func IsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if err != nil && errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

func parseErrorResponse(statusCode int, body []byte) *APIError {
	var errorResp struct {
		Message string `json:"message"`
		Success bool   `json:"success"`
		Status  int    `json:"status"`
		Error   string `json:"error"`
	}

	bodyStr := string(body)
	if err := json.Unmarshal(body, &errorResp); err == nil && (errorResp.Message != "" || errorResp.Error != "") {
		errMessage := errorResp.Error
		if errMessage == "" {
			errMessage = errorResp.Message
		}
		return &APIError{
			StatusCode: statusCode,
			Message:    errMessage,
			Body:       bodyStr,
		}
	}
	return &APIError{
		StatusCode: statusCode,
		Message:    bodyStr,
		Body:       bodyStr,
	}
}

func statusIn(code int, codes []int) bool {
	for _, c := range codes {
		if code == c {
			return true
		}
	}
	return false
}

// do performs a request, checks status, and optionally decodes JSON into result.
// successStatuses lists HTTP status codes treated as success (e.g. 200).
// path is the path including optional query (e.g. "/api/v1/mails" or "/api/v1/mails?page=1").
func (c *Client) do(method, path string, body interface{}, successStatuses []int, result interface{}, wrapErr string) error {
	resp, err := c.doRequest(method, path, body)
	if err != nil {
		return fmt.Errorf("%s: %w", wrapErr, err)
	}
	defer resp.Body.Close()

	if !statusIn(resp.StatusCode, successStatuses) {
		respBody, _ := io.ReadAll(resp.Body)
		return parseErrorResponse(resp.StatusCode, respBody)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("%s: %w", wrapErr, err)
		}
	}
	return nil
}

// doRequest performs an HTTP request.
func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	fullURL := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func pathSeg(s string) string { return url.PathEscape(s) }

// NewClient creates a new mailer service client (plain HTTP).
func NewClient(config Config) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}

	baseURL := strings.TrimRight(config.BaseURL, "/")
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// Pagination contains pagination metadata (matches mailer-service / go-pkg-utils)
type Pagination struct {
	Page         int   `json:"page"`
	PerPage      int   `json:"perPage"`
	Total        int64 `json:"total"`
	TotalPages   int   `json:"totalPages"`
	HasNext      bool  `json:"hasNext"`
	HasPrevious  bool  `json:"hasPrevious"`
	NextPage     *int  `json:"nextPage,omitempty"`
	PreviousPage *int  `json:"previousPage,omitempty"`
}

// MailListItem represents a mail record in list responses
type MailListItem struct {
	ID          string                 `json:"id"`
	Service     string                 `json:"service"`
	Type        string                 `json:"type"`
	To          string                 `json:"to"`
	Subject     string                 `json:"subject"`
	Template    string                 `json:"template"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Status      string                 `json:"status"`
	Error       string                 `json:"error,omitempty"`
	Attachments []AttachmentListItem   `json:"attachments,omitempty"`
	CreatedAt   string                 `json:"createdAt"`
	UpdatedAt   string                 `json:"updatedAt"`
}

// AttachmentListItem represents an attachment in list/detail
type AttachmentListItem struct {
	ID        string `json:"id"`
	MailID    string `json:"mailId"`
	File      string `json:"file"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ListMailsResponse represents the paginated response from listing mails
type ListMailsResponse struct {
	Success    bool           `json:"success"`
	Message    string         `json:"message"`
	Status     int            `json:"status"`
	Timestamp  string         `json:"timestamp,omitempty"`
	Data       []MailListItem `json:"data"`
	Pagination *Pagination    `json:"pagination,omitempty"`
}

// ListMails lists mails by forwarding the raw query string to mailer-service.
func (c *Client) ListMails(ctx context.Context, queryString string) (*ListMailsResponse, error) {
	path := apiPathPrefix + "/mails"
	if queryString != "" {
		path += "?" + queryString
	}
	var result ListMailsResponse
	err := c.do(http.MethodGet, path, nil, []int{http.StatusOK}, &result, "failed to list mails")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMailResponse represents the response from getting a mail
type GetMailResponse struct {
	Success   bool         `json:"success"`
	Message   string       `json:"message"`
	Status    int          `json:"status"`
	Timestamp string       `json:"timestamp,omitempty"`
	Data      MailListItem `json:"data"`
}

// GetMail gets a mail by ID
func (c *Client) GetMail(ctx context.Context, id string) (*GetMailResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("mail id is required")
	}
	path := apiPathPrefix + "/mails/" + pathSeg(id)
	var result GetMailResponse
	err := c.do(http.MethodGet, path, nil, []int{http.StatusOK}, &result, "failed to get mail")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// TemplateListItem represents a template in list responses
type TemplateListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	Description string `json:"description"`
	IsActive    bool   `json:"isActive"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// ListTemplatesResponse represents the paginated response from listing templates
type ListTemplatesResponse struct {
	Success    bool               `json:"success"`
	Message    string             `json:"message"`
	Status     int                `json:"status"`
	Timestamp  string             `json:"timestamp,omitempty"`
	Data       []TemplateListItem `json:"data"`
	Pagination *Pagination        `json:"pagination,omitempty"`
}

// ListTemplates lists templates by forwarding the raw query string to mailer-service.
func (c *Client) ListTemplates(ctx context.Context, queryString string) (*ListTemplatesResponse, error) {
	path := apiPathPrefix + "/templates"
	if queryString != "" {
		path += "?" + queryString
	}
	var result ListTemplatesResponse
	err := c.do(http.MethodGet, path, nil, []int{http.StatusOK}, &result, "failed to list templates")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTemplateResponse represents the response from getting a template
type GetTemplateResponse struct {
	Success   bool             `json:"success"`
	Message   string           `json:"message"`
	Status    int              `json:"status"`
	Timestamp string           `json:"timestamp,omitempty"`
	Data      TemplateListItem `json:"data"`
}

// GetTemplate gets a template by ID
func (c *Client) GetTemplate(ctx context.Context, id string) (*GetTemplateResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("template id is required")
	}
	path := apiPathPrefix + "/templates/" + pathSeg(id)
	var result GetTemplateResponse
	err := c.do(http.MethodGet, path, nil, []int{http.StatusOK}, &result, "failed to get template")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// AttachmentListItem is reused for single attachment response

// ListAttachmentsResponse represents the paginated response from listing attachments
type ListAttachmentsResponse struct {
	Success    bool                 `json:"success"`
	Message    string               `json:"message"`
	Status     int                  `json:"status"`
	Timestamp  string               `json:"timestamp,omitempty"`
	Data       []AttachmentListItem `json:"data"`
	Pagination *Pagination          `json:"pagination,omitempty"`
}

// ListAttachments lists attachments by forwarding the raw query string to mailer-service.
func (c *Client) ListAttachments(ctx context.Context, queryString string) (*ListAttachmentsResponse, error) {
	path := apiPathPrefix + "/attachments"
	if queryString != "" {
		path += "?" + queryString
	}
	var result ListAttachmentsResponse
	err := c.do(http.MethodGet, path, nil, []int{http.StatusOK}, &result, "failed to list attachments")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAttachmentResponse represents the response from getting an attachment
type GetAttachmentResponse struct {
	Success   bool               `json:"success"`
	Message   string             `json:"message"`
	Status    int                `json:"status"`
	Timestamp string             `json:"timestamp,omitempty"`
	Data      AttachmentListItem `json:"data"`
}

// GetAttachment gets an attachment by ID
func (c *Client) GetAttachment(ctx context.Context, id string) (*GetAttachmentResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("attachment id is required")
	}
	path := apiPathPrefix + "/attachments/" + pathSeg(id)
	var result GetAttachmentResponse
	err := c.do(http.MethodGet, path, nil, []int{http.StatusOK}, &result, "failed to get attachment")
	if err != nil {
		return nil, err
	}
	return &result, nil
}
