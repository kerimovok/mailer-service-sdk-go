package mailersdk

import (
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

// Client is the mailer service HTTP client
type Client struct {
	baseURL string
	client  *http.Client
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
			Message:   errMessage,
			Body:      bodyStr,
		}
	}
	return &APIError{
		StatusCode: statusCode,
		Message:   bodyStr,
		Body:      bodyStr,
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

// do performs a GET request, checks status, and decodes JSON into result
func (c *Client) do(ctx context.Context, method, path string, successStatuses []int, result interface{}, wrapErr string) error {
	req, err := http.NewRequestWithContext(ctx, method, path, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", wrapErr, err)
	}
	resp, err := c.client.Do(req)
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

func pathSeg(s string) string { return url.PathEscape(s) }

// NewClient creates a new mailer service client
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
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
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
	ID           string                 `json:"id"`
	Service      string                 `json:"service"`
	Type         string                 `json:"type"`
	To           string                 `json:"to"`
	Subject      string                 `json:"subject"`
	Template     string                 `json:"template"`
	Data         map[string]interface{} `json:"data,omitempty"`
	Status       string                 `json:"status"`
	Error        string                 `json:"error,omitempty"`
	Attachments  []AttachmentListItem   `json:"attachments,omitempty"`
	CreatedAt    string                 `json:"createdAt"`
	UpdatedAt    string                 `json:"updatedAt"`
}

// AttachmentListItem represents an attachment in list/detail
type AttachmentListItem struct {
	ID        string `json:"id"`
	MailID    string `json:"mailId"`
	File      string `json:"file"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ListMailsRequest represents query parameters for listing mails
type ListMailsRequest struct {
	Page    int    // Page number (default: 1)
	PerPage int    // Items per page (default: 20)
	Service string // Filter by service
	Type    string // Filter by type
	Status  string // Filter by status
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

// ListMails lists mails with optional filters and pagination
func (c *Client) ListMails(ctx context.Context, req ListMailsRequest) (*ListMailsResponse, error) {
	queryParams := make([]string, 0)
	if req.Page > 0 {
		queryParams = append(queryParams, fmt.Sprintf("page=%d", req.Page))
	}
	if req.PerPage > 0 {
		queryParams = append(queryParams, fmt.Sprintf("per_page=%d", req.PerPage))
	}
	if req.Service != "" {
		queryParams = append(queryParams, "service="+url.QueryEscape(req.Service))
	}
	if req.Type != "" {
		queryParams = append(queryParams, "type="+url.QueryEscape(req.Type))
	}
	if req.Status != "" {
		queryParams = append(queryParams, "status="+url.QueryEscape(req.Status))
	}

	path := c.baseURL + apiPathPrefix + "/mails"
	if len(queryParams) > 0 {
		path += "?" + strings.Join(queryParams, "&")
	}

	var result ListMailsResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to list mails")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListMailsWithQuery lists mails by forwarding the raw query string to mailer-service.
func (c *Client) ListMailsWithQuery(ctx context.Context, queryString string) (*ListMailsResponse, error) {
	path := c.baseURL + apiPathPrefix + "/mails"
	if queryString != "" {
		path += "?" + queryString
	}
	var result ListMailsResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to list mails")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMailResponse represents the response from getting a mail
type GetMailResponse struct {
	Success   bool           `json:"success"`
	Message   string         `json:"message"`
	Status    int            `json:"status"`
	Timestamp string         `json:"timestamp,omitempty"`
	Data      MailListItem   `json:"data"`
}

// GetMail gets a mail by ID
func (c *Client) GetMail(ctx context.Context, id string) (*GetMailResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("mail id is required")
	}
	path := c.baseURL + apiPathPrefix + "/mails/" + pathSeg(id)
	var result GetMailResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to get mail")
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

// ListTemplatesRequest represents query parameters for listing templates
type ListTemplatesRequest struct {
	Page    int  // Page number
	PerPage int  // Items per page
}

// ListTemplatesResponse represents the paginated response from listing templates
type ListTemplatesResponse struct {
	Success    bool                `json:"success"`
	Message    string              `json:"message"`
	Status     int                 `json:"status"`
	Timestamp  string              `json:"timestamp,omitempty"`
	Data       []TemplateListItem  `json:"data"`
	Pagination *Pagination         `json:"pagination,omitempty"`
}

// ListTemplates lists templates with pagination
func (c *Client) ListTemplates(ctx context.Context, req ListTemplatesRequest) (*ListTemplatesResponse, error) {
	queryParams := make([]string, 0)
	if req.Page > 0 {
		queryParams = append(queryParams, fmt.Sprintf("page=%d", req.Page))
	}
	if req.PerPage > 0 {
		queryParams = append(queryParams, fmt.Sprintf("per_page=%d", req.PerPage))
	}

	path := c.baseURL + apiPathPrefix + "/templates"
	if len(queryParams) > 0 {
		path += "?" + strings.Join(queryParams, "&")
	}

	var result ListTemplatesResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to list templates")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListTemplatesWithQuery lists templates by forwarding the raw query string to mailer-service.
func (c *Client) ListTemplatesWithQuery(ctx context.Context, queryString string) (*ListTemplatesResponse, error) {
	path := c.baseURL + apiPathPrefix + "/templates"
	if queryString != "" {
		path += "?" + queryString
	}
	var result ListTemplatesResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to list templates")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTemplateResponse represents the response from getting a template
type GetTemplateResponse struct {
	Success   bool              `json:"success"`
	Message   string            `json:"message"`
	Status    int               `json:"status"`
	Timestamp string            `json:"timestamp,omitempty"`
	Data      TemplateListItem  `json:"data"`
}

// GetTemplate gets a template by ID
func (c *Client) GetTemplate(ctx context.Context, id string) (*GetTemplateResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("template id is required")
	}
	path := c.baseURL + apiPathPrefix + "/templates/" + pathSeg(id)
	var result GetTemplateResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to get template")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// AttachmentListItem is reused for single attachment response

// ListAttachmentsRequest represents query parameters for listing attachments
type ListAttachmentsRequest struct {
	Page    int    // Page number
	PerPage int    // Items per page
	MailID  string // Filter by mail ID
}

// ListAttachmentsResponse represents the paginated response from listing attachments
type ListAttachmentsResponse struct {
	Success    bool                  `json:"success"`
	Message    string                `json:"message"`
	Status     int                   `json:"status"`
	Timestamp  string                `json:"timestamp,omitempty"`
	Data       []AttachmentListItem  `json:"data"`
	Pagination *Pagination           `json:"pagination,omitempty"`
}

// ListAttachments lists attachments with optional filters and pagination
func (c *Client) ListAttachments(ctx context.Context, req ListAttachmentsRequest) (*ListAttachmentsResponse, error) {
	queryParams := make([]string, 0)
	if req.Page > 0 {
		queryParams = append(queryParams, fmt.Sprintf("page=%d", req.Page))
	}
	if req.PerPage > 0 {
		queryParams = append(queryParams, fmt.Sprintf("per_page=%d", req.PerPage))
	}
	if req.MailID != "" {
		queryParams = append(queryParams, "mail_id="+url.QueryEscape(req.MailID))
	}

	path := c.baseURL + apiPathPrefix + "/attachments"
	if len(queryParams) > 0 {
		path += "?" + strings.Join(queryParams, "&")
	}

	var result ListAttachmentsResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to list attachments")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListAttachmentsWithQuery lists attachments by forwarding the raw query string to mailer-service.
func (c *Client) ListAttachmentsWithQuery(ctx context.Context, queryString string) (*ListAttachmentsResponse, error) {
	path := c.baseURL + apiPathPrefix + "/attachments"
	if queryString != "" {
		path += "?" + queryString
	}
	var result ListAttachmentsResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to list attachments")
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAttachmentResponse represents the response from getting an attachment
type GetAttachmentResponse struct {
	Success   bool                `json:"success"`
	Message   string              `json:"message"`
	Status    int                 `json:"status"`
	Timestamp string              `json:"timestamp,omitempty"`
	Data      AttachmentListItem  `json:"data"`
}

// GetAttachment gets an attachment by ID
func (c *Client) GetAttachment(ctx context.Context, id string) (*GetAttachmentResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("attachment id is required")
	}
	path := c.baseURL + apiPathPrefix + "/attachments/" + pathSeg(id)
	var result GetAttachmentResponse
	err := c.do(ctx, http.MethodGet, path, []int{http.StatusOK}, &result, "failed to get attachment")
	if err != nil {
		return nil, err
	}
	return &result, nil
}
