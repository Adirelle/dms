package cds

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/anacrolix/dms/logging"
	"github.com/gorilla/mux"
)

type FileServer struct {
	directory ContentDirectory
	route     *mux.Route
	l         logging.Logger
}

const FileServerPrefix = "/files"

func NewFileServer(directory ContentDirectory, router *mux.Router, logger logging.Logger) (fs *FileServer) {
	fs = &FileServer{directory: directory, l: logger}
	fs.route = router.Methods("HEAD", "GET").
		PathPrefix(FileServerPrefix + "/").
		Handler(fs)
	return
}

func (s *FileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, FileServerPrefix)
	err := s.serveObjectContent(w, r, id)
	if err == nil {
		return
	}
	if os.IsNotExist(err) {
		http.Error(w, "Not found", http.StatusNotFound)
	} else if os.IsPermission(err) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *FileServer) serveObjectContent(w http.ResponseWriter, r *http.Request, id string) (err error) {
	obj, err := s.directory.Get(id)
	if err != nil {
		return
	} else if obj.IsContainer() {
		return os.ErrPermission
	}
	fh, err := os.Open(obj.FilePath)
	if err != nil {
		return
	}
	defer fh.Close()
	mimeType := obj.MimeType()
	w.Header().Set("Content-Type", mimeType.Value)
	http.ServeContent(w, r, obj.Name(), obj.ModTime(), fh)
	return
}

func (s *FileServer) Process(obj *Object) (err error) {
	if obj.IsContainer() {
		return
	}
	url, err := s.URL(obj)
	if err == nil {
		obj.AddResource(Resource{
			URL:          url.String(),
			Size:         uint64(obj.Size()),
			ProtocolInfo: fmt.Sprintf("http-get:*:%s:*", obj.MimeType().Value),
		})
	}
	return
}

func (s *FileServer) URL(obj *Object) (url *url.URL, err error) {
	url, err = s.route.URL()
	if err == nil {
		url.Path += strings.TrimPrefix(obj.ID, "/")
	}
	return
}
