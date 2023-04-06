package gws

import (
	"bytes"
	"context"
	"errors"
	"github.com/cloudwego/netpoll"
	"github.com/hertz-contrib/websocket-netpoll/internal"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type Upgrader struct {
	option       *ServerOption
	eventHandler Event
}

func NewUpgrader(eventHandler Event, option *ServerOption) *Upgrader {
	if option == nil {
		option = new(ServerOption)
	}
	return &Upgrader{
		option:       option.initialize(),
		eventHandler: eventHandler,
	}
}

func (c *Upgrader) connectHandshake(r *http.Request, responseHeader http.Header, conn net.Conn, websocketKey string) error {
	if r.Header.Get(internal.SecWebSocketProtocol.Key) != "" {
		var subprotocolsUsed = ""
		var arr = internal.Split(r.Header.Get(internal.SecWebSocketProtocol.Key), ",")
		for _, item := range arr {
			if internal.InCollection(item, c.option.Subprotocols) {
				subprotocolsUsed = item
				break
			}
		}
		if subprotocolsUsed != "" {
			responseHeader.Set(internal.SecWebSocketProtocol.Key, subprotocolsUsed)
		}
	}

	var buf = make([]byte, 0, 256)
	buf = append(buf, "HTTP/1.1 101 Switching Protocols\r\n"...)
	buf = append(buf, "Upgrade: websocket\r\n"...)
	buf = append(buf, "Connection: Upgrade\r\n"...)
	buf = append(buf, "Sec-WebSocket-Accept: "...)
	buf = append(buf, internal.ComputeAcceptKey(websocketKey)...)
	buf = append(buf, "\r\n"...)
	for k, _ := range responseHeader {
		buf = append(buf, k...)
		buf = append(buf, ": "...)
		buf = append(buf, responseHeader.Get(k)...)
		buf = append(buf, "\r\n"...)
	}
	buf = append(buf, "\r\n"...)
	_, err := conn.Write(buf)
	return err
}

// Accept http upgrade to websocket protocol
func (c *Upgrader) accept(r *http.Request, conn netpoll.Connection) (*Conn, error) {
	socket, err := c.doAccept(r, conn)
	if err != nil {
		if socket != nil && socket.conn != nil {
			_ = socket.conn.Close()
		}
		return nil, err
	}
	return socket, err
}

func (c *Upgrader) doAccept(r *http.Request, conn netpoll.Connection) (*Conn, error) {
	var session = new(sliceMap)
	var header = c.option.ResponseHeader.Clone()
	if !c.option.CheckOrigin(r, session) {
		return nil, internal.ErrCheckOrigin
	}

	var compressEnabled = false
	if r.Method != http.MethodGet {
		return nil, internal.ErrGetMethodRequired
	}
	if version := r.Header.Get(internal.SecWebSocketVersion.Key); version != internal.SecWebSocketVersion.Val {
		msg := "websocket protocol not supported: " + version
		return nil, errors.New(msg)
	}
	if val := r.Header.Get(internal.Connection.Key); strings.ToLower(val) != strings.ToLower(internal.Connection.Val) {
		return nil, internal.ErrHandshake
	}
	if val := r.Header.Get(internal.Upgrade.Key); strings.ToLower(val) != internal.Upgrade.Val {
		return nil, internal.ErrHandshake
	}
	if val := r.Header.Get(internal.SecWebSocketExtensions.Key); strings.Contains(val, "permessage-deflate") && c.option.CompressEnabled {
		header.Set(internal.SecWebSocketExtensions.Key, internal.SecWebSocketExtensions.Val)
		compressEnabled = true
	}
	var websocketKey = r.Header.Get(internal.SecWebSocketKey.Key)
	if websocketKey == "" {
		return nil, internal.ErrHandshake
	}

	if err := c.connectHandshake(r, header, conn, websocketKey); err != nil {
		return &Conn{conn: conn}, err
	}

	//if err := internal.Errors(
	//	func() error { return netConn.SetDeadline(time.Time{}) },
	//	func() error { return netConn.SetReadDeadline(time.Time{}) },
	//	func() error { return netConn.SetWriteDeadline(time.Time{}) },
	//	func() error { return setNoDelay(netConn) }); err != nil {
	//	return nil, err
	//}
	ws := serveWebSocket(true, c.option.getConfig(), session, conn, c.eventHandler, compressEnabled)
	return ws, nil
}

//type httpWriter struct {
//	Conn netpoll.Connection
//	BRW  *bufio.ReadWriter
//}
//
//func (c *httpWriter) Header() http.Header { return http.Header{} }
//
//func (c *httpWriter) Write(b []byte) (int, error) { return c.Conn.Write(b) }
//
//func (c *httpWriter) WriteHeader(statusCode int) {}
//
//func (c *httpWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
//	return c.Conn, c.BRW, nil
//}

//func (c *Upgrader) Upgrade(ctx context.Context, conn netpoll.Connection) error {
//	reader := bufio.NewReaderSize(conn, 4*1024)
//	writer := bufio.NewWriterSize(conn, 4*1024)
//	index := 0
//	request := &http.Request{Header: http.Header{}}
//	for {
//		index++
//		line, _, err := reader.ReadLine()
//		//line, err := conn.Reader().Until('\n')
//		if err != nil {
//			return err
//		}
//
//		if len(line) == 0 {
//			break
//		}
//
//		if index == 1 {
//			arr := bytes.Split(line, []byte(" "))
//			if len(arr) == 3 {
//				request.Method = string(arr[0])
//				URL, err := url.Parse(string(arr[1]))
//				if err != nil {
//					return err
//				}
//				request.URL = URL
//			} else {
//				return internal.ErrHandshake
//			}
//			continue
//		}
//
//		arr := strings.Split(string(line), ": ")
//		if len(arr) != 2 {
//			return internal.ErrHandshake
//		}
//		request.Header.Set(arr[0], arr[1])
//	}
//
//	hw := &httpWriter{Conn: conn, BRW: bufio.NewReadWriter(reader, writer)}
//	socket, err := c.Accept(hw, request)
//	if err != nil {
//		return err
//	}
//
//	c.eventHandler.OnOpen(socket)
//
//	conn.AddCloseCallback(func(connection netpoll.Connection) error {
//		socket.emitError(internal.ErrConnClosed)
//		return nil
//	})
//
//	return conn.SetOnRequest(func(ctx context.Context, connection netpoll.Connection) error {
//		for {
//			n := conn.Reader().Len()
//			if n == 0 && socket.rbuf.Buffered() == 0 {
//				break
//			}
//			err := socket.readMessage()
//			if err != nil {
//				_ = conn.Close()
//				socket.emitError(err)
//				return err
//			}
//		}
//		return nil
//	})
//}

func (c *Upgrader) OnRequest(ctx context.Context, conn netpoll.Connection) error {
	//reader := bufio.NewReaderSize(conn, 4*1024)
	//writer := bufio.NewWriterSize(conn, 4*1024)
	index := 0
	request := &http.Request{Header: http.Header{}}
	for {
		index++
		line, err := conn.Reader().Until('\n')
		if err != nil {
			return err
		}

		n := len(line)
		if n > 0 && line[n-1] == '\n' {
			line = line[:n-1]
			n--
		}
		if n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
			n--
		}
		if len(line) == 0 {
			break
		}

		if index == 1 {
			arr := bytes.Split(line, []byte(" "))
			if len(arr) == 3 {
				request.Method = string(arr[0])
				URL, err := url.Parse(string(arr[1]))
				if err != nil {
					return err
				}
				request.URL = URL
			} else {
				return internal.ErrHandshake
			}
			continue
		}

		arr := strings.Split(string(line), ": ")
		if len(arr) != 2 {
			return internal.ErrHandshake
		}
		request.Header.Set(arr[0], arr[1])
	}

	socket, err := c.accept(request, conn)
	if err != nil {
		return err
	}

	if err := conn.AddCloseCallback(func(connection netpoll.Connection) error {
		socket.emitError(internal.ErrConnClosed)
		return nil
	}); err != nil {
		return err
	}

	c.eventHandler.OnOpen(socket)

	return conn.SetOnRequest(func(ctx context.Context, connection netpoll.Connection) error {
		err := socket.readMessage()
		if err != nil {
			_ = conn.Close()
			socket.emitError(err)
		}
		return err
	})
}
