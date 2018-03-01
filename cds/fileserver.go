package cds

import (
	"net/http"
	"os"

	"github.com/anacrolix/dms/filesystem"
	dmsHttp "github.com/anacrolix/dms/http"
	"github.com/gorilla/mux"
)

// For typing of context variables
type dmKeyType int

const (
	objectKey = dmKeyType(1)
	dmKey     = dmKeyType(2)

	FileServerRoute        = "fileserver"
	RouteObjectIDParameter = "objectID"
	RouteObjectIDTemplate  = "{objectID:/.*}"
)

// ObjectHandler
type ObjectHandler interface {
	ServeObject(http.ResponseWriter, *http.Request, *Object)
}

// ObjectHandlerFunc
type ObjectHandlerFunc func(http.ResponseWriter, *http.Request, *Object)

func (f ObjectHandlerFunc) ServeObject(w http.ResponseWriter, r *http.Request, o *Object) {
	f(w, r, o)
}

// DirectoryHandler
type DirectoryHandler struct {
	Directory ContentDirectory
	Handler   ObjectHandler
}

// ServeHTTP parses and resolves the object ID and passes the object to the
func (h *DirectoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := filesystem.ParseObjectID(vars[RouteObjectIDParameter])
	var o *Object
	if err == nil {
		o, err = h.Directory.Get(id)
	}
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
	h.Handler.ServeObject(w, r, o)
}

type FileServer struct {
	DirectoryHandler
}

func NewFileServer(d ContentDirectory) *FileServer {
	fs := &FileServer{}
	fs.DirectoryHandler = DirectoryHandler{d, fs}
	return fs
}

func (s *FileServer) ServeObject(w http.ResponseWriter, r *http.Request, obj *Object) {
	fh, err := os.Open(obj.FilePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer fh.Close()
	w.Header().Set("Content-Type", obj.MimeType.Value)
	http.ServeContent(w, r, obj.Name(), obj.ModTime(), fh)
}

func (s *FileServer) Process(obj *Object) {
	if !obj.IsContainer() {
		obj.AddResource(Resource{
			URL:      FileServerURLSpec(obj),
			Size:     uint64(obj.Size()),
			MimeType: obj.MimeType,
			FilePath: obj.FilePath,
		})
	}
}

func FileServerURLSpec(obj *Object) *dmsHttp.URLSpec {
	return dmsHttp.NewURLSpec(FileServerRoute, RouteObjectIDParameter, obj.ID.String())
}
