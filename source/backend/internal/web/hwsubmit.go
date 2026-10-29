package web

// HW-SUBMIT course settings and student deletion requests.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gtsteffaniak/filebrowser/backend/internal/adapters/fs/files"
	"github.com/gtsteffaniak/filebrowser/backend/internal/state"
	"github.com/gtsteffaniak/filebrowser/backend/internal/utils"
	"github.com/gtsteffaniak/go-logger/logger"
)

type hwsubmitCourse struct {
	Name            string   `json:"name"`
	Slug            string   `json:"slug"`
	Extensions      []string `json:"extensions"`
	DataDir         string   `json:"dataDir"`
	ReviewTokenFile string   `json:"reviewTokenFile"`
}

var (
	courseOnce sync.Once
	course     hwsubmitCourse
)

func currentCourse() hwsubmitCourse {
	courseOnce.Do(func() {
		p := os.Getenv("HW_SUBMIT_COURSE_FILE")
		if p == "" {
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			logger.Errorf("hwsubmit: cannot read course file %s: %v", p, err)
			return
		}
		if err := json.Unmarshal(b, &course); err != nil {
			logger.Errorf("hwsubmit: bad course file %s: %v", p, err)
			return
		}
		for i, e := range course.Extensions {
			e = strings.ToLower(strings.TrimSpace(e))
			if e != "" && !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			course.Extensions[i] = e
		}
	})
	return course
}

// courseExtensionSet returns the course's allowed extensions, or nil to use the built-in list.
func courseExtensionSet() map[string]struct{} {
	c := currentCourse()
	if len(c.Extensions) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(c.Extensions))
	for _, e := range c.Extensions {
		m[e] = struct{}{}
	}
	return m
}

// courseFrontendVars is exposed to the page as globalVars.hwsubmitCourse.
func courseFrontendVars() map[string]interface{} {
	c := currentCourse()
	exts := c.Extensions
	if len(exts) == 0 {
		exts = make([]string, 0, len(submitDocumentExtensions))
		for e := range submitDocumentExtensions {
			exts = append(exts, e)
		}
		sort.Strings(exts)
	}
	return map[string]interface{}{"name": c.Name, "slug": c.Slug, "extensions": exts, "uploadHealthURL": hwsubmitDirectOrigin + "/hwsubmit-upload/" + c.Slug + "/health", "uploadChunkSizeMb": 16}
}

// ---------- delete requests ----------

type deleteRequest struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	StudentName string `json:"studentName"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Reason      string `json:"reason"`
	Status      string `json:"status"` // pending, approved, rejected, failed
	Note        string `json:"note,omitempty"`
	Created     int64  `json:"created"`
	Decided     int64  `json:"decided,omitempty"`
	Notified    bool   `json:"notified"`
}

var requestsMu sync.Mutex

func requestsFile() string {
	dir := currentCourse().DataDir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "hwsubmit")
	}
	return filepath.Join(dir, "delete-requests.json")
}

func loadRequests() ([]deleteRequest, error) {
	b, err := os.ReadFile(requestsFile())
	if os.IsNotExist(err) {
		return []deleteRequest{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []deleteRequest
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func saveRequests(list []deleteRequest) error {
	p := requestsFile()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// studentNameFromScope turns "/00000001_示例学生" into "示例学生".
func studentNameFromScope(scope string) string {
	base := filepath.Base(strings.TrimRight(scope, "/"))
	if i := strings.Index(base, "_"); i > 0 {
		return base[i+1:]
	}
	return base
}

// studentFile resolves a top-level file in the student's own folder.
func studentFile(username, source, p string) (realPath string, isDir bool, err error) {
	clean, err := utils.SanitizePath(p)
	if err != nil {
		return "", false, err
	}
	name := strings.TrimPrefix(clean, "/")
	if name == "" || strings.Contains(name, "/") {
		return "", false, fmt.Errorf("invalid path")
	}
	u, err := state.GetUserByUsername(username)
	if err != nil {
		return "", false, err
	}
	info, err := files.FileInfoFaster(utils.FileOptions{Path: clean, Source: source, Expand: false}, &u)
	if err != nil {
		return "", false, err
	}
	return info.RealPath, info.Type == "directory", nil
}

type createDeleteRequestBody struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// deleteRequestCreateHandler lets a student ask for one of their files to be deleted.
func deleteRequestCreateHandler(w http.ResponseWriter, r *http.Request, d *Context) (int, error) {
	if !isStudent(d) {
		return http.StatusForbidden, fmt.Errorf("only students file delete requests")
	}
	var body createDeleteRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		return http.StatusBadRequest, fmt.Errorf("invalid request body")
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Reason == "" || utf8.RuneCountInString(body.Reason) > 200 {
		return http.StatusBadRequest, fmt.Errorf("reason must be 1-200 characters")
	}
	if _, isDir, err := studentFile(d.User.Username, body.Source, body.Path); err != nil || isDir {
		return http.StatusNotFound, fmt.Errorf("file not found")
	}
	clean, _ := utils.SanitizePath(body.Path)
	scope, err := d.User.GetScopeForSourceName(body.Source)
	if err != nil {
		return http.StatusForbidden, err
	}

	requestsMu.Lock()
	defer requestsMu.Unlock()
	list, err := loadRequests()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	for _, q := range list {
		if q.Username == d.User.Username && q.Source == body.Source && q.Path == clean && q.Status == "pending" {
			return http.StatusConflict, fmt.Errorf("a request for this file is already pending")
		}
	}
	req := deleteRequest{
		ID:          newRequestID(),
		Username:    d.User.Username,
		StudentName: studentNameFromScope(scope),
		Source:      body.Source,
		Path:        clean,
		Reason:      body.Reason,
		Status:      "pending",
		Created:     time.Now().Unix(),
	}
	list = append(list, req)
	if err := saveRequests(list); err != nil {
		return http.StatusInternalServerError, err
	}
	return renderJSON(w, r, req, http.StatusCreated)
}

// deleteRequestListHandler returns the latest request per file for the current student.
func deleteRequestListHandler(w http.ResponseWriter, r *http.Request, d *Context) (int, error) {
	if !isStudent(d) {
		return renderJSON(w, r, []deleteRequest{}, http.StatusOK)
	}
	requestsMu.Lock()
	list, err := loadRequests()
	requestsMu.Unlock()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	latest := map[string]deleteRequest{}
	for _, q := range list {
		if q.Username != d.User.Username {
			continue
		}
		k := q.Source + "\x00" + q.Path
		if old, ok := latest[k]; !ok || q.Created >= old.Created {
			latest[k] = q
		}
	}
	out := make([]deleteRequest, 0, len(latest))
	for _, q := range latest {
		if q.Status == "approved" {
			continue // the file is gone
		}
		out = append(out, q)
	}
	return renderJSON(w, r, out, http.StatusOK)
}

// ---------- bot-only endpoints ----------

func reviewTokenOK(r *http.Request) bool {
	p := currentCourse().ReviewTokenFile
	if p == "" {
		return false
	}
	want, err := os.ReadFile(p)
	if err != nil || len(strings.TrimSpace(string(want))) < 32 {
		return false
	}
	got := r.Header.Get("X-HwSubmit-Review-Token")
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(want))), []byte(got)) == 1
}

func withReviewToken(fn func(http.ResponseWriter, *http.Request) (int, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !reviewTokenOK(r) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if status, err := fn(w, r); err != nil {
			http.Error(w, err.Error(), status)
		}
	}
}

// internalPendingHandler lists requests the bot has not announced yet, plus the course name.
func internalPendingHandler(w http.ResponseWriter, r *http.Request) (int, error) {
	requestsMu.Lock()
	list, err := loadRequests()
	requestsMu.Unlock()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	out := []deleteRequest{}
	for _, q := range list {
		if q.Status == "pending" && !q.Notified {
			out = append(out, q)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	return 0, json.NewEncoder(w).Encode(map[string]interface{}{"course": currentCourse(), "requests": out})
}

type internalActionBody struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // approve or reject; empty for "notified"
}

func internalNotifiedHandler(w http.ResponseWriter, r *http.Request) (int, error) {
	var body internalActionBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	list, err := loadRequests()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	for i := range list {
		if list[i].ID == body.ID {
			list[i].Notified = true
			if err := saveRequests(list); err != nil {
				return http.StatusInternalServerError, err
			}
			w.WriteHeader(http.StatusNoContent)
			return 0, nil
		}
	}
	return http.StatusNotFound, fmt.Errorf("request not found")
}

// internalDecideHandler applies the teacher's decision. Approve deletes the file.
func internalDecideHandler(w http.ResponseWriter, r *http.Request) (int, error) {
	var body internalActionBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}
	if body.Decision != "approve" && body.Decision != "reject" {
		return http.StatusBadRequest, fmt.Errorf("decision must be approve or reject")
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	list, err := loadRequests()
	if err != nil {
		return http.StatusInternalServerError, err
	}
	idx := -1
	for i := range list {
		if list[i].ID == body.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return http.StatusNotFound, fmt.Errorf("request not found")
	}
	q := &list[idx]
	if q.Status != "pending" {
		w.Header().Set("Content-Type", "application/json")
		return 0, json.NewEncoder(w).Encode(q) // already decided: report the stored result
	}
	q.Decided = time.Now().Unix()
	if body.Decision == "reject" {
		q.Status = "rejected"
	} else {
		realPath, isDir, ferr := studentFile(q.Username, q.Source, q.Path)
		switch {
		case ferr != nil:
			q.Status, q.Note = "failed", "文件已不存在"
		case isDir:
			q.Status, q.Note = "failed", "不是文件"
		default:
			if derr := files.DeleteFiles(q.Source, realPath, false); derr != nil {
				q.Status, q.Note = "failed", "删除失败"
				logger.Errorf("hwsubmit: delete %s failed: %v", realPath, derr)
			} else {
				q.Status = "approved"
			}
		}
	}
	if err := saveRequests(list); err != nil {
		return http.StatusInternalServerError, err
	}
	w.Header().Set("Content-Type", "application/json")
	return 0, json.NewEncoder(w).Encode(q)
}

func renderJSON(w http.ResponseWriter, r *http.Request, v interface{}, status int) (int, error) {
	return RenderJSON(w, r, v, status)
}
