package main

import (
	"fmt"
	"net"
	"monitoring-system/protocol"
)

func main() {
	listener, err := net.Listen("tcp", ":2277")
	if err != nil {
		panic(err)
	}

	defer listener.Close()

	fmt.Println("Backend listening on :2277")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}

		fmt.Println("new connection:", conn.RemoteAddr())

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()



	for {
			msg, err := protocol.ReadEvent(conn)
			if err != nil {
				fmt.Println("read message error:", err)
				return
			}
			fmt.Printf("received message: %+v\n", msg)

		}


	}
