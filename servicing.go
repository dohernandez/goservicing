package goservicing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

// ErrShutdownDeadlineExceeded occurs when shutdown deadline exceeded.
var ErrShutdownDeadlineExceeded = errors.New("shutdown deadline exceeded while waiting for service to shutdown")

// Service is an interface used by ServiceGroup.
type Service interface {
	WithShutdownSignal(shutdown <-chan struct{}, done chan<- struct{})
	Start() error
	Name() string
}

// GracefulShutdownFunc function used to close all opened connections gracefully.
type GracefulShutdownFunc func(ctx context.Context)

// ServiceGroup manages services start and graceful shutdown synchronize.
type ServiceGroup struct {
	mutex  sync.Mutex
	closed bool

	sigint chan os.Signal

	gracefulShutdownFuncs []GracefulShutdownFunc
}

// WithGracefulShutDown returns a new ServiceGroup with GracefulShutdownFunc functions.
func WithGracefulShutDown(gracefulShutdownFuncs ...GracefulShutdownFunc) *ServiceGroup {
	return &ServiceGroup{
		gracefulShutdownFuncs: gracefulShutdownFuncs,
	}
}

// ErrSkipStart is a convenience error to skip service start.
var ErrSkipStart = errors.New("skip start")

// Start starts services synchronize and blocks until all services finishes by a notify signal.
//
// Returns error in case any of the services fail to starting or fail to shut down.
func (sg *ServiceGroup) Start(ctx context.Context, timeout time.Duration, log func(ctx context.Context, msg string), srvs ...Service) error {
	toShutdown := make(map[string]chan struct{})
	shutdownCh := make(chan struct{})

	sg.mutex.Lock()
	sg.sigint = make(chan os.Signal, 1)
	sg.mutex.Unlock()

	signal.Notify(sg.sigint, syscall.SIGTERM, syscall.SIGINT)

	g, ctx := errgroup.WithContext(ctx)

	for _, srv := range srvs {
		rsrv := srv

		shutdownDoneCh := make(chan struct{})
		toShutdown[srv.Name()] = shutdownDoneCh

		g.Go(func() error {
			if log != nil {
				log(ctx, sg.startMessage(rsrv))
			}

			var err error

			defer func() {
				if err != nil {
					log(ctx, sg.errorMessage(rsrv, err))

					return
				}
			}()

			rsrv.WithShutdownSignal(shutdownCh, shutdownDoneCh)

			err = rsrv.Start()

			return err
		})
	}

	done := make(chan error, 1)

	go sg.waitForSignal(ctx, shutdownCh, toShutdown, done, timeout)

	if err := g.Wait(); err != nil {
		return err
	}

	err := <-done

	return err
}

func (sg *ServiceGroup) startMessage(srv Service) string {
	msg := "start" + srv.Name()

	if x, ok := srv.(interface{ Addr() string }); ok {
		msg = fmt.Sprintf("%s server at addr %s", msg, x.Addr())
	}

	return msg
}

func (sg *ServiceGroup) errorMessage(srv Service, err error) string {
	msg := "failed to start" + srv.Name()

	if x, ok := srv.(interface{ Addr() string }); ok {
		msg = fmt.Sprintf("%s server at addr %s", msg, x.Addr())
	}

	msg = fmt.Sprintf("%s: %v", msg, err)

	return msg
}

func (sg *ServiceGroup) waitForSignal(
	ctx context.Context,
	shutdownCh chan struct{},
	toShutdown map[string]chan struct{},
	done chan error,
	timeout time.Duration,
) {
	<-sg.sigint

	signal.Stop(sg.sigint)

	defer func() {
		sg.mutex.Lock()
		defer sg.mutex.Unlock()

		close(sg.sigint)
	}()

	close(shutdownCh)

	deadline := time.After(timeout)

	for srv, shutdown := range toShutdown {
		select {
		case <-shutdown:
			continue
		case <-deadline:
			done <- fmt.Errorf("%w: %s", ErrShutdownDeadlineExceeded, srv)
		}
	}

	for _, shutdownFunc := range sg.gracefulShutdownFuncs {
		shutdownFunc(ctx)
	}

	close(done)
}

// Close invokes services to termination.
func (sg *ServiceGroup) Close() error {
	var err error

	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	if !sg.closed {
		sg.closed = true

		sg.sigint <- syscall.SIGINT
	}

	return err
}

type wrapService struct {
	name string

	startFunc func() error
	stopFunc  func()

	startFailed chan struct{}

	shutdownSignal <-chan struct{}
	shutdownDone   chan<- struct{}
}

// NewService creates new instance of Service.
//
// Example:
//
//	type Jop struct { ... }
//
//	// Run starts the job
//	func (j *Job) Run() error {...}
//
//	// Stop stops the job
//	func (j *Job) Stop() error {...}
//
//	j := &Job{}
//
//	sg := &ServiceGroup{}
//
//	err = servicing.Start(
//		context.Background(),
//		15*time.Second, // ttl is used during graceful shutdown.
//		func(ctx context.Context, msg string) {
//			log.Println(msg)
//		},
//		NewService(
//			"Job A",
//			func() error {
//				return j.Run()
//			},
//			func() error {
//				return j.Stop()
//			},
//		),
//	)
//	if err != nil {
//		panic(fmt.Errorf("failed to start servicing: %w", err))
//	}
func NewService(name string, startFunc func() error, stopFunc func()) Service {
	return &wrapService{
		name:        name,
		startFunc:   startFunc,
		stopFunc:    stopFunc,
		startFailed: make(chan struct{}),
	}
}

// Start starts serving the server.
func (wsrv *wrapService) Start() error {
	go wsrv.handleShutdown()

	err := wsrv.startFunc()
	if err != nil {
		return err
	}

	return nil
}

// handleShutdown will wait and handle shutdown signal that comes to the server
// and gracefully shutdown the server.
func (wsrv *wrapService) handleShutdown() {
	if wsrv.shutdownSignal == nil {
		return
	}

	select {
	case <-wsrv.shutdownSignal:
	case <-wsrv.startFailed:
		return
	}

	wsrv.stopFunc()

	close(wsrv.shutdownDone)
	close(wsrv.startFailed)
}

// WithShutdownSignal adds channels to wait for shutdown and to report shutdown finished.
func (wsrv *wrapService) WithShutdownSignal(shutdown <-chan struct{}, done chan<- struct{}) {
	wsrv.shutdownSignal = shutdown
	wsrv.shutdownDone = done
}

// Name Service name.
func (wsrv *wrapService) Name() string {
	return wsrv.name
}
