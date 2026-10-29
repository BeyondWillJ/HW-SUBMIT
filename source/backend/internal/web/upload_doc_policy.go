package web

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/gtsteffaniak/filebrowser/backend/internal/database/users"
)

// HW-SUBMIT: students can only submit documents.
// Non-admin accounts may upload files straight into their own folder and nothing else.
var submitDocumentExtensions = map[string]struct{}{
	// 文字处理
	".doc": {}, ".docx": {}, ".docm": {}, ".dot": {}, ".dotx": {}, ".dotm": {},
	".rtf": {}, ".odt": {}, ".ott": {}, ".wps": {}, ".wpt": {}, ".pages": {},
	".txt": {}, ".md": {}, ".markdown": {}, ".tex": {},
	// 表格
	".xls": {}, ".xlsx": {}, ".xlsm": {}, ".xlsb": {}, ".xlt": {}, ".xltx": {}, ".xltm": {},
	".csv": {}, ".ods": {}, ".ots": {}, ".et": {}, ".ett": {}, ".numbers": {},
	// 演示文稿
	".ppt": {}, ".pptx": {}, ".pptm": {}, ".pps": {}, ".ppsx": {}, ".pot": {}, ".potx": {}, ".potm": {},
	".odp": {}, ".otp": {}, ".dps": {}, ".dpt": {}, ".key": {},
	// 版式与电子书
	".pdf": {}, ".ofd": {}, ".xps": {}, ".oxps": {}, ".caj": {}, ".djvu": {}, ".epub": {}, ".mobi": {},
}

func isSubmitDocument(name string) bool {
	ext := path.Ext(name)
	if len(ext) == len(name) { // ".docx" alone has no file name
		return false
	}
	allowed := courseExtensionSet()
	if allowed == nil {
		allowed = submitDocumentExtensions
	}
	_, ok := allowed[strings.ToLower(ext)]
	return ok
}

// studentUploadCheck limits non-admin uploads to document files at the top of the
// user's own folder. Folders and nested paths are refused.
func studentUploadCheck(u *users.User, cleanPath string, isDir bool) (int, error) {
	if u == nil || u.Permissions.Admin {
		return 0, nil
	}
	if isDir {
		return http.StatusForbidden, fmt.Errorf("creating folders is not allowed")
	}
	name := strings.TrimPrefix(cleanPath, "/")
	if name == "" || strings.Contains(name, "/") {
		return http.StatusForbidden, fmt.Errorf("files must be uploaded to your own folder")
	}
	if !isSubmitDocument(name) {
		return http.StatusUnsupportedMediaType, fmt.Errorf("only document files can be submitted")
	}
	return 0, nil
}

// isStudent reports a logged-in, non-admin account outside of share links.
// Students may list their own folder and upload, but never read file content.
func isStudent(d *Context) bool {
	return d != nil && d.Share.Hash == "" && d.User != nil && !d.User.Permissions.Admin &&
		d.User.Username != "" && d.User.Username != users.AnonymousUserName
}

// A student can list their root folder, without asking the file reader to
// extract content, media metadata, or checksums from submitted files.
func studentResourceReadCheck(d *Context, cleanPath string, r *http.Request) (int, error) {
	if !isStudent(d) {
		return 0, nil
	}
	q := r.URL.Query()
	if cleanPath != "/" || q.Get("content") == "true" || q.Get("metadata") == "true" || q.Get("checksum") != "" {
		return http.StatusForbidden, fmt.Errorf("students may only list their submission folder")
	}
	return 0, nil
}

// denyStudentContent wraps routes that return file content (inline view, previews,
// media, office) so students get 403 even though they have the view permission.
func denyStudentContent(fn HandleFunc) HandleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *Context) (int, error) {
		if isStudent(d) {
			drainRequestBody(r)
			return http.StatusForbidden, fmt.Errorf("opening files is not allowed")
		}
		return fn(w, r, d)
	}
}
