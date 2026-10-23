package storage

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/spf13/afero"
)

// DevS3 is an in-process S3-compatible server for local development and
// tests. It does not check signatures and must never face the internet.
type DevS3 struct {
	URL    string
	server *http.Server
}

// StartDevS3 listens on addr ("127.0.0.1:0" picks a port). With dir set,
// objects persist there across restarts; otherwise they live in memory.
func StartDevS3(addr, dir string) (*DevS3, error) {
	var backend gofakes3.Backend
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		b, err := s3afero.MultiBucket(afero.NewBasePathFs(afero.NewOsFs(), filepath.Clean(dir)))
		if err != nil {
			return nil, fmt.Errorf("open dev s3 store: %w", err)
		}
		backend = b
	} else {
		backend = s3mem.New()
	}
	fake := gofakes3.New(backend, gofakes3.WithAutoBucket(true))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen dev s3: %w", err)
	}
	srv := &http.Server{Handler: fake.Server(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "dev s3 stopped: %v\n", err)
		}
	}()
	return &DevS3{URL: "http://" + ln.Addr().String(), server: srv}, nil
}

func (d *DevS3) Close() error { return d.server.Close() }
