package main

import (
	"log"
	"net"

	"monitoring-system/protocol"
)

func main() {

	conn, err := net.Dial("tcp", "localhost:2277")
	if err != nil {
		log.Fatal(err)
	}
	println("Connected to backend at localhost:2277")
	defer conn.Close()


	events := make(chan protocol.Event, 100)//buffer de eventos 
	go collectMetrics(events)//recolectar un evento
	go heartbeat(events)//recolectar un evento de latido
	go collectProcess(events)//recolectar un evento de procesos
	sender(conn, events)//mandar eventos indefinidamente
	}

func sender(conn net.Conn, events <-chan protocol.Event) {
    for event := range events {
        err := protocol.WriteEvent(conn, event)
        if err != nil {
            log.Println("error sending event:", err)
            return
        }
    }
}