package pool

import (
	"fmt"
	"net"
	"time"

	"github.com/sasha-s/go-deadlock"
	"gitlab.com/scpcorp/ScPrime/persist"
)

// Dispatcher contains a map of ip addresses to handlers
type Dispatcher struct {
	handlers          map[string]*Handler
	ln                net.Listener
	mu                deadlock.RWMutex
	p                 *Pool
	log               *persist.Logger
	connectionsOpened uint64
}

// NumConnections returns the number of open tcp connections
func (d *Dispatcher) NumConnections() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.handlers)
}

// NumConnectionsOpened returns the number of tcp connections that the pool
// has ever opened
func (d *Dispatcher) NumConnectionsOpened() uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connectionsOpened
}

// IncrementConnectionsOpened increments the number of tcp connections that the
// pool has ever opened
func (d *Dispatcher) IncrementConnectionsOpened() {
	// XXX: this is causing a deadlock
	/*
		d.mu.Lock()
		defer d.mu.Unlock()
		d.connectionsOpened += 1
	*/
}

// AddHandler connects the incoming connection to the handler which will handle it
func (d *Dispatcher) AddHandler(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	handler := &Handler{
		conn:   conn,
		closed: make(chan bool, 2),
		notify: make(chan bool, numPendingNotifies),
		p:      d.p,
		log:    d.log,
	}
	d.mu.Lock()
	d.handlers[addr] = handler
	d.mu.Unlock()

	// fmt.Printf("AddHandler listen() called: %s\n", addr)
	handler.Listen()

	<-handler.closed // when connection closed, remove handler from handlers
	d.mu.Lock()
	delete(d.handlers, addr)
	//fmt.Printf("Exiting AddHandler, %d connections remaining\n", len(d.handlers))
	d.mu.Unlock()
}

// ListenHandlers listens on a passed port and upon accepting the incoming connection,
// adds the handler to deal with it
func (d *Dispatcher) ListenHandlers(port string) {
	var err error
	err = d.p.tg.Add()
	if err != nil {
		// If this goroutine is not run before shutdown starts, this
		// codeblock is reachable.
		return
	}

	d.ln, err = net.Listen("tcp", ":"+port)
	if err != nil {
		d.log.Println(err)
		panic(err)
		// TODO: add error chan to report this
		//return
	}
	// fmt.Printf("Listening: %s\n", port)

	//safe close defer d.ln.Close()
	defer func() {
		e := d.ln.Close()
		if err == nil {
			err = e
		} else {
			if e != nil {
				err = fmt.Errorf("error %v on closing after %w", e.Error(), err)
			}
		}
	}()
	defer d.p.tg.Done()

	for {
		var conn net.Conn
		var err error
		select {
		case <-d.p.tg.StopChan():
			//fmt.Println("Closing listener")
			//d.ln.Close()
			//fmt.Println("Done closing listener")
			return
		default:
			conn, err = d.ln.Accept() // accept connection
			d.IncrementConnectionsOpened()
			if err != nil {
				d.log.Println(err)
				continue
			}
		}

		tcpconn := conn.(*net.TCPConn)
		tcpconn.SetKeepAlive(true)
		//tcpconn.SetKeepAlivePeriod(30 * time.Second)
		tcpconn.SetKeepAlivePeriod(15 * time.Second)
		tcpconn.SetNoDelay(true)
		// maybe this will help with our disconnection problems
		tcpconn.SetLinger(2)

		go d.AddHandler(conn)
	}
}

// NotifyClients tells the dispatcher to notify all clients that the block has
// changed
func (d *Dispatcher) NotifyClients() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.log.Printf("Notifying %d clients\n", len(d.handlers))
	for _, h := range d.handlers {
		h.notify <- true
	}
}

// ClearJobAndNotifyClients clear all stale jobs and tells the dispatcher to notify all clients that the block has
// changed
func (d *Dispatcher) ClearJobAndNotifyClients() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.log.Printf("Clear jobs and Notifying %d clients\n", len(d.handlers))
	for _, h := range d.handlers {
		if h != nil && h.s != nil {
			if h.s.CurrentWorker == nil {
				// this will happen when handler init, session init,
				// no mining.authorize happen yet, so worker is nil,
				// at this time, no stratum notify ever happen, no need to clear or notify
				d.log.Printf("Clear jobs and Notifying client: worker is nil\n")
				continue
			}
		} else {
			// this will happen when handler init, seesion is not
			d.log.Printf("Clear jobs and Notifying client: handler or session nil\n")
			continue
		}
		h.s.clearJobs()
		h.notify <- true
	}
}
