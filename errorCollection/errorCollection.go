package errorCollection

import (
	"errorX"
	queue "github.com/fwhezfwhez/go-queue"
	"log"
	"sync"
)

type ErrorBox *queue.Queue
type ErrorHandler func(e error)

type ErrorCollection struct {
	errors           ErrorBox       // store errors in queue
	ErrorHandleChain []ErrorHandler // handler to deal with error
	M                *sync.Mutex    // field lock
	HasErrorChan     chan error     // not use
	CatchErrorChan   chan error     // when error in queue,it will be put into catchErrorChan
	AutoHandleChan   chan int       // used to close the channel
	AutoHandleFlag   bool
}

// New a error collection
func NewCollection() *ErrorCollection {
	log.SetFlags(log.Llongfile | log.LstdFlags)
	return &ErrorCollection{
		errors:           queue.NewCap(200),
		ErrorHandleChain: make([]ErrorHandler, 0, 5),
		HasErrorChan:     make(chan error, 1),
		CatchErrorChan:   make(chan error, 1),
		AutoHandleChan:   make(chan int, 1),
		AutoHandleFlag:   false,
		M:                &sync.Mutex{},
	}
}
func Default() *ErrorCollection {
	e := NewCollection()
	e.AddHandler(Logger())
	return e
}
func (ec *ErrorCollection) GetQueueLock() *sync.Mutex {
	return ec.errors.Mutex
}

// Return its error number
func (ec *ErrorCollection) SafeLength() int {
	return (*queue.Queue)(ec.errors).SafeLength()
}

// Return its error number
func (ec *ErrorCollection) Length() int {
	return (*queue.Queue)(ec.errors).Length()
}

// Add an error into collect
func (ec *ErrorCollection) Add(e error) {
	(*queue.Queue)(ec.errors).Push(e)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("catch a panic：%s\n", r)
		}
	}()
}

// This is a self design  function to handle the inner errors collected
func (ec *ErrorCollection) Handle(f ErrorHandler) {
	ec.M.Lock()
	ec.newAutoHandleChan()
	ec.CatchError()
	ec.M.Unlock()
	go func() {
		log.Println("handle routine starts,use ec.CloseHandles to stop")
	L:
		for {
			select {
			case <-ec.AutoHandleChan:
				log.Println("handle finish by auto handle chan")
				break L
			case e := <-ec.CatchErrorChan:
				f(e)
			}
		}
	}()
}

func (ec *ErrorCollection) safeNewAutoHandleChan() {
	ec.M.Lock()
	defer ec.M.Unlock()
	ec.AutoHandleChan = make(chan int, 1)
}
func (ec *ErrorCollection) newAutoHandleChan() {
	ec.AutoHandleChan = make(chan int, 1)
}

func (ec *ErrorCollection) CloseHandles() {
	ec.M.Lock()
	defer ec.M.Unlock()
	close(ec.AutoHandleChan)
}

// Handle the error queue one by one  by those handler added
// How to add a handler>
// ec.AddHandler(Logger(),Panic(),SendEmail()) ...
func (ec *ErrorCollection) HandleChain() {
	ec.newAutoHandleChan()
	ec.CatchError()
	go func() {
		log.Println("handleChain routine starts,use ec.CloseHandles to stop")
	L:
		for {
			select {
			case e := <-ec.CatchError():
				for _, f := range ec.ErrorHandleChain {
					f(e)
				}
			case <-ec.AutoHandleChan:
				log.Println("handleChain finish by auto handle chan")
				break L
			}
		}
	}()

}

// Add handler to handler chain
func (ec *ErrorCollection) AddHandler(handler ... ErrorHandler) {
	ec.M.Lock()
	defer ec.M.Unlock()
	ec.ErrorHandleChain = append(ec.ErrorHandleChain, handler...)
}

// Clear errors in collection
func (ec *ErrorCollection) Clear() {
	ec.M.Lock()
	defer ec.M.Unlock()
	ec.errors = ErrorBox(queue.NewCap(500))
}

// Pop an error.
// The popped error is from queue' head.
// When use Pop(), it means the error has been dealed and deleted from the queue
func (ec *ErrorCollection) Pop() error {
	q := (*queue.Queue)(ec.errors)
	ec.GetQueueLock().Lock()
	defer ec.GetQueueLock().Unlock()
	if ec.Length() > 0 {
		e := q.Pop().(error)
		return errorx.Wrap(e)
	}
	return nil
}

// Get an error
// The error is from queue' head.
// When use Get(), it means the error is only for query,not deleted from the queue
func (ec *ErrorCollection) GetError() error {
	q := (*queue.Queue)(ec.errors)
	if ec.Length() != 0 {
		h, _ := q.SafeValidHead()
		return errorx.Wrap(h.(error))
	}
	return nil
}

// When an error in , it will be pop into a chanel waiting for handling
func (ec *ErrorCollection) CatchError() <-chan error {
	go func() {
	L:
		for {
			select {
			case <-ec.AutoHandleChan:
				log.Println("close error catching by auto handle chan")
				break L
			default:
				ec.M.Lock()
				if ec.Length() > 0 {
					ec.CatchErrorChan <- ec.Pop()
				}
				ec.M.Unlock()
			}

		}
	}()
	return ec.CatchErrorChan
}

// When an error in , it will be passed into a chanel waiting for handling but the error remains in the collection
func (ec *ErrorCollection) HasError() <-chan error {
	go func() {
		for {
			if ec.Length() > 0 {
				ec.CatchErrorChan <- ec.GetError()
			}
		}
	}()
	return ec.HasErrorChan
}

//// When error in, it will be automatically handled by the added handlers
//func (ec *ErrorCollection) AutoHandle(){
//	for{
//		select{
//			case <-ec.AutoHandleChan:
//				log.Println("auto handle close successfully")
//				break
//			case e := <- ec.CatchError():
//
//			//TODO
//		}
//	}
//}

// Logger records error msg in log without modifying error queue.
func Logger() func(e error) {
	return func(e error) {
		log.SetFlags(log.LstdFlags | log.Llongfile)
		log.Println(e)
	}
}

// Panic records the first error and recover.
// Panic aims to fix the panic cause error and ignore other errors unless you fix the error up
func Panic() func(e error) {
	return func(e error) {
		defer func() {
			if r := recover(); r != nil {
				log.SetFlags(log.LstdFlags | log.Llongfile)
				log.Println("panic and recover,because of:", r)
			}
		}()
		panic(e.Error())

	}
}
