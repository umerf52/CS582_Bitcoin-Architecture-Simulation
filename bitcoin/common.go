package bitcoin

import (
	"net"
)

func MySend(conn net.Conn, marshalled []byte) error {
	_, err := conn.Write(marshalled)
	if err != nil {
		return err
	}
	return nil
}

func MyRead(conn net.Conn) ([]byte, error) {
	buffer := make([]byte, 1024)
	size, err := conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer[:size], nil
}
