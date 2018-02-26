package cds

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/anacrolix/dms/logging"
)

// ObjectURLGenerator builds URL paths for Objects
type ObjectPathGenerator interface {
	URLPath(o *Object) string
}

// For typing of context variables
type dmKeyType int

const (
	objectKey = dmKeyType(1)
	dmKey     = dmKeyType(2)
)

// DirectoryMiddleware parses and resolves object ID in URL Path
type DirectoryMiddleware struct {
	PathPrefix string
	Directory  ContentDirectory
	Handler    http.Handler
}

// ServeHTTP parses and resolves the object ID and passes the object to the
func (m *DirectoryMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, m.PathPrefix)
	obj, err := m.Directory.Get(id)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not found", http.StatusNotFound)
		} else if os.IsPermission(err) {
			http.Error(w, "Forbidden", http.StatusForbidden)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	m.Handler.ServeHTTP(
		w, r.WithContext(
			context.WithValue(
				context.WithValue(r.Context(), dmKey, *m),
				objectKey, obj,
			),
		),
	)
}

// URL generates a local URL for the given Object for the middleware.
func (h *DirectoryMiddleware) URLPath(o *Object) string {
	return h.PathPrefix + o.ID
}

// RequestObject extracts the Object from request context
func RequestObject(r *http.Request) (o *Object) {
	o, _ = r.Context().Value(objectKey).(*Object)
	return
}

// RequestDirectory extracts the ContentDirectory from request context
func RequestDirectory(r *http.Request) (d ContentDirectory) {
	d, _ = r.Context().Value(dmKey).(ContentDirectory)
	return
}

type FileServer struct {
	DirectoryMiddleware
}

type FileServerResource struct {
	Resource
	FilePath string
}

func NewFileServer(directory ContentDirectory, pathPrefix string, logger logging.Logger) *FileServer {
	fs := &FileServer{
		DirectoryMiddleware{
			Directory:  directory,
			PathPrefix: strings.TrimRight(pathPrefix, "/"),
		},
	}
	fs.Handler = http.HandlerFunc(fs.serveObjectContent)
	return fs
}

func (s *FileServer) serveObjectContent(w http.ResponseWriter, r *http.Request) {
	obj := RequestObject(r)
	fh, err := os.Open(obj.FilePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fh.Close()
	w.Header().Set("Content-Type", obj.MimeType().Value)
	http.ServeContent(w, r, obj.Name(), obj.ModTime(), fh)
}

func (s *FileServer) Process(obj *Object) {
	if !obj.IsContainer() {
		obj.AddResource(s.Resource(obj))
	}
}

func (s *FileServer) Resource(obj *Object) Resource {
	return Resource{
		URL:          s.URLPath(obj),
		Size:         uint64(obj.Size()),
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:*", obj.MimeType().Value),
		MimeType:     obj.MimeType(),
		FilePath:     obj.FilePath,
	}
}
