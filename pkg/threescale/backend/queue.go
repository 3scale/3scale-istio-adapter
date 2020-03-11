package backend

import (
	"gopkg.in/oleiade/lane.v1"
)

// dequeue is a head-tail linked list data structure provided for working with Application(s)
type dequeue struct {
	queue *lane.Deque
}

// prepend inserts application at the front of the Deque
func (q *dequeue) prepend(application *Application) bool {
	return q.queue.Prepend(application)
}

// append inserts application at the back of the Deque
func (q *dequeue) append(application *Application) bool {
	return q.queue.Append(application)
}

// first returns the the first application in the Deque
func (q *dequeue) first() *Application {
	return q.queue.First().(*Application)
}

// last returns the the last application in the Deque
func (q *dequeue) last() *Application {
	return q.queue.Last().(*Application)
}

// shift removes the first application from the Deque
func (q *dequeue) shift() *Application {
	return q.queue.Shift().(*Application)
}

// pop removes the last application from the Deque
func (q *dequeue) pop() *Application {
	return q.queue.Pop().(*Application)
}

// isFull returns true if the deque is at full capacity
func (q *dequeue) isFull() bool {
	return q.queue.Full()
}

// isEmpty returns true if the deque is empty
func (q *dequeue) isEmpty() bool {
	return q.queue.Empty()
}

// newQueue returns a Deque with the capacity limited to the provided size
func newQueue(size int) *dequeue {
	return &dequeue{queue: lane.NewCappedDeque(size)}
}
