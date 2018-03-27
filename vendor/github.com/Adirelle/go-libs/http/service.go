package http

import (
	"context"
	"net/http"
	"time"

	"github.com/Adirelle/go-libs/logging"
)

type Service struct {
	http.Server
	logging.Logger
}

func (w *Service) Serve() {
	w.Infof("listening on %s", w.Addr)
	err := w.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		w.Error(err)
	}
}

func (w *Service) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := w.Shutdown(ctx)
	if err != nil {
		w.Error(err)
	}
	w.Info("stopped")
}
