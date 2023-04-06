package gws

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"github.com/hertz-contrib/websocket-netpoll/internal"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// NewClient 创建WebSocket客户端
func NewClient(handler Event, option *ClientOption) (client *Conn, resp *http.Response, e error) {
	var d = &dialer{
		eventHandler: handler,
		resp: &http.Response{
			Header: http.Header{},
		},
	}
	if option == nil {
		option = new(ClientOption)
	}
	option.initialize()

	URL, err := url.Parse(option.Addr)
	if err != nil {
		return nil, d.resp, err
	}

	var conn net.Conn
	var dialError error
	var hostname = URL.Hostname()
	var port = URL.Port()
	var host = ""
	switch URL.Scheme {
	case "ws":
		if port == "" {
			port = "80"
		}
		host = hostname + ":" + port
		conn, dialError = net.DialTimeout("tcp", host, option.DialTimeout)
	case "wss":
		if port == "" {
			port = "443"
		}
		host = hostname + ":" + port
		var tlsDialer = &net.Dialer{Timeout: option.DialTimeout}
		conn, dialError = tls.DialWithDialer(tlsDialer, "tcp", host, option.TlsConfig)
	default:
		return nil, d.resp, internal.ErrSchema
	}

	if dialError != nil {
		return nil, d.resp, dialError
	}
	if err := conn.SetDeadline(time.Now().Add(option.DialTimeout)); err != nil {
		return nil, d.resp, err
	}

	d.host = host
	d.option = option
	d.conn = conn
	return d.handshake()
}

type dialer struct {
	option       *ClientOption
	conn         net.Conn
	host         string
	eventHandler Event
	resp         *http.Response
}

func (c *dialer) stradd(ss ...string) string {
	var b []byte
	for _, item := range ss {
		b = append(b, item...)
	}
	return string(b)
}

// 生成报文
func (c *dialer) generateTelegram() []byte {
	if c.option.RequestHeader.Get(internal.SecWebSocketKey.Key) == "" {
		var key [16]byte
		binary.BigEndian.PutUint64(key[0:8], internal.AlphabetNumeric.Uint64())
		binary.BigEndian.PutUint64(key[8:16], internal.AlphabetNumeric.Uint64())
		c.option.RequestHeader.Set(internal.SecWebSocketKey.Key, base64.StdEncoding.EncodeToString(key[0:]))
	}
	if c.option.CompressEnabled {
		c.option.RequestHeader.Set(internal.SecWebSocketExtensions.Key, internal.SecWebSocketExtensions.Val)
	}

	var buf []byte
	buf = append(buf, c.stradd("GET ", c.option.Addr, " HTTP/1.1\r\n")...)
	buf = append(buf, c.stradd("Host: ", c.host, "\r\n")...)
	buf = append(buf, "Connection: Upgrade\r\n"...)
	buf = append(buf, "Upgrade: websocket\r\n"...)
	buf = append(buf, "Sec-WebSocket-Version: 13\r\n"...)
	for k, _ := range c.option.RequestHeader {
		buf = append(buf, c.stradd(k, ": ", c.option.RequestHeader.Get(k), "\r\n")...)
	}
	buf = append(buf, "\r\n"...)
	return buf
}

func (c *dialer) handshake() (*Conn, *http.Response, error) {
	brw := bufio.NewReadWriter(
		bufio.NewReaderSize(c.conn, c.option.ReadBufferSize),
		bufio.NewWriterSize(c.conn, c.option.WriteBufferSize),
	)
	telegram := c.generateTelegram()
	if err := internal.WriteN(brw.Writer, telegram, len(telegram)); err != nil {
		return nil, c.resp, err
	}
	if err := brw.Writer.Flush(); err != nil {
		return nil, c.resp, err
	}

	var ch = make(chan error)
	ctx, cancel := context.WithTimeout(context.Background(), c.option.DialTimeout)
	defer cancel()

	go func() {
		var index = 0
		for {
			line, isPrefix, err := brw.Reader.ReadLine()
			if err != nil {
				ch <- err
				return
			}
			if isPrefix {
				ch <- internal.ErrLongLine
				return
			}
			if index == 0 {
				arr := bytes.Split(line, []byte(" "))
				if len(arr) >= 2 {
					code, _ := strconv.Atoi(string(arr[1]))
					c.resp.StatusCode = code
				}
				if len(arr) != 4 || c.resp.StatusCode != 101 {
					ch <- internal.ErrStatusCode
					return
				}
			} else {
				if len(line) == 0 {
					ch <- nil
					return
				}
				arr := strings.Split(string(line), ": ")
				if len(arr) != 2 {
					ch <- internal.ErrHandshake
					return
				}
				c.resp.Header.Set(arr[0], arr[1])
			}
			index++
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, c.resp, internal.ErrDialTimeout
		case err := <-ch:
			if err != nil {
				return nil, c.resp, err
			}
			if err := c.checkHeaders(); err != nil {
				return nil, c.resp, err
			}
			var compressEnabled = false
			if c.option.CompressEnabled && strings.Contains(c.resp.Header.Get(internal.SecWebSocketExtensions.Key), "permessage-deflate") {
				compressEnabled = true
			}
			if err := internal.Errors(
				func() error { return c.conn.SetDeadline(time.Time{}) },
				func() error { return c.conn.SetReadDeadline(time.Time{}) },
				func() error { return c.conn.SetWriteDeadline(time.Time{}) },
				func() error { return setNoDelay(c.conn) }); err != nil {
				return nil, c.resp, err
			}
			ws := serveWebSocket(false, c.option.getConfig(), new(sliceMap), c.conn, c.eventHandler, compressEnabled)
			return ws, c.resp, nil
		}
	}
}

func (c *dialer) checkHeaders() error {
	if c.resp.Header.Get(internal.Connection.Key) != internal.Connection.Val {
		return internal.ErrHandshake
	}
	if c.resp.Header.Get(internal.Upgrade.Key) != internal.Upgrade.Val {
		return internal.ErrHandshake
	}
	var expectedKey = internal.ComputeAcceptKey(c.option.RequestHeader.Get(internal.SecWebSocketKey.Key))
	var actualKey = c.resp.Header.Get(internal.SecWebSocketAccept.Key)
	if actualKey != expectedKey {
		return internal.ErrHandshake
	}
	return nil
}
