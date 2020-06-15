package main

import (
	"bitcoin"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strconv"
)

type clientChunkStruct struct {
	client    net.Conn
	chunkSize uint64
}

type nonceHashStruct struct {
	nonce uint64
	hash  uint64
}

type subJob struct {
	j        *job
	newUpper uint64
	newLower uint64
}

type job struct {
	client  net.Conn
	message *bitcoin.Message
	srv     *server
}

type server struct {
	listener                              net.Listener
	addMinerChannel, giveMinerChannel     chan net.Conn
	askMinerChannel, incrementNum, askNum chan bool
	jobChannel, HugeResultChan, done      chan job
	numMiners                             int
	giveNum                               chan int
	clientChunkChan                       chan clientChunkStruct
	HugeJobChan                           chan subJob
}

func startServer(port int) (*server, error) {
	// TODO: implement this!
	var svr *server
	svr = new(server)
	var err error
	svr.listener, err = net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		LOGF.Println("Error while starting server:", err)
		return nil, err
	}
	return svr, err
}

var LOGF *log.Logger

const chunkSize = 20000

func main() {
	// You may need a logger for debug purpose
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
	// Usage: LOGF.Println() or LOGF.Printf()

	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Printf("Usage: ./%s <port>", os.Args[0])
		return
	}

	port, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Println("Port must be a number:", err)
		return
	}

	srv, err := startServer(port)
	if err != nil {
		LOGF.Println(err)
		return
	}
	LOGF.Println("Server listening on port", port)
	srv.addMinerChannel, srv.giveMinerChannel = make(chan net.Conn), make(chan net.Conn)
	srv.askMinerChannel, srv.incrementNum, srv.askNum = make(chan bool), make(chan bool), make(chan bool)
	srv.giveNum = make(chan int)
	srv.jobChannel, srv.HugeResultChan, srv.done = make(chan job), make(chan job), make(chan job)
	srv.clientChunkChan = make(chan clientChunkStruct)
	srv.HugeJobChan = make(chan subJob)

	go handleMiners(srv)
	go mineIt(srv)
	go maintainNum(srv)
	go sendAndCompileResults(srv)
	defer srv.listener.Close()

	// TODO: implement this!

	for {
		conn, err := srv.listener.Accept()
		if err == nil {
			go handleConnections(conn, srv)
		}
	}
}

func handleConnections(conn net.Conn, srv *server) {
	buffer, err := bitcoin.MyRead(conn)
	if err != nil {
		LOGF.Println(err)
	}
	unmarshalled := new(bitcoin.Message)
	_ = json.Unmarshal(buffer, unmarshalled)
	switch unmarshalled.Type {
	case bitcoin.Join:
		srv.addMinerChannel <- conn

	case bitcoin.Request:
		srv.jobChannel <- job{conn, unmarshalled, srv}
	}
}

func handleMiners(srv *server) {
	for {
		select {
		case temp := <-srv.addMinerChannel:
			srv.incrementNum <- true
			srv.giveMinerChannel <- temp
		}
	}
}

func mineIt(srv *server) {
	for {
		srv.askNum <- true
		currentMiners := <-srv.giveNum
		if currentMiners == 0 {
			currentMiners++
		}
		j := <-srv.jobChannel
		noncesToCalculate := j.message.Upper - j.message.Lower
		if (noncesToCalculate <= chunkSize) || (noncesToCalculate > chunkSize && currentMiners == 1) {
			miner := <-srv.giveMinerChannel
			newMsg := bitcoin.NewRequest(j.message.Data, j.message.Lower, j.message.Upper)
			marshalled, _ := json.Marshal(*newMsg)
			err := bitcoin.MySend(miner, marshalled)
			if err != nil {
				LOGF.Println(err)
			} else {
				go waitForResults(miner, srv, j)
			}
		} else if noncesToCalculate > chunkSize && currentMiners > 1 {
			numChunks := noncesToCalculate / chunkSize
			srv.clientChunkChan <- clientChunkStruct{j.client, numChunks}
			go func() {
				for i := uint64(0); i < noncesToCalculate; {
					if i+chunkSize > noncesToCalculate {
						srv.HugeJobChan <- subJob{&j, i, j.message.Upper}
						break
					} else {
						srv.HugeJobChan <- subJob{&j, i, i + chunkSize}
					}
					i = i + chunkSize
				}
			}()
		}
	}
}

func sendAndCompileResults(srv *server) {
	clientChunkMap := make(map[net.Conn]uint64)
	processedChunkMap := make(map[net.Conn]uint64)
	currentResultsMap := make(map[net.Conn]nonceHashStruct)
	for {
		select {
		case sj := <-srv.HugeJobChan:
			miner := <-srv.giveMinerChannel
			go func() {
				newMsg := bitcoin.NewRequest(sj.j.message.Data, sj.newUpper, sj.newLower)
				marshalled, err := json.Marshal(*newMsg)
				err = bitcoin.MySend(miner, marshalled)
				if err != nil {
					LOGF.Println(err)
				} else {
					buffer, err := bitcoin.MyRead(miner)
					if err != nil {
						// Requeue job if miner fails
						srv.HugeJobChan <- sj
						return
					}
					unmarshalled := new(bitcoin.Message)
					_ = json.Unmarshal(buffer, &unmarshalled)
					go func() {
						srv.HugeResultChan <- job{client: sj.j.client, message: unmarshalled}
					}()
					srv.giveMinerChannel <- miner
				}
			}()

		case j := <-srv.HugeResultChan:
			if _, ok := clientChunkMap[j.client]; ok {
				processedChunkMap[j.client]++
				if j.message.Hash < currentResultsMap[j.client].hash {
					currentResultsMap[j.client] = nonceHashStruct{j.message.Nonce, j.message.Hash}
				}
				if processedChunkMap[j.client] == clientChunkMap[j.client] {
					newMsg := bitcoin.NewResult(currentResultsMap[j.client].hash, currentResultsMap[j.client].nonce)
					go func() {
						srv.done <- job{client: j.client, message: newMsg}
					}()
				}
			}

		case j := <-srv.done:
			marshalled, _ := json.Marshal(*j.message)
			err := bitcoin.MySend(j.client, marshalled)
			if err != nil {
				LOGF.Println(err)
			}

		case clientChunk := <-srv.clientChunkChan:
			clientChunkMap[clientChunk.client] = clientChunk.chunkSize
			processedChunkMap[clientChunk.client] = 0
			currentResultsMap[clientChunk.client] = nonceHashStruct{math.MaxUint64, math.MaxUint64}
		}
	}
}

func waitForResults(miner net.Conn, srv *server, j job) {
	buffer, err := bitcoin.MyRead(miner)
	if err != nil {
		srv.jobChannel <- j
		return
	}
	unmarshalled := new(bitcoin.Message)
	_ = json.Unmarshal(buffer, unmarshalled)
	msg := bitcoin.NewResult(unmarshalled.Hash, unmarshalled.Nonce)
	marshalled, _ := json.Marshal(*msg)
	err = bitcoin.MySend(j.client, marshalled)
	if err != nil {
		LOGF.Println(err)
	} else {
		srv.giveMinerChannel <- miner
	}
}

func maintainNum(srv *server) {
	for {
		select {
		case value := <-srv.incrementNum:
			if value {
				srv.numMiners++
			} else {
				srv.numMiners--
			}

		case <-srv.askNum:
			srv.giveNum <- srv.numMiners
		}
	}
}
