package main

import (
	"log"
	"net"
	"time"

	"monitoring-system/protocol"

)

func main() {

	conn, err := net.Dial("tcp", "localhost:2277")
	if err != nil {
		log.Fatal(err)
	}

	defer conn.Close()

	for {
		cpuUsage, ramUsage, err := collectMetrics()
	if err != nil {
		log.Println(err)
		continue
		}

		msg := protocol.Message{
			Type:      "heartbeat",
			Hostname:  "agent-1",
			Timestamp: time.Now().Unix(),
			CPU:       cpuUsage,
			RAM:       ramUsage,
		}

		err = protocol.WriteMessage(conn, msg)
		if err != nil {
			log.Println(err)
			return
		}

		log.Println("heartbeat enviado")

		time.Sleep(5 * time.Second)
	}
}