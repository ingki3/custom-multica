package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type listDirsResponse struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"`
	Entries []fsEntry `json:"entries"`
}

// ListDirectories returns the list of subdirectories under the requested path.
// Query params:
//
//	path       – absolute directory path to list (defaults to the user's home directory)
//	showHidden – if "true", include dot-prefixed directories
func (h *Handler) ListDirectories(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("path")
	showHidden := r.URL.Query().Get("showHidden") == "true"
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cannot determine home directory")
			return
		}
		dir = home
	}

	// Normalise and ensure the path is absolute.
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "directory not found")
			return
		}
		if os.IsPermission(err) {
			writeError(w, http.StatusForbidden, "permission denied")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read directory")
		return
	}

	dirs := make([]fsEntry, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden directories (dot-prefixed) unless explicitly requested.
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		// Skip well-known non-user directories on macOS/Linux.
		if runtime.GOOS != "windows" && dir == "/" {
			switch name {
			case "proc", "sys", "dev", "run", "snap", "boot", "sbin", "bin", "lib", "lib64":
				continue
			}
		}
		dirs = append(dirs, fsEntry{
			Name: name,
			Path: filepath.Join(dir, name),
		})
	}

	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := filepath.Dir(dir)
	if parent == dir {
		// Already at root.
		parent = ""
	}

	writeJSON(w, http.StatusOK, listDirsResponse{
		Path:    dir,
		Parent:  parent,
		Entries: dirs,
	})
}
