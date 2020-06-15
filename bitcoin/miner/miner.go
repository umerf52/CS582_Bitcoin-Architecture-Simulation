package main

import (
	"bitcoin"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
)

// Attempt to connect miner as a client to the server.
func joinWithServer(hostport string) (net.Conn, error) {
	// TODO: implement this!
	miner, err := net.Dial("tcp", hostport)
	if err != nil {
		LOGF.Println("Failed to connect to server:", err)
		return nil, err
	}

	msg := bitcoin.NewJoin()
	marshalled, err := json.Marshal(*msg)
	err = bitcoin.MySend(miner, marshalled)
	if err != nil {
		LOGF.Println(err)
		return nil, err
	}
	return miner, nil
}

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

	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Printf("Usage: ./%s <hostport>", os.Args[0])
		return
	}

	hostport := os.Args[1]
	miner, err := joinWithServer(hostport)
	if err != nil {
		LOGF.Println("Failed to join with server:", err)
		return
	}

	defer miner.Close()

	// TODO: implement this!
	for {
		buffer, err := bitcoin.MyRead(miner)
		if err != nil {
			LOGF.Println(err)
		}
		request := new(bitcoin.Message)
		_ = json.Unmarshal(buffer, request)

		var minNonce uint64 = 0
		minHash := bitcoin.Hash(request.Data, 0)
		for i := request.Lower + 1; i <= request.Upper; i++ {
			tempHash := bitcoin.Hash(request.Data, i)
			if tempHash < minHash {
				minHash = tempHash
				minNonce = i
			}
		}

		msg := bitcoin.NewResult(minHash, minNonce)
		marshalled, err := json.Marshal(*msg)
		err = bitcoin.MySend(miner, marshalled)
		if err != nil {
			LOGF.Println(err)
		}
	}
}
