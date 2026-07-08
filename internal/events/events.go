package events

import (
	"sync"
)

var (
	mu      sync.Mutex
	clients map[chan string]bool
)

func init() {
	clients = make(map[chan string]bool)
}

// Subscribe returns a channel that receives SSE-formatted event strings.
func Subscribe() chan string {
	mu.Lock()
	defer mu.Unlock()
	c := make(chan string, 100)
	clients[c] = true
	return c
}

// Unsubscribe removes the channel from the broadcast list and closes it.
func Unsubscribe(c chan string) {
	mu.Lock()
	defer mu.Unlock()
	if clients[c] {
		delete(clients, c)
		close(c)
	}
}

// Publish broadcasts an event and JSON data to all subscribed clients.
func Publish(event, data string) {
	mu.Lock()
	defer mu.Unlock()
	msg := "event: " + event + "\ndata: " + data + "\n\n"
	for c := range clients {
		select {
		case c <- msg:
		default:
			// Drop message if client is too slow rather than blocking
		}
	}
}
