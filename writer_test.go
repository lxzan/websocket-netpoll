package websocket_netpoll

import (
	"bytes"
	"github.com/hertz-contrib/websocket-netpoll/internal"
	"github.com/stretchr/testify/assert"
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
	if err := internal.WriteN(c.conn, header[:headerLength], headerLength); err != nil {
		return err
	}
	if err := internal.WriteN(c.conn, payload, n); err != nil {
		return err
	}
	return nil
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
