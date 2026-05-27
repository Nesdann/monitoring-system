package main

import (
	"fmt"
	"net"
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

	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("read error:", err)
			return
		}

		fmt.Printf("received: %s\n", string(buffer[:n]))
	}
}