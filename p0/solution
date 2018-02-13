// Implementation of a KeyValueServer.

package p0

import (
	"bufio"
	"bytes"
	//"fmt"
	"io"
	"net"
	"strconv"
)

const MAX_MESSAGE_QUEUE_LENGTH = 500

// Stores a connection and corresponding message queue.
type client struct {
	connection       net.Conn
	messageQueue     chan []byte
	quitSignal_Read  chan int
	quitSignal_Write chan int
}

// Used to specify DBRequests
type db struct {
	isGet  bool
	key    string
	value  []byte
    client *client
}

// Implements KeyValueServer.
type keyValueServer struct {
	listener          net.Listener
	currentClients    []*client
	newConnection     chan net.Conn
	deadClient        chan *client
	dbQuery           chan *db
	countClients      chan int
	clientCount       chan int
	quitSignal_Main   chan int
	quitSignal_Accept chan int
}

// Initializes a new KeyValueServer.
func New() KeyValueServer {
	return &keyValueServer{
		nil,
		nil,
		make(chan net.Conn),
		make(chan *client),
		make(chan *db),
		make(chan int),
		make(chan int),
		make(chan int),
		make(chan int)}
}

// Implementation of Start for keyValueServer.
func (kvs *keyValueServer) Start(port int) error {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return err
	}

	kvs.listener = ln
	init_db()

	go runServer(kvs)
	go acceptRoutine(kvs)

	return nil
}

// Implementation of Close for keyValueServer.
func (kvs *keyValueServer) Close() {
	kvs.listener.Close()
	kvs.quitSignal_Main <- 0
	kvs.quitSignal_Accept <- 0
}

// Implementation of Count.
func (kvs *keyValueServer) Count() int {
	kvs.countClients <- 0
	return <-kvs.clientCount
}

// Main server routine.
func runServer(kvs *keyValueServer) {
	// defer fmt.Println("\"runServer\" ended.")

	for {
		select {
		// Add a new client to the client list.
		case newConnection := <-kvs.newConnection:
			c := &client{
				newConnection,
				make(chan []byte, MAX_MESSAGE_QUEUE_LENGTH),
				make(chan int),
				make(chan int)}
			kvs.currentClients = append(kvs.currentClients, c)
			go readRoutine(kvs, c)
			go writeRoutine(c)

		// Remove the dead client.
		case deadClient := <-kvs.deadClient:
			for i, c := range kvs.currentClients {
				if c == deadClient {
					kvs.currentClients =
						append(kvs.currentClients[:i], kvs.currentClients[i+1:]...)
					break
				}
			}

		// Run a query on the DB
		case request := <-kvs.dbQuery:
			// response required for GET query
			if request.isGet {
				s := get(request.key)
                //fmt.Printf("request key: %s; values: ", request.key)
                for _, v := range s {
                    //fmt.Printf("%s, ", v)
                    message := append(append([]byte(request.key), ","...), v...)
                    if len(request.client.messageQueue) == MAX_MESSAGE_QUEUE_LENGTH {
                        <-request.client.messageQueue
                    }
                    request.client.messageQueue <- message
                }
                //fmt.Printf("\n")
			} else {
				put(request.key, request.value)
			}

		// Get the number of clients.
		case <-kvs.countClients:
			kvs.clientCount <- len(kvs.currentClients)

		// End each client routine.
		case <-kvs.quitSignal_Main:
			for _, c := range kvs.currentClients {
				c.connection.Close()
				c.quitSignal_Write <- 0
				c.quitSignal_Read <- 0
			}
			return
		}
	}
}

// One running instance; accepts new clients and sends them to the server.
func acceptRoutine(kvs *keyValueServer) {
	// defer fmt.Println("\"acceptRoutine\" ended.")

	for {
		select {
		case <-kvs.quitSignal_Accept:
			return
		default:
			conn, err := kvs.listener.Accept()
			if err == nil {
				kvs.newConnection <- conn
			}
		}
	}
}

// One running instance for each client; reads in
// new  messages and sends them to the server.
func readRoutine(kvs *keyValueServer, c *client) {
	// defer fmt.Println("\"readRoutine\" ended.")

	clientReader := bufio.NewReader(c.connection)

	// Read in messages.
	for {
		select {
		case <-c.quitSignal_Read:
			return
		default:
			message, err := clientReader.ReadBytes('\n')

			if err == io.EOF {
				kvs.deadClient <- c
			} else if err != nil {
				return
			} else {
				tokens := bytes.Split(message, []byte(","))
				if string(tokens[0]) == "put" {
					key := string(tokens[1][:])

					// do a "put" query
					kvs.dbQuery <- &db{
						isGet: false,
						key:   key,
						value: tokens[2],
                        client: c,
					}
				} else {
					// remove trailing \n from get,key\n request
					keyBin := tokens[1][:len(tokens[1])-1]
					key := string(keyBin[:])

                    //fmt.Printf("Getting key %s\n", key)

					// do a "get" query
					kvs.dbQuery <- &db{
						isGet: true,
						key:   key,
                        client: c,
					}
				}
			}
		}
	}
}

// One running instance for each client; writes messages
// from the message queue to the client.
func writeRoutine(c *client) {
	// defer fmt.Println("\"writeRoutine\" ended.")

	for {
		select {
		case <-c.quitSignal_Write:
			return
		case message := <-c.messageQueue:
			c.connection.Write(message)
		}
	}
}
