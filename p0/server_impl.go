// Implementation of a KeyValueServer. Students should write their code in this file.

package p0

import (
	"net"
	"errors"
	"strconv"
	"fmt"
	"bufio"
	"strings"
	"io"
)

func startNewPort(l net.Listener, kvs *keyValueServer) {
	conn,err := l.Accept()
	defer l.Close()
	kvs.noOfClients++
	if err == nil{
		//
	} else{
		return
	}
	defer conn.Close()
	go handleConenction(conn,kvs)
  <-kvs.closeConnection
}

func strlen(in string) int{
	for i:=0;;i++{
		if in[i] == '\n' || in[i] == 0{
			return i
		}
	}
}

func handleWrite(kill chan int, buffer chan []byte, conn net.Conn){
	for{
		select{
			case <-kill:
				return
			case msg :=<-buffer:
			{
				fmt.Print("Sending: ")
				fmt.Print(msg)
				conn.Write(msg)
			}
		}
	}
}

func handleConenction(conn net.Conn, kvs *keyValueServer){
	kill := make(chan int)
	buffer := make(chan []byte,500)
	go handleWrite(kill, buffer, conn)
	for {
		input,err := bufio.NewReader(conn).ReadString('\n')
		if err == io.EOF{
			kvs.noOfClients--
			kill<-1
			return
		}else if err != nil {
			fmt.Println("Closing Connection in handleConenction")
			kill<-1
			return
		}
		if strlen(input) == 0{
			continue
		}
		//input = input[:strlen(input
		request := strings.Split(input,",")
		<-kvs.signalRead
		fmt.Println("Received: " + input)
		if len(request) == 2{
			val := get(request[1])
			fmt.Println(val)
			for _,v:=range val{
				fmt.Println(v)
				message := append(append([]byte(request[1]), ","...), v...)
				if(len(buffer)<500){
					fmt.Println("Entering roi")
					buffer<-message
				}
			}

		} else if len(request) ==3{
			put(request[1],[]byte(request[2]))
		}
		kvs.signalRead<-1
	}
}

type keyValueServer struct {
	closeConnection chan int
	noOfClients int
	signalRead chan int
}

// New creates and returns (but does not start) a new KeyValueServer.
func New() KeyValueServer {
	ret := keyValueServer{make(chan int,1),0,make(chan int,1)}
	ret.signalRead <- 1
	return &ret
}

func (kvs *keyValueServer) Start(port int) error {
	l,err := net.Listen("tcp",":"+strconv.Itoa(port))
	if (err != nil){
		return errors.New("Error listening to: " + strconv.Itoa(port))
	}

	go startNewPort(l, kvs)
	return nil
}

func (kvs *keyValueServer) Close() {
	for i := 0; i < kvs.noOfClients; i++ {
		kvs.closeConnection <- 1;
	}
	kvs.noOfClients = 0
	fmt.Println("Close called")
}

func (kvs *keyValueServer) Count() int {

	return kvs.noOfClients
}

// TODO: add additional methods/functions below!
