package rest

import (
	"context"
	"net/http"
	"os"

	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/didl_lite"
	dmsHttp "github.com/anacrolix/dms/http"
	"github.com/anacrolix/dms/logging"
	"github.com/jchannon/negotiator"
)

const RouteName = "rest"

type Server struct {
	cds.DirectoryHandler
	negt *negotiator.Negotiator
}

type response struct {
	didl_lite.Object `xml:",omitempty"`
	Children         []didl_lite.Object `xml:"children>child,omitempty" json:",omitempty"`
}

func New(d cds.ContentDirectory) *Server {
	s := &Server{
		negt: negotiator.New(
			negotiator.NewJSONIndent2Spaces(),
			negotiator.NewXMLIndent2Spaces(),
			htmlProcessor{},
		),
	}
	s.DirectoryHandler = cds.DirectoryHandler{d, s}
	return s
}

func (s *Server) ServeObject(w http.ResponseWriter, r *http.Request, o *cds.Object) {
	ctx, cFunc := context.WithCancel(r.Context())
	defer cFunc()
	data, err := s.getResponse(o, ctx)
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

func (s *Server) getResponse(o *cds.Object, ctx context.Context) (data response, err error) {
	urlGen := dmsHttp.URLGeneratorFromContext(ctx)
	data.Object, err = o.MarshalDIDLLite(urlGen)
	if err != nil {
		return
	}
	children, errs := cds.GetChildren(s.Directory, o, ctx)
	logger := logging.MustFromContext(ctx)
	for true {
		select {
		case <-ctx.Done():
			err = context.Canceled
			return
		case child, open := <-children:
			if !open {
				return
			}
			if obj, err := child.MarshalDIDLLite(urlGen); err == nil {
				data.Children = append(data.Children, obj)
			} else {
				logger.Warn(err)
			}
		case warn, open := <-errs:
			if !open {
				return
			}
			logger.Warn(warn)
		}
	}
	return
}
