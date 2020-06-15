package main

import (
	"bitcoin"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
)

var LOGF *log.Logger

func main() {
	const (
		name = "log.txt"
		flag = os.O_RDWR | os.O_CREATE
		perm = os.FileMode(0666)
	)

	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return
	}
	defer file.Close()

	LOGF = log.New(file, "", log.Lshortfile|log.Lmicroseconds)

	const numArgs = 4
	if len(os.Args) != numArgs {
		fmt.Printf("Usage: ./%s <hostport> <message> <maxNonce>", os.Args[0])
		return
	}
	hostport := os.Args[1]
	message := os.Args[2]
	maxNonce, err := strconv.ParseUint(os.Args[3], 10, 64)
	if err != nil {
		fmt.Printf("%s is not a number.\n", os.Args[3])
		return
	}

	client, err := net.Dial("tcp", hostport)
	if err != nil {
		fmt.Println("Failed to connect to server:", err)
		return
	}

	defer client.Close()

	// TODO: implement this!
	msg := bitcoin.NewRequest(message, 0, maxNonce)
	marshalled, _ := json.Marshal(msg)
	err = bitcoin.MySend(client, marshalled)
	if err != nil {
		LOGF.Println(err)
	}

	buffer, err := bitcoin.MyRead(client)
	if err != nil {
		printDisconnected()
		return
	}

	unmarshalled := new(bitcoin.Message)
	_ = json.Unmarshal(buffer, unmarshalled)

	printResult(unmarshalled.Hash, unmarshalled.Nonce)
}

// printResult prints the final result to stdout.
func printResult(hash, nonce uint64) {
	fmt.Println("Result", hash, nonce)
}

// printDisconnected prints a disconnected message to stdout.
func printDisconnected() {
	fmt.Println("Disconnected")
}
