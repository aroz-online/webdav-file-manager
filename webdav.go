package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// WebDAVClient provides methods to interact with a WebDAV server
type WebDAVClient struct {
	BaseURL  string
	Username string
	Password string
	client   *http.Client
}

// WebDAVFileInfo represents a file or directory from a WebDAV PROPFIND response
type WebDAVFileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	IsDir    bool   `json:"is_dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// multistatusXML represents the XML response from a PROPFIND request
type multistatusXML struct {
	XMLName   xml.Name      `xml:"multistatus"`
	Responses []responseXML `xml:"response"`
}

type responseXML struct {
	Href     string        `xml:"href"`
	Propstat []propstatXML `xml:"propstat"`
}

type propstatXML struct {
	Prop   propXML `xml:"prop"`
	Status string  `xml:"status"`
}

type propXML struct {
	DisplayName      string        `xml:"displayname"`
	ContentLength    int64         `xml:"getcontentlength"`
	LastModified     string        `xml:"getlastmodified"`
	ResourceType     resourcetType `xml:"resourcetype"`
	ContentType      string        `xml:"getcontenttype"`
	CreationDate     string        `xml:"creationdate"`
	IsCollection     bool
}

type resourcetType struct {
	Collection *struct{} `xml:"collection"`
}

// NewWebDAVClient creates a new WebDAV client
func NewWebDAVClient(host string, port int, username, password string) *WebDAVClient {
	return &WebDAVClient{
		BaseURL:  fmt.Sprintf("http://127.0.0.1:%d", port),
		Username: username,
		Password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// newRequest creates a new HTTP request with basic auth
func (w *WebDAVClient) newRequest(method, urlPath string, body io.Reader) (*http.Request, error) {
	fullURL := w.BaseURL + urlPath
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(w.Username, w.Password)
	return req, nil
}

// List lists files and directories at the given path
func (w *WebDAVClient) List(dirPath string) ([]WebDAVFileInfo, error) {
	if dirPath == "" {
		dirPath = "/"
	}
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	req, err := w.newRequest("PROPFIND", dirPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebDAV server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: invalid username or password")
	}

	if resp.StatusCode != 207 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var ms multistatusXML
	if err := xml.Unmarshal(bodyBytes, &ms); err != nil {
		return nil, fmt.Errorf("failed to parse WebDAV response: %w", err)
	}

	var files []WebDAVFileInfo
	for _, r := range ms.Responses {
		href := r.Href
		// Skip the directory itself (first entry is always the requested directory)
		cleanHref := strings.TrimSuffix(href, "/")
		cleanDir := strings.TrimSuffix(dirPath, "/")
		if cleanHref == cleanDir || cleanHref == "" {
			continue
		}

		fi := WebDAVFileInfo{
			Path: href,
			Name: path.Base(href),
		}

		for _, ps := range r.Propstat {
			if !strings.Contains(ps.Status, "200") {
				continue
			}
			fi.Size = ps.Prop.ContentLength
			fi.Modified = ps.Prop.LastModified
			if ps.Prop.ResourceType.Collection != nil {
				fi.IsDir = true
			}
			if ps.Prop.DisplayName != "" {
				fi.Name = ps.Prop.DisplayName
			}
		}

		files = append(files, fi)
	}

	return files, nil
}

// Download retrieves a file from the WebDAV server
func (w *WebDAVClient) Download(filePath string) (io.ReadCloser, string, int64, error) {
	req, err := w.newRequest("GET", filePath, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to download file: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("authentication failed")
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("failed to download file: status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return resp.Body, contentType, resp.ContentLength, nil
}

// Upload uploads a file to the WebDAV server
func (w *WebDAVClient) Upload(filePath string, body io.Reader) error {
	req, err := w.newRequest("PUT", filePath, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed")
	}

	// 201 Created or 204 No Content are both success
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload file: status %d", resp.StatusCode)
	}

	return nil
}

// Delete deletes a file or directory from the WebDAV server
func (w *WebDAVClient) Delete(filePath string) error {
	req, err := w.newRequest("DELETE", filePath, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed")
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete: status %d", resp.StatusCode)
	}

	return nil
}

// Rename renames/moves a file on the WebDAV server using MOVE
func (w *WebDAVClient) Rename(oldPath, newPath string) error {
	req, err := w.newRequest("MOVE", oldPath, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Destination", w.BaseURL+newPath)
	req.Header.Set("Overwrite", "F")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to rename: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed")
	}

	if resp.StatusCode == http.StatusPreconditionFailed {
		return fmt.Errorf("destination already exists")
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to rename: status %d", resp.StatusCode)
	}

	return nil
}

// Move moves a file to a new directory on the WebDAV server using MOVE
func (w *WebDAVClient) Move(srcPath, destDir string) error {
	fileName := path.Base(srcPath)
	destPath := path.Join(destDir, fileName)
	if !strings.HasPrefix(destPath, "/") {
		destPath = "/" + destPath
	}
	return w.Rename(srcPath, destPath)
}

// MkDir creates a directory via MKCOL
func (w *WebDAVClient) MkDir(dirPath string) error {
	if !strings.HasSuffix(dirPath, "/") {
		dirPath += "/"
	}

	req, err := w.newRequest("MKCOL", dirPath, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed")
	}

	if resp.StatusCode == http.StatusMethodNotAllowed {
		return fmt.Errorf("directory already exists")
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create directory: status %d", resp.StatusCode)
	}

	return nil
}

// GetFileContent retrieves file content as bytes (for previewing text files, etc.)
func (w *WebDAVClient) GetFileContent(filePath string) ([]byte, string, error) {
	body, contentType, _, err := w.Download(filePath)
	if err != nil {
		return nil, "", err
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file content: %w", err)
	}

	return data, contentType, nil
}
