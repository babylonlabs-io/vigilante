package zmq

import (
	"encoding/binary"
	"github.com/babylonchain/vigilante/types"
	"sync"
	"time"

	zmq "github.com/pebbe/zmq4"
)

// SequenceMsg is a subscription event coming from a "sequence" ZMQ message.
type SequenceMsg struct {
	Hash       [32]byte // use encoding/hex.EncodeToString() to get it into the RPC method string format.
	Event      types.EventType
	MempoolSeq uint64
}

type subscriptions struct {
	sync.RWMutex

	exited      chan struct{}
	zfront      *zmq.Socket
	latestEvent time.Time

	sequence []chan *SequenceMsg
}

// SubscribeSequence subscribes to the ZMQ "sequence" messages as SequenceMsg items pushed onto the channel.
// Call cancel to cancel the subscription and let the client release the resources. The channel is closed
// when the subscription is canceled or when the client is closed.
func (c *Client) SubscribeSequence() (subCh chan *SequenceMsg, cancel func(), err error) {
	if c.zsub == nil {
		err = ErrSubscribeDisabled
		return
	}
	c.subs.Lock()
	select {
	case <-c.subs.exited:
		err = ErrSubscribeExited
		c.subs.Unlock()
		return
	default:
	}
	if len(c.subs.sequence) == 0 {
		_, err = c.subs.zfront.SendMessage("subscribe", "sequence")
		if err != nil {
			c.subs.Unlock()
			return
		}
	}
	subCh = make(chan *SequenceMsg, c.subChannelBufferSize)
	c.subs.sequence = append(c.subs.sequence, subCh)
	c.subs.Unlock()
	cancel = func() {
		err = c.unsubscribeSequence(subCh)
		if err != nil {
			log.Errorf("Error unsubscribing from sequence: %v", err)
			return
		}
	}
	return
}

func (c *Client) unsubscribeSequence(subCh chan *SequenceMsg) (err error) {
	c.subs.Lock()
	select {
	case <-c.subs.exited:
		err = ErrSubscribeExited
		c.subs.Unlock()
		return
	default:
	}
	for i, ch := range c.subs.sequence {
		if ch == subCh {
			c.subs.sequence = append(c.subs.sequence[:i], c.subs.sequence[i+1:]...)
			if len(c.subs.sequence) == 0 {
				_, err = c.subs.zfront.SendMessage("unsubscribe", "sequence")
			}
			break
		}
	}
	c.subs.Unlock()
	close(subCh)
	return
}

func (c *Client) zmqHandler() {
	defer c.wg.Done()
	defer func(zsub *zmq.Socket) {
		err := zsub.Close()
		if err != nil {
			log.Errorf("Error closing ZMQ socket: %v", err)
		}
	}(c.zsub)
	defer func(zback *zmq.Socket) {
		err := zback.Close()
		if err != nil {
			log.Errorf("Error closing ZMQ socket: %v", err)
		}
	}(c.zback)

	poller := zmq.NewPoller()
	poller.Add(c.zsub, zmq.POLLIN)
	poller.Add(c.zback, zmq.POLLIN)
OUTER:
	for {
		// Wait forever until a message can be received or the context was cancelled.
		polled, err := poller.Poll(-1)
		if err != nil {
			break OUTER
		}

		for _, p := range polled {
			switch p.Socket {
			case c.zsub:
				msg, err := c.zsub.RecvMessage(0)
				if err != nil {
					break OUTER
				}
				c.subs.latestEvent = time.Now()
				switch msg[0] {
				case "sequence":
					var sequenceMsg SequenceMsg
					copy(sequenceMsg.Hash[:], msg[1])
					switch msg[1][32] {
					case 'C':
						sequenceMsg.Event = types.BlockConnected
					case 'D':
						sequenceMsg.Event = types.BlockDisconnected
					case 'R':
						sequenceMsg.Event = types.TransactionRemoved
						sequenceMsg.MempoolSeq = binary.LittleEndian.Uint64([]byte(msg[1][33:]))
					case 'A':
						sequenceMsg.Event = types.TransactionAdded
						sequenceMsg.MempoolSeq = binary.LittleEndian.Uint64([]byte(msg[1][33:]))
					default:
						// This is a fault. Drop the message.
						continue
					}
					c.subs.RLock()
					for _, ch := range c.subs.sequence {
						select {
						case ch <- &sequenceMsg:
						default:
							select {
							// Pop the oldest item and push the newest item (the user will miss a message).
							case _ = <-ch:
								ch <- &sequenceMsg
							case ch <- &sequenceMsg:
							default:
							}
						}
					}
					c.subs.RUnlock()
				}

			case c.zback:
				msg, err := c.zback.RecvMessage(0)
				if err != nil {
					break OUTER
				}
				switch msg[0] {
				case "subscribe":
					if err := c.zsub.SetSubscribe(msg[1]); err != nil {
						break OUTER
					}
				case "unsubscribe":
					if err := c.zsub.SetUnsubscribe(msg[1]); err != nil {
						break OUTER
					}
				case "term":
					break OUTER
				}
			}
		}
	}

	c.subs.Lock()
	close(c.subs.exited)
	err := c.subs.zfront.Close()
	if err != nil {
		log.Errorf("Error closing zfront: %v", err)
		return
	}
	// Close all subscriber channels, that will make them notice that we failed.
	if len(c.subs.sequence) > 0 {
		err := c.zsub.SetUnsubscribe("sequence")
		if err != nil {
			log.Errorf("Error unsubscribing from sequence: %v", err)
			return
		}
	}
	for _, ch := range c.subs.sequence {
		close(ch)
	}
	c.subs.Unlock()
}