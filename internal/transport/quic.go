package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// quicListen starts a QUIC listener on addr and wraps it as a net.Listener.
// Each accepted QUIC connection exposes its first bidirectional stream as a
// net.Conn so the rest of VX6 (which speaks net.Conn) needs no changes.
func quicListen(addr string) (net.Listener, error) {
	tlsCfg, err := selfSignedTLS()
	if err != nil {
		return nil, err
	}
	ln, err := quic.ListenAddr(addr, tlsCfg, quicCfg())
	if err != nil {
		return nil, err
	}
	return &quicListener{ln: ln}, nil
}

// quicDial opens a QUIC connection to addr and returns its first stream as a net.Conn.
func quicDial(ctx context.Context, addr string) (net.Conn, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: true, // peer identity verified by VX6's secure/session layer above
		NextProtos:         []string{"vx6/1"},
		MinVersion:         tls.VersionTLS13,
	}
	conn, err := quic.DialAddr(ctx, addr, tlsCfg, quicCfg())
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "stream open failed")
		return nil, err
	}
	return &quicStreamConn{conn: conn, stream: stream}, nil
}

// quicCfg returns shared QUIC settings.
func quicCfg() *quic.Config {
	return &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}
}

// selfSignedTLS generates a throw-away TLS certificate for the QUIC listener.
// Real peer authentication happens in VX6's secure/session handshake on top.
func selfSignedTLS() (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "vx6-quic"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(87600 * time.Hour), // 10 years
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{{
			Certificate: [][]byte{der},
			PrivateKey:  key,
		}},
		NextProtos: []string{"vx6/1"},
		MinVersion: tls.VersionTLS13,
	}, nil
}

// ── quicListener wraps *quic.Listener as net.Listener ─────────────────────────

type quicListener struct{ ln *quic.Listener }

func (l *quicListener) Accept() (net.Conn, error) {
	conn, err := l.ln.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		_ = conn.CloseWithError(0, "stream accept failed")
		return nil, err
	}
	return &quicStreamConn{conn: conn, stream: stream}, nil
}

func (l *quicListener) Close() error   { return l.ln.Close() }
func (l *quicListener) Addr() net.Addr { return l.ln.Addr() }

// ── quicStreamConn wraps a QUIC stream as net.Conn ────────────────────────────

type quicStreamConn struct {
	conn   *quic.Conn   // *quic.Conn in v0.53+  (was quic.Connection interface before)
	stream *quic.Stream // *quic.Stream in v0.53+ (was quic.Stream interface before)
}

func (c *quicStreamConn) Read(b []byte) (int, error)  { return c.stream.Read(b) }
func (c *quicStreamConn) Write(b []byte) (int, error) { return c.stream.Write(b) }
func (c *quicStreamConn) Close() error {
	c.stream.CancelRead(0)
	return c.conn.CloseWithError(0, "closed")
}
func (c *quicStreamConn) LocalAddr() net.Addr             { return c.conn.LocalAddr() }
func (c *quicStreamConn) RemoteAddr() net.Addr            { return c.conn.RemoteAddr() }
func (c *quicStreamConn) SetDeadline(t time.Time) error      { return c.stream.SetDeadline(t) }
func (c *quicStreamConn) SetReadDeadline(t time.Time) error  { return c.stream.SetReadDeadline(t) }
func (c *quicStreamConn) SetWriteDeadline(t time.Time) error { return c.stream.SetWriteDeadline(t) }