package gws

import (
	"bufio"
	"bytes"
	"github.com/hertz-contrib/websocket-netpoll/internal"
	"github.com/stretchr/testify/assert"
	"net"
	"sync"
	"testing"
)

func testWrite(c *Conn, fin bool, opcode Opcode, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	var useCompress = c.compressEnabled && opcode.IsDataFrame() && len(payload) >= c.config.CompressThreshold
	if useCompress {
		compressedContent, err := c.compressor.Compress(bytes.NewBuffer(payload))
		if err != nil {
			return internal.NewError(internal.CloseInternalServerErr, err)
		}
		payload = compressedContent.Bytes()
	}
	if len(payload) > c.config.WriteMaxPayloadSize {
		return internal.CloseMessageTooLarge
	}

	var header = frameHeader{}
	var n = len(payload)
	headerLength, maskBytes := header.GenerateHeader(c.isServer, fin, useCompress, opcode, n)
	if !c.isServer {
		internal.MaskXOR(payload, maskBytes)
	}
	if err := internal.WriteN(c.wbuf, header[:headerLength], headerLength); err != nil {
		return err
	}
	if err := internal.WriteN(c.wbuf, payload, n); err != nil {
		return err
	}
	return c.wbuf.Flush()
}

func TestConn_WriteMessage(t *testing.T) {
	var as = assert.New(t)
	var handler = new(webSocketMocker)
	var upgrader = NewUpgrader(handler, nil)
	var writer = bytes.NewBuffer(nil)
	var reader = bytes.NewBuffer(nil)
	var brw = bufio.NewReadWriter(bufio.NewReader(reader), bufio.NewWriter(writer))
	conn, _ := net.Pipe()
	var socket = serveWebSocket(true, upgrader.option.getConfig(), new(sliceMap), conn, brw, handler, false)

	t.Run("text v1", func(t *testing.T) {
		handler.reset(socket, reader, writer)
		socket.WriteString("hello")
		var p = make([]byte, 7)
		_, _ = writer.Read(p)
		as.Equal("hello", string(p[2:]))
		var fh = frameHeader{}
		copy(fh[0:], p[:2])
		as.Equal(OpcodeText, fh.GetOpcode())
		as.Equal(true, fh.GetFIN())
		as.Equal(false, fh.GetRSV1())
		as.Equal(false, fh.GetRSV2())
		as.Equal(false, fh.GetRSV3())
		as.Equal(false, fh.GetMask())
		as.Equal(uint8(5), fh.GetLengthCode())
	})

	t.Run("binary v2", func(t *testing.T) {
		handler.reset(socket, reader, writer)
		var contentLength = 500
		var text = internal.AlphabetNumeric.Generate(contentLength)
		socket.WriteMessage(OpcodeBinary, text)
		var p = make([]byte, contentLength+4)
		_, _ = writer.Read(p)
		as.Equal(string(text), string(p[4:]))
		var fh = frameHeader{}
		copy(fh[0:], p[:2])
		as.Equal(OpcodeBinary, fh.GetOpcode())
		as.Equal(true, fh.GetFIN())
		as.Equal(false, fh.GetRSV1())
		as.Equal(false, fh.GetMask())
		as.Equal(uint8(126), fh.GetLengthCode())
	})

	t.Run("ping", func(t *testing.T) {
		handler.reset(socket, reader, writer)
		socket.WritePing([]byte("ping"))
		var p = make([]byte, 6)
		_, _ = writer.Read(p)
		as.Equal("ping", string(p[2:]))
		var fh = frameHeader{}
		copy(fh[0:], p[:2])
		as.Equal(OpcodePing, fh.GetOpcode())
		as.Equal(true, fh.GetFIN())
		as.Equal(false, fh.GetRSV1())
		as.Equal(false, fh.GetMask())
		as.Equal(uint8(4), fh.GetLengthCode())
	})

	t.Run("pong", func(t *testing.T) {
		handler.reset(socket, reader, writer)
		socket.WritePong(nil)
		var p = make([]byte, 6)
		_, _ = writer.Read(p)
		var fh = frameHeader{}
		copy(fh[0:], p[:2])
		as.Equal(OpcodePong, fh.GetOpcode())
		as.Equal(true, fh.GetFIN())
		as.Equal(false, fh.GetRSV1())
		as.Equal(false, fh.GetMask())
		as.Equal(uint8(0), fh.GetLengthCode())
	})

	t.Run("close", func(t *testing.T) {
		socket.closed = 1
		writer.Reset()
		socket.WriteMessage(OpcodeText, internal.AlphabetNumeric.Generate(500))
		as.Equal(0, writer.Len())
	})
}

func TestConn_WriteMessageCompress(t *testing.T) {
	var as = assert.New(t)
	var handler = new(webSocketMocker)
	var upgrader = NewUpgrader(handler, &ServerOption{
		CheckUtf8Enabled: true,
	})
	var writer = bytes.NewBuffer(nil)
	var reader = bytes.NewBuffer(nil)
	var brw = bufio.NewReadWriter(bufio.NewReader(reader), bufio.NewWriter(writer))
	conn, _ := net.Pipe()
	var socket = serveWebSocket(true, upgrader.option.getConfig(), new(sliceMap), conn, brw, handler, true)

	// 消息长度低于阈值, 不压缩内容
	t.Run("text v1", func(t *testing.T) {
		handler.reset(socket, reader, writer)
		var n = 64
		var text = internal.AlphabetNumeric.Generate(n)
		socket.WriteMessage(OpcodeText, text)
		var compressedLength = writer.Len() - 2

		var p = make([]byte, 2)
		_, _ = writer.Read(p)
		as.Equal(string(text), writer.String())
		var fh = frameHeader{}
		copy(fh[0:], p[:2])
		as.Equal(OpcodeText, fh.GetOpcode())
		as.Equal(true, fh.GetFIN())
		as.Equal(false, fh.GetRSV1())
		as.Equal(false, fh.GetRSV2())
		as.Equal(false, fh.GetRSV3())
		as.Equal(false, fh.GetMask())
		as.Equal(uint8(compressedLength), fh.GetLengthCode())
	})

	t.Run("text v2", func(t *testing.T) {
		handler.reset(socket, reader, writer)
		var n = 1024
		var text = internal.AlphabetNumeric.Generate(n)
		socket.WriteMessage(OpcodeText, text)

		buffer, err := socket.decompressor.Decompress(bytes.NewBuffer(writer.Bytes()[4:]))
		if err != nil {
			as.NoError(err)
			return
		}

		as.Equal(string(text), string(buffer.Bytes()))
		var p = make([]byte, 4)
		_, _ = writer.Read(p)
		var fh = frameHeader{}
		copy(fh[0:], p)
		as.Equal(OpcodeText, fh.GetOpcode())
		as.Equal(true, fh.GetFIN())
		as.Equal(true, fh.GetRSV1())
		as.Equal(false, fh.GetRSV2())
		as.Equal(false, fh.GetRSV3())
		as.Equal(false, fh.GetMask())
		as.Equal(uint8(126), fh.GetLengthCode())
	})

	t.Run("write to closed socket", func(t *testing.T) {
		handler.reset(socket, reader, writer)
		var n = 1024
		var text = internal.AlphabetNumeric.Generate(n)
		socket.closed = 1
		as.Equal(internal.ErrConnClosed, socket.WriteMessage(OpcodeText, text))
	})
}

func TestWriteBigMessage(t *testing.T) {
	var serverHandler = new(webSocketMocker)
	var clientHandler = new(webSocketMocker)
	var serverOption = &ServerOption{WriteMaxPayloadSize: 16}
	var clientOption = &ClientOption{}
	server, client := newPeer(serverHandler, serverOption, clientHandler, clientOption)
	go server.Listen()
	go client.Listen()
	var err = server.WriteMessage(OpcodeText, internal.AlphabetNumeric.Generate(128))
	assert.Error(t, err)
}

func TestWriteClose(t *testing.T) {
	var as = assert.New(t)
	var serverHandler = new(webSocketMocker)
	var clientHandler = new(webSocketMocker)
	var serverOption = &ServerOption{}
	var clientOption = &ClientOption{}

	var wg = sync.WaitGroup{}
	wg.Add(1)
	serverHandler.onError = func(socket *Conn, err error) {
		as.Error(err)
		wg.Done()
	}
	server, client := newPeer(serverHandler, serverOption, clientHandler, clientOption)
	go server.Listen()
	go client.Listen()
	server.WriteClose(1000, []byte("goodbye"))
	wg.Wait()
}

func TestConn_WriteAsyncError(t *testing.T) {
	var as = assert.New(t)

	t.Run("", func(t *testing.T) {
		var serverHandler = new(webSocketMocker)
		var clientHandler = new(webSocketMocker)
		var serverOption = &ServerOption{WriteAsyncCap: 1}
		var clientOption = &ClientOption{}
		server, _ := newPeer(serverHandler, serverOption, clientHandler, clientOption)
		server.WriteAsync(OpcodeText, nil)
		server.WriteAsync(OpcodeText, nil)
		err := server.WriteAsync(OpcodeText, nil)
		as.Equal(internal.ErrAsyncIOCapFull, err)
	})

	t.Run("", func(t *testing.T) {
		var serverHandler = new(webSocketMocker)
		var clientHandler = new(webSocketMocker)
		var serverOption = &ServerOption{}
		var clientOption = &ClientOption{}
		server, _ := newPeer(serverHandler, serverOption, clientHandler, clientOption)
		server.closed = 1
		err := server.WriteAsync(OpcodeText, nil)
		as.Equal(internal.ErrConnClosed, err)
	})
}
