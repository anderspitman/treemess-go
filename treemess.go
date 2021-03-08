package treemess

import (
	"sync"
        "crypto/rand"
        "math/big"
)

type TreeMess struct {
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
