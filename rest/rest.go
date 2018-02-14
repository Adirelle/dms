package rest

import (
	"net/http"
	"os"
	"strings"

	"github.com/anacrolix/dms/content_directory"
	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/logging"
	"github.com/jchannon/negotiator"
)

type Server struct {
	Prefix     string
	Filesystem filesystem.Filesystem
	Backend    content_directory.Backend
	L          logging.Logger

	negt *negotiator.Negotiator
}

type response struct {
	content_directory.Object `xml:",omitempty"`
	Children                 []content_directory.Object `xml:"children>child,omitempty" json:",omitempty"`
}

func New(prefix string, fs filesystem.Filesystem, backend content_directory.Backend, logger logging.Logger) *Server {
	return &Server{
		Prefix:     prefix,
		Filesystem: fs,
		Backend:    backend,
		L:          logger,
		negt: negotiator.New(
			negotiator.NewJSONIndent2Spaces(),
			negotiator.NewXMLIndent2Spaces(),
			htmlProcessor{prefix},
		),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, err := s.getResponse(strings.TrimPrefix(r.URL.Path, s.Prefix))
	if err == nil {
		err = s.negt.Negotiate(w, r, data)
	}
	if err == nil {
		return
	}
	s.L.Warn(err)
	if os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusNotFound)
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) getResponse(id string) (data response, err error) {
	fsObj, err := s.Filesystem.GetObjectByID(id)
	if err != nil {
		return
	}
	data.Object, err = s.Backend.Get(fsObj)
	if err == nil && fsObj.IsDir() {
		data.Children, err = s.getChildren(fsObj.(filesystem.Directory))
	}
	return
}

func (s *Server) getChildren(fsObj filesystem.Directory) (children []content_directory.Object, err error) {
	fsChildren, err := fsObj.Children()
	if err != nil {
		return
	}
	children = make([]content_directory.Object, 0, len(fsChildren))
	for _, child := range fsChildren {
		if obj, err := s.Backend.Get(child); err == nil {
			children = append(children, obj)
		} else {
			s.L.Warnf("%s: %s", child, err)
		}
	}
	return
}
