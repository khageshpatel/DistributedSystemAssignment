// Contains the implementation of a LSP server.

package lsp

import (
	"errors"
	"github.com/cmu440/lspnet"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

func poll(svr *server){
	for {
		data := make([]byte,3000)
		var msg Message
		n,addr,err:=svr.conn.ReadFromUDP(data)
		if err!=nil{
			fmt.Println(err)
			return
		}
		err = json.Unmarshal(data[:n],&msg)
		if err!=nil{
			fmt.Println(err)
			continue
		}
		if msg.Type == MsgData{
			if msg.Size != len(msg.Payload){
				fmt.Println("Server: Payload size not matching skipping\n")
				continue
			}
		}
		fmt.Println(addr.String())
		var v int
		if msg.Type == MsgConnect{
			v,ok := svr.addrToId[addr.String()]
			if !ok{
				svr.portPool = svr.portPool + 1
				v = svr.portPool
				svr.addrToId[addr.String()] = v
			}
			toSend :=NewAck(v,msg.SeqNum)
			toSendBuf,_ := json.Marshal(&toSend)
			svr.conn.WriteToUDP(toSendBuf,addr)
		}
		v,_ = svr.addrToId[addr.String()]
		if msg.Type == MsgData{
			toSend :=NewAck(v,msg.SeqNum)
			toSendBuf,_ := json.Marshal(&toSend)
			svr.conn.WriteToUDP(toSendBuf,addr)
		}

		cli,ok := svr.idToClient[v]
		if !ok{
			cli = &clientHandle{
				id : v,addr:addr,
				svr : svr,writeBuf:make(chan *Message,3000),
				writeBufStaging:make(chan *Message,3000),
				readBuf:make(chan *Message,3000),
				received:make(map[int][]byte),
				currentSeqReadId:0,currentSeqWriteId:0,
				exit: make(chan int, 20)}
			go cli.consume()
			go cli.processUpdates()
			fmt.Print("Client id created: ")
			fmt.Println(v)
			svr.idToClient[v] = cli
		}
		cli.readBuf<-&msg
	}
}

func (cli *clientHandle) Close() error {
	if cli.status == -1{
		return errors.New("Server already close")
	}
	cli.status = -1
	for i:=0;i<10;i++{
		cli.exit<-1
	}
	return nil
}

func (cli *clientHandle) consume(){
	for{
		select {
		case <-cli.exit:
				return
			case msg := <- cli.writeBuf:
				/*for{
					if cli.status == -1{
						return
					}
					if len(cli.unacked)<cli.svr.params.WindowSize+1{
						break;
					} else{
						fmt.Println("Got enough unacked messages so cant send the message")
					}
				}*/
				toSend,_ := json.Marshal(msg)
				fmt.Print("writing to buffer")
				fmt.Print(len(toSend))
				cli.svr.conn.WriteToUDP(toSend,cli.addr)
		}
	}
}

func (cli *clientHandle) processUpdates(){
	var readMsg *Message
	var writeMsg *Message
	ticker := time.NewTicker(time.Duration(cli.svr.params.EpochMillis)*time.Millisecond)
	defer ticker.Stop()
	SeqIdToAckCount := make(map[int]int)
	for {
		select {
		case <-cli.exit:
				return
		case <-ticker.C:
			for _,val := range(cli.unacked){
					SeqIdToAckCount[val.SeqNum]++
					if SeqIdToAckCount[val.SeqNum] > cli.svr.params.EpochLimit{
						cli.Close()
						return
					}
					cli.writeBuf<-val
			}
		case readMsg = <- cli.readBuf:
				if(readMsg.Type == MsgAck){
					for ind,val:=range(cli.unacked){
						if(val.SeqNum == readMsg.SeqNum){
							cli.unacked = append(cli.unacked[:ind],cli.unacked[ind+1:]...)
							delete(SeqIdToAckCount,readMsg.SeqNum)
							break
						}
					}
				}
				fmt.Print("Server: Got data from client")
				fmt.Println(readMsg)
				if(readMsg.Type == MsgData){
					fmt.Print("payload actual size: ")
					fmt.Println(len(readMsg.Payload))
					fmt.Print("size provided: ")
					fmt.Println(readMsg.Size)
					cli.received[readMsg.SeqNum] = readMsg.Payload[:readMsg.Size]
					for {
						val,ok:=cli.received[cli.currentSeqReadId+1]
						if ok{
							cli.currentSeqReadId = cli.currentSeqReadId + 1
							pReadDone := &processedReadMessage{
														id:cli.id,
														data:val}
							cli.svr.processedRead<-pReadDone
						} else{
							break
						}
					}
				}
			case writeMsg = <- cli.writeBufStaging:
				//fmt.Println("Client: Got messaged to be sent for write")
				if len(cli.unacked)<cli.svr.params.WindowSize{
					cli.unacked = append(cli.unacked,writeMsg)
					cli.writeBuf<-writeMsg
					SeqIdToAckCount[writeMsg.SeqNum]++
				} else {
					cli.writeBufStaging <- writeMsg
				}
		}
	}
}

func (cli *clientHandle) write(payload []byte) error{
	l := len(payload)
	s:=0
	e:=800
	var end int
	var data []byte
	fmt.Println("Server: Putting message to buffer")
	for{
		end = e
		if(l<=e){
			end = l
		}
		data = payload[s:end]
		cli.currentSeqWriteId = cli.currentSeqWriteId + 1
		msg:=NewData(cli.id,cli.currentSeqWriteId,len(data),data)
		cli.writeBufStaging<-msg
		if(l<=e){
			break
		}
		s = s+800
		e=e+800
	}
	if cli.status == -1{
		return errors.New("Server error")
	}
	return nil
}

type processedReadMessage struct{
	id int
	data []byte
}

type clientHandle struct{
	//Each client connection we are having
	unacked []*Message
	status int
	writeBuf chan *Message
	readBuf chan *Message
	writeBufStaging chan *Message
	addr *lspnet.UDPAddr
	id int
	svr *server
	received map[int][]byte
	currentSeqReadId int
	currentSeqWriteId int
	exit chan int
}

type server struct {
	conn *lspnet.UDPConn
	addr *lspnet.UDPAddr
	portPool int
	idToClient map[int]*clientHandle
	params *Params
	addrToId map[string]int
	processedRead chan *processedReadMessage
	status int
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	addr,err := lspnet.ResolveUDPAddr("udp", ":" + strconv.Itoa(port))
	fmt.Print("Server:" + strconv.Itoa(port))
	if err != nil{
		return nil,err
	}
	conn,err := lspnet.ListenUDP("udp", addr)
	if err != nil{
		return nil,err
	}
	res := server{conn:conn,addr:addr,portPool:0,
		            idToClient:make(map[int]*clientHandle),
							  params:params,addrToId:make(map[string]int),
								processedRead:make(chan *processedReadMessage,3000)}
	go poll(&res)
	return &res, nil
}

func (s *server) Read() (int, []byte, error) {
	if s.status == -1{
		return -1, nil, errors.New("not yet implemented")
	}
	data := <-s.processedRead
	return data.id,data.data,nil
}

func (s *server) Write(connID int, payload []byte) error {
	if s.status == -1{
		return errors.New("not yet implemented")
	}
	cli,ok := s.idToClient[connID]
	if !ok{
		return errors.New("Invalid conn id")
	}
	err:=cli.write(payload)
	return err
}

func (s *server) CloseConn(connID int) error {
	cli,ok := s.idToClient[connID]
	if !ok{
		return errors.New("conn id does not exist")
	}
	err:=cli.Close()
	return err
}

func (s *server) Close() error {
	if s.status == -1{
		return errors.New("Server already close")
	}
	s.status = -1
	for _,cli := range(s.idToClient){
		cli.Close()
	}
	s.conn.Close()
	return nil
}
