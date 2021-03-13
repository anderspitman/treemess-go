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
	listeners []func(string, interface{})
	maps      []func(string, interface{}) (string, interface{})
	mut       *sync.Mutex
}

type internalMessage struct {
	Channel string
	SrcId   string
	Data    interface{}
}

func NewTreeMess() *TreeMess {
	id, _ := genRandomCode(32)

	in := make(chan internalMessage)
	out := make(map[string]chan internalMessage)

	tm := &TreeMess{
		Id: id,

		in:        in,
		out:       out,
		listeners: []func(string, interface{}){},
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
				go listener(outMessage.Channel, outMessage.Data)
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

func (t *TreeMess) Listen(callback func(string, interface{})) {
	t.mut.Lock()
	defer t.mut.Unlock()

	t.listeners = append(t.listeners, callback)
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

func (t *TreeMess) getListeners() []func(string, interface{}) {
	t.mut.Lock()
	defer t.mut.Unlock()

	listeners := []func(string, interface{}){}

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

type PubSub struct {
	id                   string
	parent               *PubSub
	children             []*PubSub
	channelMappings      [][]string
	callbacks            map[string][]func(interface{})
	allMessagesCallbacks []func(Message)
	mut                  *sync.Mutex
}

type Message struct {
	Channel string      `json:"channel"`
	Type    string      `json:"type"`
	Data    interface{} `json:"data"`
	SrcId   string      `json:"srcId"`
}

func NewPubSub() *PubSub {
	id, _ := genRandomCode(32)

	return &PubSub{
		id:              id,
		parent:          nil,
		children:        []*PubSub{},
		channelMappings: [][]string{},
		callbacks:       make(map[string][]func(interface{})),
		mut:             &sync.Mutex{},
	}
}

func (ps *PubSub) Pub(channel string, message Message) {
	// TODO: is it safe to use a goroutine here?
	go ps.PubBroadcast(channel, message, ps.id)
}

// Propagates pubs while avoiding infinite rebounds
func (ps *PubSub) PubBroadcast(channel string, message Message, srcId string) {
	ps.mut.Lock()
	defer ps.mut.Unlock()

	// TODO: feels hacky
	message.Channel = channel
	message.SrcId = srcId

	for _, callback := range ps.allMessagesCallbacks {
		callback(message)
	}

	if callbacks, ok := ps.callbacks[ps.getOurChannel(channel)]; ok {
		for _, callback := range callbacks {
			callback(message.Data)
		}
	}

	if ps.parent != nil && ps.parent.id != message.SrcId {
		ps.parent.PubBroadcast(ps.getParentChannel(channel), message, ps.id)
	}

	for _, child := range ps.children {
		if child.id != message.SrcId {
			ch := ps.getOurChannel(channel)
			child.PubBroadcast(ch, message, ps.id)
		}
	}
}

func (ps *PubSub) Sub(channel string, callback func(interface{})) {
	ps.mut.Lock()
	defer ps.mut.Unlock()

	ps.callbacks[channel] = append(ps.callbacks[channel], callback)
}

func (ps *PubSub) SubAllChannels(callback func(Message)) {
	ps.mut.Lock()
	defer ps.mut.Unlock()

	ps.allMessagesCallbacks = append(ps.allMessagesCallbacks, callback)
}

func (ps *PubSub) Map(parentChannel, ourChannel string) {
	ps.mut.Lock()
	defer ps.mut.Unlock()

	mapping := []string{parentChannel, ourChannel}
	// insert into front of array
	ps.channelMappings = append([][]string{mapping}, ps.channelMappings...)
}

func (ps *PubSub) Inherit() *PubSub {
	id, _ := genRandomCode(32)

	child := &PubSub{
		id:              id,
		parent:          ps,
		children:        []*PubSub{},
		channelMappings: [][]string{},
		callbacks:       make(map[string][]func(interface{})),
		mut:             &sync.Mutex{},
	}

	ps.mut.Lock()
	defer ps.mut.Unlock()

	ps.children = append(ps.children, child)

	return child
}

func (ps *PubSub) getOurChannel(channel string) string {
	//ps.mut.Lock()
	//defer ps.mut.Unlock()

	for _, mapping := range ps.channelMappings {
		parentChannel := mapping[0]
		ourChannel := mapping[1]

		if parentChannel == channel {
			return ourChannel
		}
	}

	return channel
}

func (ps *PubSub) getParentChannel(channel string) string {
	//ps.mut.Lock()
	//defer ps.mut.Unlock()

	for _, mapping := range ps.channelMappings {
		parentChannel := mapping[0]
		ourChannel := mapping[1]

		if ourChannel == channel {
			return parentChannel
		}
	}

	return channel
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
