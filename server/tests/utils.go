package tests

import "net"

const bufferSize = 8192

// Connect to a socket using the provided address
func ConnectToSocket(address string) (net.Conn, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// Close the connection
func CloseConnection(conn net.Conn) {
	conn.Close()
}

// Send a message to the connection
func SendMessage(conn net.Conn, message string) error {
	_, err := conn.Write([]byte(message))
	if err != nil {
		return err
	}

	return nil
}

// Read a message from the connection
func ReadMessage(conn net.Conn) (string, error) {
	buffer := make([]byte, bufferSize)
	n, err := conn.Read(buffer)

	if err != nil {
		return "", err
	}

	return string(buffer[:n]), nil
}
