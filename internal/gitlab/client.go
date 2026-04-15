package gitlab

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

// Client is a minimal GitLab REST API client.
type Client struct {
	BaseURL string // e.g. http://10.0.0.60:8929
	Token   string // Personal Access Token
	http    *http.Client
}

// New creates a Client. baseURL trailing slash is stripped.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) do(method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

// GetCurrentUser returns the username for the authenticated token.
func (c *Client) GetCurrentUser() (string, error) {
	resp, err := c.do("GET", "/api/v4/user", nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitLab /user: HTTP %d", resp.StatusCode)
	}
	var u struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", err
	}
	return u.Username, nil
}

// RepoInfo holds the data returned for an existing project.
type RepoInfo struct {
	ID            int    `json:"id"`
	HTTPURLToRepo string `json:"http_url_to_repo"`
}

// CheckRepo returns (info, true, nil) if the project namespace/name exists,
// (nil, false, nil) if it does not, or (nil, false, err) on error.
func (c *Client) CheckRepo(namespace, name string) (*RepoInfo, bool, error) {
	encoded := url.PathEscape(namespace + "/" + name)
	resp, err := c.do("GET", "/api/v4/projects/"+encoded, nil)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, false, nil
	}
	if resp.StatusCode != 200 {
		return nil, false, fmt.Errorf("GitLab check repo: HTTP %d", resp.StatusCode)
	}
	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, false, err
	}
	return &info, true, nil
}

// CreateRepo creates a new project under the authenticated user's namespace.
// Returns the HTTP clone URL of the new project.
func (c *Client) CreateRepo(name, description string, private bool) (string, error) {
	visibility := "public"
	if private {
		visibility = "private"
	}
	payload := map[string]any{
		"name":        name,
		"description": description,
		"visibility":  visibility,
	}
	resp, err := c.do("POST", "/api/v4/projects", payload)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitLab create repo: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var info RepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.HTTPURLToRepo, nil
}

// RepoHTTPURL returns the HTTP clone URL for an existing project.
// It combines the base URL with credentials for use as a git remote.
func (c *Client) RepoHTTPURL(namespace, name string) string {
	// Embed token in URL so git doesn't prompt for credentials.
	base := strings.TrimPrefix(strings.TrimPrefix(c.BaseURL, "https://"), "http://")
	scheme := "http"
	if strings.HasPrefix(c.BaseURL, "https://") {
		scheme = "https"
	}
	return fmt.Sprintf("%s://oauth2:%s@%s/%s/%s.git", scheme, c.Token, base, namespace, name)
}
