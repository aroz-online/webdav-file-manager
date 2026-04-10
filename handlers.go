package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
)

var (
	currentConfig WebDAVClientConfig
	davClient     *WebDAVClient
	clientMu      sync.RWMutex
)

// initWebDAVClient initializes or re-initializes the WebDAV client from config
func initWebDAVClient() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	clientMu.Lock()
	defer clientMu.Unlock()
	currentConfig = cfg
	davClient = NewWebDAVClient("127.0.0.1", cfg.Port, cfg.Username, cfg.Password)
	return nil
}

// getClient returns the current WebDAV client (thread-safe)
func getClient() *WebDAVClient {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return davClient
}

// stopAllWebDAVServerConnections gracefully stops WebDAV connections
func stopAllWebDAVServerConnections() {
	clientMu.Lock()
	defer clientMu.Unlock()
	davClient = nil
}

// jsonResponse writes a JSON response
func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// jsonError writes a JSON error response
func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// handleGetConfigs returns the current WebDAV configurations
func handleGetConfigs(w http.ResponseWriter, r *http.Request) {
	clientMu.RLock()
	cfg := currentConfig
	clientMu.RUnlock()
	jsonResponse(w, cfg)
}

// handleSetConfigs updates the WebDAV configuration
func handleSetConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var newCfg WebDAVClientConfig
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		jsonError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate port range
	if newCfg.Port < 1 || newCfg.Port > 65535 {
		jsonError(w, "Port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	if newCfg.Username == "" {
		jsonError(w, "Username is required", http.StatusBadRequest)
		return
	}

	if newCfg.MaxUploadSize <= 0 {
		newCfg.MaxUploadSize = 100 * 1024 * 1024 // default 100MB
	}

	if err := SaveConfig(newCfg); err != nil {
		jsonError(w, "Failed to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	clientMu.Lock()
	currentConfig = newCfg
	davClient = NewWebDAVClient("127.0.0.1", newCfg.Port, newCfg.Username, newCfg.Password)
	clientMu.Unlock()

	jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": "Configuration updated successfully",
	})
}

// handleList lists files and directories at the given path
func handleList(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		dirPath = "/"
	}

	// Sanitize path to prevent directory traversal
	dirPath = path.Clean(dirPath)
	if !strings.HasPrefix(dirPath, "/") {
		dirPath = "/" + dirPath
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	files, err := client.List(dirPath)
	if err != nil {
		jsonError(w, "Failed to list files: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"path":  dirPath,
		"files": files,
	})
}

// handleOpenFile returns file content or metadata for previewing
func handleOpenFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		jsonError(w, "Missing 'path' parameter", http.StatusBadRequest)
		return
	}

	filePath = path.Clean(filePath)
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	data, contentType, err := client.GetFileContent(filePath)
	if err != nil {
		jsonError(w, "Failed to open file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// For text-based files, return content as JSON
	if isTextContentType(contentType) || isTextFile(filePath) {
		jsonResponse(w, map[string]interface{}{
			"path":         filePath,
			"content":      string(data),
			"content_type": contentType,
			"size":         len(data),
		})
		return
	}

	// For binary files, return metadata only
	jsonResponse(w, map[string]interface{}{
		"path":         filePath,
		"content_type": contentType,
		"size":         len(data),
		"binary":       true,
		"message":      "Binary file - use download to retrieve",
	})
}

// handleDeleteFile deletes a file or directory
func handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" || req.Path == "/" {
		jsonError(w, "Cannot delete root directory", http.StatusBadRequest)
		return
	}

	req.Path = path.Clean(req.Path)
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := client.Delete(req.Path); err != nil {
		jsonError(w, "Failed to delete: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Deleted: %s", req.Path),
	})
}

// handleUploadFile handles file uploads
func handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientMu.RLock()
	maxUpload := currentConfig.MaxUploadSize
	clientMu.RUnlock()

	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		jsonError(w, "File too large or invalid form data", http.StatusBadRequest)
		return
	}

	destDir := r.FormValue("path")
	if destDir == "" {
		destDir = "/"
	}
	destDir = path.Clean(destDir)
	if !strings.HasPrefix(destDir, "/") {
		destDir = "/" + destDir
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	uploadPath := path.Join(destDir, header.Filename)
	if !strings.HasPrefix(uploadPath, "/") {
		uploadPath = "/" + uploadPath
	}

	if err := client.Upload(uploadPath, file); err != nil {
		jsonError(w, "Failed to upload file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("Uploaded: %s", uploadPath),
		"filename": header.Filename,
		"path":     uploadPath,
	})
}

// handleDownloadFile proxies file download from WebDAV
func handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		jsonError(w, "Missing 'path' parameter", http.StatusBadRequest)
		return
	}

	filePath = path.Clean(filePath)
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	body, contentType, contentLength, err := client.Download(filePath)
	if err != nil {
		jsonError(w, "Failed to download file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer body.Close()

	fileName := path.Base(filePath)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", fileName))
	if contentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", contentLength))
	}

	if _, err := copyWithLimit(w, body, 0); err != nil {
		// Headers already sent, can't return error
		return
	}
}

// handleRenameFile renames a file or directory
func handleRenameFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" || req.NewName == "" {
		jsonError(w, "Both 'path' and 'new_name' are required", http.StatusBadRequest)
		return
	}

	// Validate new name doesn't contain path separators
	if strings.Contains(req.NewName, "/") || strings.Contains(req.NewName, "\\") {
		jsonError(w, "New name cannot contain path separators", http.StatusBadRequest)
		return
	}

	req.Path = path.Clean(req.Path)
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}

	dir := path.Dir(req.Path)
	newPath := path.Join(dir, req.NewName)
	if !strings.HasPrefix(newPath, "/") {
		newPath = "/" + newPath
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := client.Rename(req.Path, newPath); err != nil {
		jsonError(w, "Failed to rename: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("Renamed to: %s", newPath),
		"new_path": newPath,
	})
}

// handleMoveFile moves a file to a different directory
func handleMoveFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path    string `json:"path"`
		DestDir string `json:"dest_dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" || req.DestDir == "" {
		jsonError(w, "Both 'path' and 'dest_dir' are required", http.StatusBadRequest)
		return
	}

	req.Path = path.Clean(req.Path)
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}
	req.DestDir = path.Clean(req.DestDir)
	if !strings.HasPrefix(req.DestDir, "/") {
		req.DestDir = "/" + req.DestDir
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := client.Move(req.Path, req.DestDir); err != nil {
		jsonError(w, "Failed to move: "+err.Error(), http.StatusInternalServerError)
		return
	}

	newPath := path.Join(req.DestDir, path.Base(req.Path))
	jsonResponse(w, map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("Moved to: %s", newPath),
		"new_path": newPath,
	})
}

// handleCutFile is an alias for move (cut + paste = move)
func handleCutFile(w http.ResponseWriter, r *http.Request) {
	handleMoveFile(w, r)
}

// handleSaveFile saves text content to a file via WebDAV PUT
func handleSaveFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		jsonError(w, "'path' is required", http.StatusBadRequest)
		return
	}

	req.Path = path.Clean(req.Path)
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := client.Upload(req.Path, strings.NewReader(req.Content)); err != nil {
		jsonError(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Saved: %s", req.Path),
	})
}

// handleNewFolder creates a new folder
func handleNewFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		jsonError(w, "'name' is required", http.StatusBadRequest)
		return
	}

	if strings.Contains(req.Name, "/") || strings.Contains(req.Name, "\\") {
		jsonError(w, "Folder name cannot contain path separators", http.StatusBadRequest)
		return
	}

	if req.Path == "" {
		req.Path = "/"
	}
	req.Path = path.Clean(req.Path)
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}

	folderPath := path.Join(req.Path, req.Name)
	if !strings.HasPrefix(folderPath, "/") {
		folderPath = "/" + folderPath
	}

	client := getClient()
	if client == nil {
		jsonError(w, "WebDAV client not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := client.MkDir(folderPath); err != nil {
		jsonError(w, "Failed to create folder: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Created folder: %s", folderPath),
		"path":    folderPath,
	})
}

// isTextContentType checks if the content type is text-based
func isTextContentType(ct string) bool {
	ct = strings.ToLower(ct)
	textTypes := []string{
		"text/", "application/json", "application/xml",
		"application/javascript", "application/ecmascript",
		"application/x-yaml", "application/toml",
		"application/xhtml+xml", "application/svg+xml",
	}
	for _, t := range textTypes {
		if strings.Contains(ct, t) {
			return true
		}
	}
	return false
}

// isTextFile checks if the file extension indicates a text file
func isTextFile(filePath string) bool {
	ext := strings.ToLower(path.Ext(filePath))
	textExts := map[string]bool{
		".txt": true, ".md": true, ".html": true, ".htm": true,
		".css": true, ".js": true, ".json": true, ".xml": true,
		".yaml": true, ".yml": true, ".toml": true, ".ini": true,
		".cfg": true, ".conf": true, ".sh": true, ".bash": true,
		".py": true, ".go": true, ".java": true, ".c": true,
		".cpp": true, ".h": true, ".hpp": true, ".rs": true,
		".ts": true, ".tsx": true, ".jsx": true, ".vue": true,
		".sql": true, ".csv": true, ".log": true, ".env": true,
		".gitignore": true, ".dockerfile": true, ".makefile": true,
		".svg": true, ".php": true, ".rb": true, ".pl": true,
	}
	return textExts[ext]
}

// copyWithLimit copies from reader to writer, with an optional limit (0 = no limit)
func copyWithLimit(w http.ResponseWriter, r interface{ Read([]byte) (int, error) }, limit int64) (int64, error) {
	buf := make([]byte, 32*1024) // 32KB buffer
	var written int64
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			nw, writeErr := w.Write(buf[:n])
			written += int64(nw)
			if writeErr != nil {
				return written, writeErr
			}
			if limit > 0 && written >= limit {
				return written, nil
			}
		}
		if readErr != nil {
			if readErr.Error() == "EOF" {
				break
			}
			return written, readErr
		}
	}
	return written, nil
}
