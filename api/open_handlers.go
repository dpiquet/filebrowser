package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"text/template"

	"github.com/go-chi/chi"
	"github.com/markbates/pkger"
)

// openHandlers provides router for all requests with no required auth
type openHandlers struct {
	BasePath string
	Revision string
}

// indexHandler returns index.html
func (h *openHandlers) indexHandler(w http.ResponseWriter, r *http.Request) {
	file, err := pkger.Open("/frontend/dist/index.html")
	if err != nil {
		SendErrorJSON(w, r, http.StatusInternalServerError, err, "index.html file read error", ErrCodeInternal)
		return
	}
	b, err := ioutil.ReadAll(file)
	if err != nil {
		SendErrorJSON(w, r, http.StatusInternalServerError, err, "failed to read index.html file", ErrCodeInternal)
		return
	}
	index := template.Must(template.New("index").Delims("[{[", "]}]").Parse(string(b)))

	data := map[string]interface{}{
		"Name":            "File Browser",
		"DisableExternal": true,
		"BaseURL":         h.BasePath,
		"Version":         h.Revision,
		"StaticURL":       path.Join(h.BasePath, "/static"),
		"Signup":          true,
		"NoAuth":          false,
		"AuthMethod":      "json",
		"LoginPage":       true,
		"CSS":             false,
		"ReCaptcha":       false,
		"Theme":           "",
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		SendErrorJSON(w, r, http.StatusInternalServerError, err, "data encoding error", ErrCodeInternal)
		return
	}

	data["Json"] = string(jsonData)

	if err := index.Execute(w, data); err != nil {
		SendErrorJSON(w, r, http.StatusInternalServerError, err, "failed to render template", ErrCodeInternal)
		return
	}
}

// staticHandler returns static assets
func (h *openHandlers) staticHandler(w http.ResponseWriter, r *http.Request) {
	rctx := chi.RouteContext(r.Context())
	pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
	fs := http.StripPrefix(pathPrefix, http.FileServer(pkger.Dir("/frontend/dist")))
	fs.ServeHTTP(w, r)
}
