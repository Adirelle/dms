package rest

import (
	"context"
	"fmt"
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

	negt *negotiator.Negotiator
}

type response struct {
	*cds.Object `xml:",omitempty"`
	Children    []*cds.Object `xml:"children>child,omitempty" json:",omitempty"`
}

func New(prefix string, Directory cds.ContentDirectory) *Server {
	return &Server{
		Prefix:    prefix,
		Directory: Directory,
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
		if err == nil {
			return
		}
	}
	if os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusNotFound)
	} else {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) getResponse(id string, ctx context.Context) (data response, err error) {
	data.Object, err = s.Directory.Get(id)
	if err != nil {
		err = fmt.Errorf("error getting %q: %s", id, err.Error())
		return
	}
	children, errs := cds.GetChildren(s.Directory, id, ctx)
	open := true
	logger := logging.MustFromContext(ctx)
	var (
		child *cds.Object
		warn  error
	)
	for open {
		select {
		case _, open = <-ctx.Done():
			err = context.Canceled
		case child, open = <-children:
			if open {
				data.Children = append(data.Children, child)
			}
		case warn, open = <-errs:
			if open {
				logger.Warn(warn)
			}
		}
	}
	cds.SortObjects(data.Children)
	return
}
