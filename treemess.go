package treemess

import (
	"time"
	//"fmt"
	"crypto/rand"
	"math/big"
	"sync"
)

type TreeMess struct {
	Id string

	in        chan internalMessage
	out       map[string]chan internalMessage
	listeners []chan Message
	maps      []func(string, interface{}) (string, interface{})
	mut       *sync.Mutex
}

type Message struct {
	Channel string      `json:"channel"`
	Data    interface{} `json:"data"`
}

type internalMessage struct {
	Channel string
	Data    interface{}
	SrcId   string
}

func NewTreeMess() *TreeMess {
	id, _ := genRandomCode(32)

	in := make(chan internalMessage)
	out := make(map[string]chan internalMessage)

	tm := &TreeMess{
		Id: id,

		in:        in,
		out:       out,
		listeners: []chan Message{},
		maps:      []func(string, interface{}) (string, interface{}){},
		mut:       &sync.Mutex{},
	}

	go tm.handleMessages()

	return tm
}

func (t *TreeMess) handleMessages() {
	for inMessage := range t.in {

		//fmt.Println(t.Id, inMessage)

		outMessage := inMessage

		for _, mapFunc := range t.getMaps() {
			channel, data := mapFunc(outMessage.Channel, outMessage.Data)
			outMessage.Channel = channel
			outMessage.Data = data
		}

		for _, listener := range t.getListeners() {
			//fmt.Println(t.Id, "attempt send listener", inMessage)
			if outMessage.Channel != "" {
				//fmt.Println(t.Id, "attempt send listener", inMessage)
				// TODO: I feel like we have too many go routines being used in TreeMess.
				go func() {
					msg := Message{
						Channel: outMessage.Channel,
						Data:    outMessage.Data,
					}
					listener <- msg
				}()
			}
		}

		for id, outChannel := range t.getOutChannels() {
			//fmt.Println(t.Id, "check", id, inMessage)
			if outMessage.Channel != "" && id != inMessage.SrcId {
				//fmt.Println(t.Id, "send", id, inMessage)
				outMessage.SrcId = t.Id
				select {
				case outChannel <- outMessage:
				case <-time.After(1 * time.Second):
					panic("Timeout sending")
				}
				//fmt.Println(t.Id, "done", id, inMessage)
			}
		}
	}
}

func (t *TreeMess) Send(channel string, inMessage interface{}) {
	msg := internalMessage{
		Channel: channel,
		SrcId:   t.Id,
		Data:    inMessage,
	}

	t.in <- msg
}

func (t *TreeMess) Listen(ch chan Message) {
	t.mut.Lock()
	defer t.mut.Unlock()

	t.listeners = append(t.listeners, ch)
}

func (t *TreeMess) ListenFunc(callback func(Message)) {

	ch := make(chan Message)

	t.Listen(ch)

	go func() {
		for msg := range ch {
			callback(msg)
		}
	}()
}

func (t *TreeMess) Map(callback func(string, interface{}) (string, interface{})) {
	t.mut.Lock()
	defer t.mut.Unlock()

	t.maps = append(t.maps, callback)
}

func (t *TreeMess) Branch() *TreeMess {

	t.mut.Lock()
	defer t.mut.Unlock()

	newTm := NewTreeMess()

	newTm.out[t.Id] = t.in
	t.out[newTm.Id] = newTm.in

	return newTm
}

func (t *TreeMess) BranchWithId(id string) *TreeMess {
	t.mut.Lock()
	defer t.mut.Unlock()

	newTm := NewTreeMess()
	newTm.Id = id

	newTm.out[t.Id] = t.in
	t.out[newTm.Id] = newTm.in

	return newTm
}

func (t *TreeMess) getListeners() []chan Message {
	t.mut.Lock()
	defer t.mut.Unlock()

	listeners := []chan Message{}

	for _, listener := range t.listeners {
		listeners = append(listeners, listener)
	}

	return listeners
}

func (t *TreeMess) getOutChannels() map[string]chan internalMessage {
	t.mut.Lock()
	defer t.mut.Unlock()

	out := make(map[string]chan internalMessage)

	for k, v := range t.out {
		out[k] = v
	}

	return out
}

func (t *TreeMess) getMaps() []func(string, interface{}) (string, interface{}) {
	t.mut.Lock()
	defer t.mut.Unlock()

	mapFuncs := []func(string, interface{}) (string, interface{}){}

	for _, mapFunc := range t.maps {
		mapFuncs = append(mapFuncs, mapFunc)
	}

	return mapFuncs
}

const chars string = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func genRandomCode(length int) (string, error) {
	id := ""
	for i := 0; i < length; i++ {
		randIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		id += string(chars[randIndex.Int64()])
	}
	return id, nil
}
