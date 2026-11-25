package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	queryPath     = "/rest/table/v1/query"
	nonQueryPath  = "/rest/table/v1/nonQuery"
	insertPath    = "/rest/table/v1/insertTablet"
	maxBodyBytes  = 32 << 20
	maxQueryLimit = 1000000
)

// Config describes an Apache IoTDB 2.x Table REST endpoint.
type Config struct {
	Address  string
	Username string
	Password string
	Timeout  time.Duration
}

// Client calls the IoTDB Table REST v1 API.
type Client struct {
	baseURL string
	user    string
	pass    string
	http    *http.Client
}

type Request struct {
	Database string `json:"database"`
	SQL      string `json:"sql,omitempty"`
	RowLimit int    `json:"row_limit,omitempty"`
}

type Tablet struct {
	Database         string          `json:"database"`
	Table            string          `json:"table"`
	ColumnNames      []string        `json:"column_names"`
	ColumnCategories []string        `json:"column_categories"`
	DataTypes        []string        `json:"data_types"`
	Timestamps       []int64         `json:"timestamps"`
	Values           [][]interface{} `json:"values"`
}

type Response struct {
	Code        json.RawMessage `json:"code"`
	Message     string          `json:"message"`
	ColumnNames []string        `json:"column_names"`
	DataTypes   []string        `json:"data_types"`
	Values      [][]interface{} `json:"values"`
}

func New(config Config) (*Client, error) {
	address := strings.TrimSpace(config.Address)
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("IoTDB address must be an absolute http or https URL")
	}
	if config.Username == "" || config.Password == "" {
		return nil, errors.New("IoTDB REST username and password are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(address, "/"),
		user:    config.Username,
		pass:    config.Password,
		http:    &http.Client{Timeout: config.Timeout},
	}, nil
}

func (c *Client) Query(ctx context.Context, database, sql string, rowLimit int) (*Response, error) {
	if rowLimit < 1 || rowLimit > maxQueryLimit {
		return nil, fmt.Errorf("IoTDB row limit must be between 1 and %d", maxQueryLimit)
	}
	var result Response
	err := c.post(ctx, queryPath, Request{Database: database, SQL: sql, RowLimit: rowLimit}, &result)
	return &result, err
}

func (c *Client) NonQuery(ctx context.Context, database, sql string) error {
	return c.post(ctx, nonQueryPath, Request{Database: database, SQL: sql}, nil)
}

func (c *Client) InsertTablet(ctx context.Context, tablet Tablet) error {
	if strings.TrimSpace(tablet.Database) == "" || !ValidIdentifier(tablet.Table) {
		return errors.New("IoTDB tablet requires a database and a plain table identifier")
	}
	if len(tablet.ColumnNames) == 0 || len(tablet.ColumnNames) != len(tablet.ColumnCategories) ||
		len(tablet.ColumnNames) != len(tablet.DataTypes) {
		return errors.New("IoTDB tablet columns, categories, and data types must have equal non-zero lengths")
	}
	if len(tablet.Timestamps) != len(tablet.Values) {
		return errors.New("IoTDB tablet timestamps and values must have equal lengths")
	}
	for i, row := range tablet.Values {
		if len(row) != len(tablet.ColumnNames) {
			return fmt.Errorf("IoTDB tablet row %d has %d values for %d columns", i, len(row), len(tablet.ColumnNames))
		}
	}
	return c.post(ctx, insertPath, tablet, nil)
}

func (c *Client) post(ctx context.Context, path string, payload interface{}, out *Response) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode IoTDB request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create IoTDB request: %w", err)
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("IoTDB request failed (write outcome may be unknown): %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read IoTDB response (write outcome may be unknown): %w", err)
	}
	if len(data) > maxBodyBytes {
		return errors.New("IoTDB response exceeded the 32 MiB limit")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("IoTDB HTTP %d: %s", resp.StatusCode, responseMessage(data))
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	var result Response
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("IoTDB returned invalid JSON: %w", err)
	}
	if err := checkCode(result.Code, result.Message); err != nil {
		return err
	}
	if out != nil {
		*out = result
	}
	return nil
}

func responseMessage(data []byte) string {
	var result struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &result) == nil && result.Message != "" {
		return result.Message
	}
	message := strings.TrimSpace(string(data))
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}

func checkCode(raw json.RawMessage, message string) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var code int
	if err := json.Unmarshal(raw, &code); err != nil {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return errors.New("IoTDB returned an invalid status code")
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return errors.New("IoTDB returned an invalid status code")
		}
		code = parsed
	}
	if code != http.StatusOK {
		if message == "" {
			message = "request rejected"
		}
		return fmt.Errorf("IoTDB error %d: %s", code, message)
	}
	return nil
}

func ValidIdentifier(name string) bool {
	if name == "" || !isLetter(name[0]) && name[0] != '_' {
		return false
	}
	for i := 1; i < len(name); i++ {
		char := name[i]
		if !isLetter(char) && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func isLetter(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}
