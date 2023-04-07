package main

import (
	"fmt"
	websocket "github.com/hertz-contrib/websocket-netpoll"
	"log"
	"net/http"
)

func main() {
	socket, _, err := websocket.NewClient(new(WebSocket), &websocket.ClientOption{
		Addr: "ws://82.157.123.54:9010/ajaxchattest",
		RequestHeader: http.Header{
			"Origin": []string{"http://coolaf.com"},
		},
	})
	if err != nil {
		log.Printf(err.Error())
	}

	for {
		var msg string
		if _, err := fmt.Scanf("%s", &msg); err != nil {
			return
		}
		socket.WriteString(msg)
	}
}

type WebSocket struct {
}

func (c *WebSocket) OnOpen(socket *websocket.Conn) {

}

func (c *WebSocket) OnError(socket *websocket.Conn, err error) {

}

func (c *WebSocket) OnClose(socket *websocket.Conn, code uint16, reason []byte) {

}

func (c *WebSocket) OnPing(socket *websocket.Conn, payload []byte) {

}

func (c *WebSocket) OnPong(socket *websocket.Conn, payload []byte) {

}

func (c *WebSocket) OnMessage(socket *websocket.Conn, message *websocket.Message) {
	fmt.Printf("recv: %s\n", message.Data.String())
}
