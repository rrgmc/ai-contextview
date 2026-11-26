// Package appconn is the bridge's connection to the viewer daemon: dial, start
// the daemon when needed, hello, request/response, buffered ask results, and
// reconnect with replay of the last spec_show and the pending ask.
package appconn

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/rrgmc/ai-specview/internal/detach"
	"github.com/rrgmc/ai-specview/internal/protocol"
)

// ErrUnreachable is returned when the daemon cannot be reached.
var ErrUnreachable = errors.New("ai-specview viewer not reachable; ask your question in the chat instead")

// Options configure a Client.
type Options struct {
	Socket string
	// DaemonCmd starts the daemon when the socket is not reachable. Empty
	// means never start one.
	DaemonCmd []string
	Hello     protocol.Hello
	// ConnectTimeout bounds one connect attempt including daemon start.
	ConnectTimeout time.Duration
	Logf           func(format string, args ...any)
	// OnDocShown is called when the daemon made a written file the working
	// document (doc_shown). The client already replays it after a reconnect.
	OnDocShown func(protocol.DocShown)
}

// Client is the bridge side of one workspace connection.
type Client struct {
	opts Options

	mu           sync.Mutex
	conn         net.Conn
	connected    bool
	connecting   chan struct{}
	reconnecting bool
	closed       bool
	nextID       int
	waiters      map[string]chan protocol.Envelope
	results      map[string]*protocol.AskResult
	notify       map[string]chan struct{}
	url          string
	daemonVer    string
	lastSpecShow *protocol.SpecShow
	pendingAsk   *protocol.Ask
	lastDone     *protocol.Done
	lastAskMode  *protocol.AskMode

	writeMu sync.Mutex
}

// New creates a client; it does not connect until the first request.
func New(opts Options) *Client {
	if opts.ConnectTimeout == 0 {
		opts.ConnectTimeout = 10 * time.Second
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Client{
		opts:    opts,
		waiters: map[string]chan protocol.Envelope{},
		results: map[string]*protocol.AskResult{},
		notify:  map[string]chan struct{}{},
	}
}

// URL returns the workspace page URL, once connected.
func (c *Client) URL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.url
}

// DaemonVersion returns the daemon version from hello_ack.
func (c *Client) DaemonVersion() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.daemonVer
}

// Connected reports whether the socket is currently up.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// Ensure connects if needed, starting the daemon when the socket is down.
func (c *Client) Ensure(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("connection closed")
	}
	if c.connected {
		c.mu.Unlock()
		return nil
	}
	if ch := c.connecting; ch != nil {
		c.mu.Unlock()
		select {
		case <-ch:
			if c.Connected() {
				return nil
			}
			return ErrUnreachable
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ch := make(chan struct{})
	c.connecting = ch
	c.mu.Unlock()

	err := c.connect(ctx)

	c.mu.Lock()
	c.connecting = nil
	c.mu.Unlock()
	close(ch)
	if err != nil {
		c.opts.Logf("connect: %v", err)
		return ErrUnreachable
	}
	return nil
}

func (c *Client) dial() (net.Conn, error) {
	return net.DialTimeout("unix", c.opts.Socket, time.Second)
}

func (c *Client) startDaemon() {
	if len(c.opts.DaemonCmd) == 0 {
		return
	}
	cmd := exec.Command(c.opts.DaemonCmd[0], c.opts.DaemonCmd[1:]...)
	cmd.Env = append(os.Environ(), "AI_SPECVIEW_SOCKET="+c.opts.Socket)
	cmd.SysProcAttr = detach.SysProcAttr()
	if err := cmd.Start(); err != nil {
		c.opts.Logf("start daemon %v: %v", c.opts.DaemonCmd, err)
		return
	}
	c.opts.Logf("started daemon %v (pid %d)", c.opts.DaemonCmd, cmd.Process.Pid)
	go func() { _ = cmd.Wait() }()
}

// connect dials (starting the daemon if needed), sends hello and replays
// state.
func (c *Client) connect(ctx context.Context) error {
	deadline := time.Now().Add(c.opts.ConnectTimeout)
	conn, err := c.dial()
	if err != nil {
		c.startDaemon()
		backoff := 100 * time.Millisecond
		for {
			if time.Now().After(deadline) {
				return fmt.Errorf("dial %s: %w", c.opts.Socket, err)
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			if backoff < 2*time.Second {
				backoff *= 2
			}
			if conn, err = c.dial(); err == nil {
				break
			}
		}
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()
	go c.readLoop(conn)

	hctx, cancel := context.WithDeadline(ctx, deadline.Add(5*time.Second))
	defer cancel()
	hello := c.opts.Hello
	hello.Protocol = protocol.Protocol
	reply, err := c.roundTrip(hctx, protocol.TypeHello, &hello)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("hello: %w", err)
	}
	var ack protocol.HelloAck
	if err := reply.Decode(&ack); err != nil {
		_ = conn.Close()
		return err
	}
	c.mu.Lock()
	c.url = ack.URL
	c.daemonVer = ack.DaemonVersion
	if hello.BridgeVersion != "" && ack.DaemonVersion != hello.BridgeVersion {
		c.opts.Logf("daemon is version %s, bridge is %s; run `ai-specview serve --restart` to update the daemon", ack.DaemonVersion, hello.BridgeVersion)
	}
	spec := c.lastSpecShow
	ask := c.pendingAsk
	done := c.lastDone
	mode := c.lastAskMode
	c.mu.Unlock()

	// Replay: spec_show, then the pending ask. Replies are not needed.
	if spec != nil {
		if _, err := c.roundTrip(hctx, protocol.TypeSpecShow, spec); err != nil {
			c.opts.Logf("replay spec_show: %v", err)
		}
	}
	if mode != nil {
		if _, err := c.roundTrip(hctx, protocol.TypeAskMode, mode); err != nil {
			c.opts.Logf("replay ask_mode: %v", err)
		}
	}
	if ask != nil {
		if _, err := c.roundTrip(hctx, protocol.TypeAsk, ask); err != nil {
			c.opts.Logf("replay ask: %v", err)
		}
	}
	if done != nil {
		if _, err := c.roundTrip(hctx, protocol.TypeDone, done); err != nil {
			c.opts.Logf("replay done: %v", err)
		}
	}
	return nil
}

func (c *Client) readLoop(conn net.Conn) {
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), protocol.MaxLine)
	for sc.Scan() {
		var env protocol.Envelope
		if err := json.Unmarshal(sc.Bytes(), &env); err != nil {
			c.opts.Logf("bad message: %v", err)
			continue
		}
		c.dispatch(env)
	}
	c.mu.Lock()
	if err := sc.Err(); err != nil && !c.closed {
		c.opts.Logf("read: %v", err)
	}
	if c.conn == conn {
		c.connected = false
		c.conn = nil
	}
	for id, w := range c.waiters {
		delete(c.waiters, id)
		close(w)
	}
	closed := c.closed
	start := !closed && !c.reconnecting
	if start {
		c.reconnecting = true
	}
	c.mu.Unlock()
	_ = conn.Close()
	if start {
		go c.reconnectLoop()
	}
}

func (c *Client) dispatch(env protocol.Envelope) {
	if env.Type == protocol.TypeDocShown {
		var d protocol.DocShown
		if err := env.Decode(&d); err != nil || d.Validate() != nil {
			return
		}
		c.mu.Lock()
		c.lastSpecShow = &protocol.SpecShow{RootAbs: d.RootAbs, Auto: true}
		c.mu.Unlock()
		if c.opts.OnDocShown != nil {
			c.opts.OnDocShown(d)
		}
		return
	}
	if env.Type == protocol.TypeAskResult {
		var r protocol.AskResult
		if err := env.Decode(&r); err != nil {
			return
		}
		c.mu.Lock()
		c.results[r.QSID] = &r
		if ch, ok := c.notify[r.QSID]; ok {
			close(ch)
			delete(c.notify, r.QSID)
		}
		if c.pendingAsk != nil && c.pendingAsk.QSID == r.QSID {
			c.pendingAsk = nil
		}
		c.mu.Unlock()
		return
	}
	if env.Re == "" {
		return
	}
	c.mu.Lock()
	w, ok := c.waiters[env.Re]
	if ok {
		delete(c.waiters, env.Re)
	}
	c.mu.Unlock()
	if ok {
		w <- env
		close(w)
	}
}

func (c *Client) reconnectLoop() {
	defer func() {
		c.mu.Lock()
		c.reconnecting = false
		c.mu.Unlock()
	}()
	backoff := 100 * time.Millisecond
	for {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return
		}
		if err := c.Ensure(context.Background()); err == nil {
			c.opts.Logf("reconnected")
			return
		}
		time.Sleep(backoff)
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) send(env protocol.Envelope) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return ErrUnreachable
	}
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = conn.Write(b)
	return err
}

// roundTrip sends a message and waits for the reply with the same id.
func (c *Client) roundTrip(ctx context.Context, typ string, body any) (protocol.Envelope, error) {
	c.mu.Lock()
	c.nextID++
	id := "m_" + strconv.Itoa(c.nextID)
	w := make(chan protocol.Envelope, 1)
	c.waiters[id] = w
	c.mu.Unlock()

	env, err := protocol.NewEnvelope(typ, id, "", c.opts.Hello.WS, body)
	if err != nil {
		return env, err
	}
	if err := c.send(env); err != nil {
		c.mu.Lock()
		delete(c.waiters, id)
		c.mu.Unlock()
		return env, err
	}
	select {
	case reply, ok := <-w:
		if !ok {
			return reply, ErrUnreachable
		}
		if reply.Type == protocol.TypeError {
			var e protocol.ErrorBody
			if err := reply.Decode(&e); err != nil {
				return reply, err
			}
			return reply, &e
		}
		return reply, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.waiters, id)
		c.mu.Unlock()
		return protocol.Envelope{}, ctx.Err()
	}
}

// Request connects if needed and sends one request, returning its reply.
// spec_show, ask, done and ask_mode are remembered for replay after a
// reconnect.
func (c *Client) Request(ctx context.Context, typ string, body any) (protocol.Envelope, error) {
	switch b := body.(type) {
	case *protocol.SpecShow:
		c.mu.Lock()
		c.lastSpecShow = b
		c.mu.Unlock()
	case *protocol.Ask:
		c.mu.Lock()
		c.pendingAsk = b
		c.lastDone = nil
		c.mu.Unlock()
	case *protocol.Done:
		c.mu.Lock()
		c.lastDone = b
		c.mu.Unlock()
	case *protocol.AskMode:
		c.mu.Lock()
		c.lastAskMode = b
		c.mu.Unlock()
	case *protocol.AskCancel:
		c.mu.Lock()
		if c.pendingAsk != nil && c.pendingAsk.QSID == b.QSID {
			c.pendingAsk = nil
		}
		c.mu.Unlock()
	}
	if err := c.Ensure(ctx); err != nil {
		return protocol.Envelope{}, err
	}
	return c.roundTrip(ctx, typ, body)
}

// WaitResult waits up to d for the ask_result of qs. It returns (nil, nil) on
// timeout. A result that arrived earlier is returned at once.
func (c *Client) WaitResult(ctx context.Context, qs string, d time.Duration) (*protocol.AskResult, error) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		c.mu.Lock()
		if r, ok := c.results[qs]; ok {
			delete(c.results, qs)
			c.mu.Unlock()
			return r, nil
		}
		ch, ok := c.notify[qs]
		if !ok {
			ch = make(chan struct{})
			c.notify[qs] = ch
		}
		c.mu.Unlock()
		select {
		case <-ch:
		case <-timer.C:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// HasResult reports whether a result for qs is buffered.
func (c *Client) HasResult(qs string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.results[qs]
	return ok
}

// ClearAutoSpec forgets the last spec_show when it replays a document the
// daemon picked (doc_shown), so a reconnect does not show it again.
func (c *Client) ClearAutoSpec() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastSpecShow != nil && c.lastSpecShow.Auto {
		c.lastSpecShow = nil
	}
}

// Close sends bye and closes the connection without reconnecting.
func (c *Client) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		if env, err := protocol.NewEnvelope(protocol.TypeBye, "bye", "", c.opts.Hello.WS, protocol.Bye{}); err == nil {
			b, _ := json.Marshal(env)
			c.writeMu.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
			_, _ = conn.Write(append(b, '\n'))
			c.writeMu.Unlock()
		}
		_ = conn.Close()
	}
}
