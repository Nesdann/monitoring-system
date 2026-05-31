package protocol

import (
	"encoding/binary"
	"encoding/json"
	"io"
)

func WriteEvent(w io.Writer, msg Event) error {
	//w io.writer es cualquier cosa donde pueda escribir, como un socket o un archivo

	payload, err := json.Marshal(msg)//lo pasa a json, lo convierte a bytes
	if err != nil {
		return err
	}

	length := uint32(len(payload))
	//le dice al receptor cuantos bytes va a recibir, para que sepa cuando termina el mensaje
	//protocolo de longitud-prefijada, primero se envía la longitud del mensaje y luego el mensaje en sí

	err = binary.Write(
		//escribimos en el buffer la longitud del mensaje, con un orden de bytes específico (big endian)
		w,
		binary.BigEndian,
		length,
	)

	if err != nil {
		return err
	}
	//luego escribimos el mensaje en sí
	_, err = w.Write(payload)
	if err != nil {
		return err
	}

	return nil
}

func ReadEvent(r io.Reader)(Event, error){
	var length uint32
	var msg Event
	//leer solo la longitud del mensaje
	header:= make([]byte, 4)
	_, err := io.ReadFull(r, header)//ler exactamente 4 bytes, que es el tamaño de uint32
	if err != nil {
		return msg, err
	}
	length = binary.BigEndian.Uint32(header)
	payload:= make([]byte, length)
	//crea un buffer del tamaño del mensaje, para leer el mensaje completo
	_, err = io.ReadFull(r, payload)
	if err != nil {
		return msg, err
	}


	err = json.Unmarshal(payload, &msg)//parsea el mnensaje
	if err != nil {
		return msg, err
	}

	return msg, nil

}