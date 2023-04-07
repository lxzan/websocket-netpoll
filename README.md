# websocket-netpoll

### Quick Start

#### Server 
```go
package main

import (
	"github.com/cloudwego/netpoll"
	websocket "github.com/hertz-contrib/websocket-netpoll"
	"log"
	"net"
)

var upgrader = websocket.NewUpgrader(new(WebSocket), &websocket.ServerOption{})

func main() {
	listener, err := net.Listen("tcp", ":3000")
	if err != nil {
		log.Panicln(err.Error())
		return
	}
	eventLoop, err := netpoll.NewEventLoop(upgrader.OnRequest)
	if err != nil {
		log.Println(err.Error())
		return
	}
	if err := eventLoop.Serve(listener); err != nil {
		log.Println(err.Error())
		return
	}
}

type WebSocket struct {}

func (c *WebSocket) OnOpen(socket *websocket.Conn) {}

func (c *WebSocket) OnError(socket *websocket.Conn, err error) {}

func (c *WebSocket) OnClose(socket *websocket.Conn, code uint16, reason []byte) {}

func (c *WebSocket) OnPing(socket *websocket.Conn, payload []byte) {
	_ = socket.WritePong(payload)
}

func (c *WebSocket) OnPong(socket *websocket.Conn, payload []byte) {}

func (c *WebSocket) OnMessage(socket *websocket.Conn, message *websocket.Message) {
	defer message.Close()
	_ = socket.WriteMessage(message.Opcode, message.Bytes())
}

```

#### Client
```go
package main

import (
	"fmt"
	websocket "github.com/hertz-contrib/websocket-netpoll"
	"log"
	"net/http"
)

func main() {
	socket, _, err := websocket.NewClient(new(WebSocket), &websocket.ClientOption{
		Addr: "ws://127.0.0.1:3000",
	})
	if err != nil {
		log.Printf(err.Error())
	}
}

type WebSocket struct{}

func (c *WebSocket) OnOpen(socket *websocket.Conn) {}

func (c *WebSocket) OnError(socket *websocket.Conn, err error) {}

func (c *WebSocket) OnClose(socket *websocket.Conn, code uint16, reason []byte) {}

func (c *WebSocket) OnPing(socket *websocket.Conn, payload []byte) {}

func (c *WebSocket) OnPong(socket *websocket.Conn, payload []byte) {}

func (c *WebSocket) OnMessage(socket *websocket.Conn, message *websocket.Message) {}

```