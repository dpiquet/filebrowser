package api

import (
	"net/http"

	"github.com/go-chi/render"
	"github.com/go-pkgz/rest"

	"github.com/filebrowser/filebrowser/v3/log"
)

// All error codes for UI mapping and translation
const (
	ErrCodeInternal = 0 // any internal error
	ErrDecode       = 1 // failed to unmarshal incoming request
	ErrNoAccess     = 2 // rejected by auth
	ErrUserBlocked  = 3 // user blocked
	ErrReadOnly     = 4 // write failed on read only
)

// SendErrorJSON makes {"error": "blah", "details": "blah", "code": 0} json body and responds with error code
func SendErrorJSON(w http.ResponseWriter, r *http.Request, httpStatusCode int, err error, details string, errCode int) {
	log.WithContext(r.Context()).WithFields(log.Fields{
		"http_status": httpStatusCode,
		"details":     details,
		"error_code":  errCode,
	}).Warnf("%+v", err)

	render.Status(r, httpStatusCode)
	render.JSON(w, r, rest.JSON{"error": err.Error(), "details": details, "code": errCode})
}
