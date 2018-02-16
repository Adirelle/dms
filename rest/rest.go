package rest

import (
	"net/http"
	"os"
	"strings"

	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/logging"
	"github.com/jchannon/negotiator"
)

type Server struct {
	Prefix    string
	Directory cds.ContentDirectory
	L         logging.Logger

	negt *negotiator.Negotiator
}

type response struct {
	*cds.Object `xml:",omitempty"`
	Children    []*cds.Object `xml:"children>child,omitempty" json:",omitempty"`
}

func New(prefix string, Directory cds.ContentDirectory, logger logging.Logger) *Server {
	return &Server{
		Prefix:    prefix,
		Directory: Directory,
		L:         logger,
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
	data.Object, err = s.Directory.Get(id)
	if err == nil {
		data.Children, err = cds.GetChildren(s.Directory, id)
	}
	return
}
