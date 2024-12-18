package goservicing_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dohernandez/goservicing"
)

type worker struct {
	sm sync.Mutex

	serve chan struct{}

	started bool
	closed  bool
}

func (w *worker) Run() error {
	w.sm.Lock()

	fmt.Println("worker started")

	w.started = true

	w.serve = make(chan struct{})

	w.sm.Unlock()

	<-w.serve

	return nil
}

func (w *worker) Stop() {
	w.sm.Lock()
	defer w.sm.Unlock()

	if w.closed || !w.started {
		fmt.Println("err: worker is not running")
	}

	fmt.Println("worker closed")

	w.closed = true

	close(w.serve)
}

func ExampleNewService() {
	w := &worker{}

	sg := &goservicing.ServiceGroup{}

	done := make(chan error, 1)

	go func() {
		err := sg.Start(
			context.Background(),
			time.Minute,
			func(_ context.Context, msg string) {
				fmt.Println(msg)
			},
			goservicing.NewService(
				"worker",
				func() error {
					return w.Run()
				},
				func() {
					w.Stop()
				},
			),
		)
		if err != nil {
			fmt.Println("err starting")
		}

		close(done)
	}()

	// wait for the service start
	select {
	case <-done:
		// Start failed with error
		fmt.Println("failed to start")

		return

	case <-time.After(time.Millisecond * 300):
		// Start successfully, proceed to close
		_ = sg.Close()
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		fmt.Println("failed to shutdown in reasonable time")
	}

	//nolint:dupword
	// Output:
	// start worker
	// worker started
	// worker closed
}
