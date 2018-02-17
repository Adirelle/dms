package rest

import (
	"context"
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
	ctx, cFunc := context.WithCancel(r.Context())
	defer cFunc()
	data, err := s.getResponse(strings.TrimPrefix(r.URL.Path, s.Prefix), ctx)
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

func (s *Server) getResponse(id string, ctx context.Context) (data response, err error) {
	data.Object, err = s.Directory.Get(id)
	if err != nil {
		return
	}
	children, errs := cds.GetChildren(s.Directory, id, ctx)
	open := true
	var child *cds.Object
	for open {
		select {
		case _, open = <-ctx.Done():
			err = context.Canceled
		case child, open = <-children:
			if open {
				data.Children = append(data.Children, child)
			}
		case err, open = <-errs:
			if open {
				s.L.Warn(err)
				err = nil
			}
		}
	}
	cds.SortObjects(data.Children)
	return
}
