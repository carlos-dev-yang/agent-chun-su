package control

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"chunsu/internal/files"
)

const SocketPath = "state/control.sock"

type Request struct {
	Operation string          `json:"operation"`
	JobID     string          `json:"job_id,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Answer    string          `json:"answer,omitempty"`
}
type Response struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}
type Server struct {
	listener net.Listener
	done     chan struct{}
	workers  sync.WaitGroup
	cancel   context.CancelFunc
}
type Handler func(context.Context, Request) (any, error)

// Listen must be called only by the process holding the data-root writer lock.
func Listen(parent context.Context, root string, limit int64, timeout time.Duration, handler Handler) (*Server, error) {
	path := filepath.Join(root, SocketPath)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("management path is not a socket")
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, files.FileMode); err != nil {
		listener.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Server{listener: listener, done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(s.done)
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			s.workers.Add(1)
			go func() {
				defer s.workers.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(timeout))
				var req Request
				reader := io.LimitReader(conn, limit+1)
				decoder := json.NewDecoder(reader)
				decoder.DisallowUnknownFields()
				var response Response
				if e := decoder.Decode(&req); e != nil {
					response.Error = "invalid management request"
				} else {
					data, e := handler(ctx, req)
					if e != nil {
						response.Error = e.Error()
					} else {
						response.Data, e = json.Marshal(data)
						if e != nil {
							response.Error = "cannot encode management response"
						}
					}
				}
				_ = json.NewEncoder(conn).Encode(response)
			}()
		}
	}()
	return s, nil
}
func (s *Server) Close() error {
	s.cancel()
	err := s.listener.Close()
	<-s.done
	s.workers.Wait()
	return err
}

func Call(ctx context.Context, root string, timeout time.Duration, limit int64, req Request) (json.RawMessage, bool, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, true, err
	}
	if int64(len(data)) > limit {
		return nil, true, errors.New("management request exceeds byte limit")
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "unix", filepath.Join(root, SocketPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil, false, nil
		}
		return nil, true, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.Write(append(data, '\n')); err != nil {
		return nil, true, err
	}
	var response Response
	if err = json.NewDecoder(io.LimitReader(conn, limit+1)).Decode(&response); err != nil {
		return nil, true, err
	}
	if response.Error != "" {
		return nil, true, errors.New(response.Error)
	}
	return response.Data, true, nil
}
