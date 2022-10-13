package tokay

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

func listenAndServe(engine *Engine, addr string) error {
	s := engine.Server
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	if tcpln, ok := ln.(*net.TCPListener); ok {
		listener := NewGracefulListener(tcpKeepaliveListener{
			TCPListener:     tcpln,
			keepalive:       s.TCPKeepalive,
			keepalivePeriod: s.TCPKeepalivePeriod,
		}, engine.maxGracefulWaitTime)
		engine.Close = listener.Close
		return s.Serve(listener)
	}
	return s.Serve(ln)
}

// ListenAndServeTLS serves HTTPS requests from the given TCP4 addr.
//
// certFile and keyFile are paths to TLS certificate and key files.
//
// Pass custom listener to Serve if you need listening on non-TCP4 media
// such as IPv6.
//
// If the certFile or keyFile has not been provided to the server structure,
// the function will use the previously added TLS configuration.
//
// Accepted connections are configured to enable TCP keep-alives.
func listenAndServeTLS(engine *Engine, addr, certFile, keyFile string) error {
	s := engine.Server
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	if tcpln, ok := ln.(*net.TCPListener); ok {
		listener := NewGracefulListener(tcpKeepaliveListener{
			TCPListener:     tcpln,
			keepalive:       s.TCPKeepalive,
			keepalivePeriod: s.TCPKeepalivePeriod,
		}, engine.maxGracefulWaitTime)
		engine.Close = listener.Close
		return s.ServeTLS(listener, certFile, keyFile)
	}
	return s.ServeTLS(ln, certFile, keyFile)
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe, ListenAndServeTLS and
// ListenAndServeTLSEmbed so dead TCP connections (e.g. closing laptop mid-download)
// eventually go away.
type tcpKeepaliveListener struct {
	*net.TCPListener
	keepalive       bool
	keepalivePeriod time.Duration
}

func (ln tcpKeepaliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	if err := tc.SetKeepAlive(ln.keepalive); err != nil {
		tc.Close() //nolint:errcheck
		return nil, err
	}
	if ln.keepalivePeriod > 0 {
		if err := tc.SetKeepAlivePeriod(ln.keepalivePeriod); err != nil {
			tc.Close() //nolint:errcheck
			return nil, err
		}
	}
	return tc, nil
}

// GracefulListener defines a listener that we can gracefully stop
type GracefulListener struct {
	// inner listener
	ln net.Listener

	// maximum wait time for graceful shutdown
	maxWaitTime time.Duration

	// this channel is closed during graceful shutdown on zero open connections.
	done chan struct{}

	// the number of open connections
	connsCount uint64

	// becomes non-zero when graceful shutdown starts
	shutdown uint64
}

// NewGracefulListener wraps the given listener into 'graceful shutdown' listener.
func NewGracefulListener(ln net.Listener, maxWaitTime time.Duration) net.Listener {
	return &GracefulListener{
		ln:          ln,
		maxWaitTime: maxWaitTime,
		done:        make(chan struct{}),
	}
}

// Accept creates a conn
func (ln *GracefulListener) Accept() (net.Conn, error) {
	c, err := ln.ln.Accept()

	if err != nil {
		return nil, err
	}

	atomic.AddUint64(&ln.connsCount, 1)
	return &gracefulConn{
		Conn: c,
		ln:   ln,
	}, nil
}

// Addr returns the listen address
func (ln *GracefulListener) Addr() net.Addr {
	return ln.ln.Addr()
}

// Close closes the inner listener and waits until all the pending
// open connections are closed before returning.
func (ln *GracefulListener) Close() (err error) {
	defer func() {
		ln.ln.Close()
	}()

	return ln.waitForZeroConns()
}

func (ln *GracefulListener) waitForZeroConns() error {
	atomic.AddUint64(&ln.shutdown, 1)

	if atomic.LoadUint64(&ln.connsCount) == 0 {
		close(ln.done)
		return nil
	}

	select {
	case <-ln.done:
		return nil
	case <-time.After(ln.maxWaitTime):
		return fmt.Errorf("cannot complete graceful shutdown in %s", ln.maxWaitTime)
	}
}

func (ln *GracefulListener) closeConn() {
	connsCount := atomic.AddUint64(&ln.connsCount, ^uint64(0))
	if atomic.LoadUint64(&ln.shutdown) != 0 && connsCount == 0 {
		close(ln.done)
	}
}

type gracefulConn struct {
	net.Conn
	ln *GracefulListener
}

func (c *gracefulConn) Close() error {
	err := c.Conn.Close()

	if err != nil {
		return err
	}

	c.ln.closeConn()

	return nil
}
