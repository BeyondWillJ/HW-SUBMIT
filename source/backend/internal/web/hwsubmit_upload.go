package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gtsteffaniak/filebrowser/backend/internal/utils"
	"github.com/gtsteffaniak/go-cache/cache"
)

var hwsubmitPageOrigin = strings.TrimRight(os.Getenv("HW_SUBMIT_PAGE_ORIGIN"), "/")
var hwsubmitDirectOrigin = strings.TrimRight(os.Getenv("HW_SUBMIT_UPLOAD_ORIGIN"), "/")

const hwsubmitUploadHeader = "X-Hwsubmit-Upload-Token"

type hwsubmitUploadTicket struct {
	Token, Source, Path, Session string
	Total                        int64
	Override                     bool
	Expires                      time.Time
}

var hwsubmitTickets = struct {
	sync.Mutex
	items map[string]hwsubmitUploadTicket
}{items: make(map[string]hwsubmitUploadTicket)}

// A ticket carries no login credential to the browser. It is restricted to one
// destination, file size and upload session; normal auth and permissions are
// checked again for every chunk, including after logout or account revocation.
func hwsubmitUploadTicketHandler(w http.ResponseWriter, r *http.Request, d *Context) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	if currentCourse().Slug == "" || d.Token == "" {
		return http.StatusNotFound, fmt.Errorf("upload acceleration unavailable")
	}
	var input struct {
		Source   string `json:"source"`
		Path     string `json:"path"`
		Session  string `json:"session"`
		Total    int64  `json:"total"`
		Override bool   `json:"override"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		return http.StatusBadRequest, fmt.Errorf("invalid upload ticket request")
	}
	path, err := utils.SanitizePath(input.Path)
	if err != nil || path != input.Path || input.Total <= 0 {
		return http.StatusBadRequest, fmt.Errorf("invalid upload destination or size")
	}
	test := &http.Request{Header: make(http.Header)}
	test.Header.Set(uploadSessionHeader, input.Session)
	if _, err := parseUploadSession(test); err != nil {
		return http.StatusBadRequest, err
	}
	if status, err := studentUploadCheck(d.User, path, false); err != nil {
		return status, err
	}
	if _, err := d.User.GetScopeForSourceName(input.Source); err != nil {
		return http.StatusForbidden, fmt.Errorf("source access denied")
	}
	perms, err := effectiveFilePerms(d, input.Source)
	if err != nil || !perms.Create || (isStudent(d) && input.Override) {
		return http.StatusForbidden, fmt.Errorf("upload access denied")
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return http.StatusInternalServerError, err
	}
	id := hex.EncodeToString(random[:])
	now := time.Now()
	ticket := hwsubmitUploadTicket{Token: d.Token, Source: input.Source, Path: path,
		Session: input.Session, Total: input.Total, Override: input.Override, Expires: now.Add(10 * time.Minute)}
	hwsubmitTickets.Lock()
	for key, item := range hwsubmitTickets.items {
		if now.After(item.Expires) {
			delete(hwsubmitTickets.items, key)
		}
	}
	if len(hwsubmitTickets.items) >= 4096 {
		hwsubmitTickets.Unlock()
		return http.StatusTooManyRequests, fmt.Errorf("too many active uploads")
	}
	hwsubmitTickets.items[id] = ticket
	hwsubmitTickets.Unlock()
	w.Header().Set("Content-Type", "application/json")
	return 0, json.NewEncoder(w).Encode(map[string]interface{}{
		"token": id, "expires": ticket.Expires.UnixMilli(),
		"url": hwsubmitDirectOrigin + "/hwsubmit-upload/" + currentCourse().Slug + "/chunk",
	})
}

func hwsubmitUploadCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Origin")
	if r.Header.Get("Origin") != hwsubmitPageOrigin {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", hwsubmitPageOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Hwsubmit-Upload-Token, X-File-Upload-Session, X-File-Chunk-Offset, X-File-Total-Size")
	w.Header().Set("Access-Control-Max-Age", "600")
	return true
}

// The direct virtual host routes only health and POST/OPTIONS chunk requests.
// Cookie and Authorization headers from that host are never used here.
func hwsubmitDirectUploadHandler(w http.ResponseWriter, r *http.Request) {
	if !hwsubmitUploadCORS(w, r) {
		http.Error(w, "origin denied", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.Header.Get(hwsubmitUploadHeader)
	hwsubmitTickets.Lock()
	ticket, ok := hwsubmitTickets.items[id]
	hwsubmitTickets.Unlock()
	if !ok || time.Now().After(ticket.Expires) {
		http.Error(w, "upload ticket expired", http.StatusUnauthorized)
		return
	}
	total, present, err := parseUploadTotalSize(r)
	if err != nil || !present || total != ticket.Total ||
		r.Header.Get(uploadSessionHeader) != ticket.Session || r.Header.Get("X-File-Chunk-Offset") == "" {
		http.Error(w, "upload ticket does not match file", http.StatusForbidden)
		return
	}
	r = r.Clone(r.Context())
	query := r.URL.Query()
	// Discard client-supplied destination/override/auth query parameters.
	for key := range query {
		query.Del(key)
	}
	query.Set("source", ticket.Source)
	query.Set("path", ticket.Path)
	if ticket.Override {
		query.Set("override", "true")
	}
	r.URL.RawQuery = query.Encode()
	r.Header.Del("Cookie")
	r.Header.Set("Authorization", "Bearer "+ticket.Token)
	withUser(ResourcePostHandler)(w, r)
}

func hwsubmitDirectHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Origin")
	if r.Header.Get("Origin") != hwsubmitPageOrigin {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", hwsubmitPageOrigin)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// Requests for one path serialize, including a retry arriving on another route
// while the original connection is still unwinding. Other files run in parallel.
type uploadPathGate struct {
	ready chan struct{}
	refs  int
}

var uploadPathGates = struct {
	sync.Mutex
	items map[string]*uploadPathGate
}{items: make(map[string]*uploadPathGate)}

func lockUploadPath(ctx context.Context, path string) (func(), error) {
	uploadPathGates.Lock()
	gate := uploadPathGates.items[path]
	if gate == nil {
		gate = &uploadPathGate{ready: make(chan struct{}, 1)}
		uploadPathGates.items[path] = gate
	}
	gate.refs++
	uploadPathGates.Unlock()
	drop := func() {
		uploadPathGates.Lock()
		gate.refs--
		if gate.refs == 0 {
			delete(uploadPathGates.items, path)
		}
		uploadPathGates.Unlock()
	}
	select {
	case gate.ready <- struct{}{}:
		return func() { <-gate.ready; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

type completedUpload struct{ Total int64 }

var completedUploads = cache.NewCache[completedUpload](10 * time.Minute)

func completedUploadKey(path, session string) string {
	return path + "\x00" + strings.TrimSpace(session)
}
