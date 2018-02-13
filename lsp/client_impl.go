// Contains the implementation of a LSP client.

package lsp

import (
	"errors"
	"github.com/cmu440/lspnet"
	"encoding/json"
	"fmt"
	"time"
)

func ackTicker(cli *client){
	ticker := time.NewTicker(time.Duration(cli.params.EpochMillis)*time.Millisecond)
	defer ticker.Stop()
	var count int
	count = 0
	for {
		select {
			case <- cli.exit:
				return
			case <-ticker.C:
				count = count + 1
				if(cli.connId!=0){
					return
				}
				if(count>cli.params.EpochLimit){
					cli.Close()
					return
				}
				msg := NewConnect()
				toSend,_ := json.Marshal(msg)
				cli.conn.Write(toSend)
		}
	}
}

func consume(cli *client){
	for{
		select {
		case <-cli.exit:
			return
		case	msg := <- cli.writeBuf:
			if cli.status == -1{
				return
			}
			toSend,_ := json.Marshal(msg)
			fmt.Print("writing to buffer")
			fmt.Print(len(toSend))
			cli.conn.Write(toSend)
		}
	}
}

func listenRead(cli *client){
	for {
		data := make([]byte,3000)
		var msg Message
		n,_,_:=cli.conn.ReadFromUDP(data)
		err := json.Unmarshal(data[:n],&msg)
		if err!=nil{
			fmt.Println(err)
			return
		}
		fmt.Print("Client received: ")
		fmt.Println(msg)

		if msg.Type == MsgData{
			if msg.Size != len(msg.Payload){
				fmt.Println("Client: Payload size not matching skipping\n")
				continue
			}
		}

		if msg.SeqNum == 0 && cli.connId  == 0{
			cli.connId = msg.ConnID
			go processUpdates(cli)
			var id int
			for i:=0;i<len(cli.sendWhenConnected);i++{
				id = cli.sendWhenConnected[i]
				toSend := NewAck(cli.connId,id)
				toSendBuf,_ := json.Marshal(toSend)
				cli.conn.Write(toSendBuf)
			}
		} else{
			cli.readBuf<-&msg
		}

		if msg.Type == MsgData && cli.connId  != 0{
			toSend := NewAck(cli.connId,msg.SeqNum)
			toSendBuf,_ := json.Marshal(toSend)
			cli.conn.Write(toSendBuf)
		}

		if msg.Type == MsgData && cli.connId  == 0{
			cli.sendWhenConnected = append(cli.sendWhenConnected,msg.SeqNum)
		}

	}
}

func processUpdates(cli *client){
	var readMsg *Message
	var writeMsg *Message
	ticker := time.NewTicker(time.Duration(cli.params.EpochMillis)*time.Millisecond)
	defer ticker.Stop()
	SeqIdToAckCount := make(map[int]int)
	for {
		select {
		case <-cli.exit:
			return
		case <-ticker.C:
			for _,val := range(cli.unacked){
					SeqIdToAckCount[val.SeqNum]++
					if SeqIdToAckCount[val.SeqNum] > cli.params.EpochLimit{
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
				if(readMsg.Type == MsgData){
					cli.received[readMsg.SeqNum] = readMsg.Payload[:readMsg.Size]
					for {
						val,ok:=cli.received[cli.currentSeqReadId+1]
						if ok{
							cli.currentSeqReadId = cli.currentSeqReadId + 1
							cli.processedRead<-val
						} else{
							break
						}
					}
				}
			case writeMsg = <- cli.writeBufStaging:
				fmt.Println("Client: Got messaged to be sent for write")
				if len(cli.unacked)<cli.params.WindowSize{
					cli.unacked = append(cli.unacked,writeMsg)
					cli.writeBuf<-writeMsg
					SeqIdToAckCount[writeMsg.SeqNum]++
				} else {
					fmt.Println("Client: Lots of messages are unacked so not sending anymore")
					cli.writeBufStaging <- writeMsg
				}
		}
	}
}


type client struct {
	params *Params
	connId int
	seqIdSent int
	unacked []*Message
	writeBuf chan *Message
	readBuf chan *Message
	writeBufStaging chan *Message
	status int
	conn *lspnet.UDPConn
	addr *lspnet.UDPAddr
	received map[int][]byte
	currentSeqReadId int
	currentSeqWriteId int
	processedRead chan []byte
	sendWhenConnected []int
	exit chan int
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	addr,err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil{
		return nil,err
	}
	fmt.Println("Client: " + hostport)
	conn,err := lspnet.DialUDP("udp", nil, addr)
	if err != nil{
		return nil,err
	}
	res := client{writeBuf:make(chan *Message,2000),
								status:0,
								params:params,
								seqIdSent:-1,
								conn:conn,
								writeBufStaging:make(chan *Message,2000),
								readBuf:make(chan *Message,2000),
								received:make(map[int][]byte),
								currentSeqReadId:0,currentSeqWriteId:0,
								processedRead:make(chan []byte,2000),
								exit:make(chan int, 20)}
	msg := NewConnect()
	res.writeBuf<-msg
	go consume(&res)
	go listenRead(&res)
	go ackTicker(&res)
	time.Sleep(1 * time.Millisecond)
	/*for {
		if res.status==0{
			time.Sleep(100 * time.Millisecond)
			continue
		}
  }*/
	if res.status == -1{
		return nil,errors.New("Some error happened")
	}
	return &res,nil
}

func (c *client) ConnID() int {
	return c.connId
}

func (c *client) Read() ([]byte, error) {
	msg := <- c.processedRead
	return msg,nil
}

func (c *client) Write(payload []byte) error {
	l := len(payload)
	s:=0
	e:=800
	var end int
	var data []byte

	for{
		end = e
		if(l<=e){
			end = l
		}

		data = payload[s:end]
		fmt.Println("Client: Putting message to buffer")
		c.currentSeqWriteId = c.currentSeqWriteId + 1
		msg:=NewData(c.connId,c.currentSeqWriteId,len(data),data)
		c.writeBufStaging<-msg
		if(l<=e){
			break
		}
		s = s+800
		e=e+800
	}
	if c.status == -1{
		errors.New("Server error")
	}
	return nil

}

func (c *client) Close() error {
	if(c.status == -1){
		return errors.New("Already Closed")
	}
	c.status = -1
	for i:=0;i<10;i++{
		c.exit <-1
	}
	c.conn.Close()
	return nil
}
