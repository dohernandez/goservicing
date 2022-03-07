package goservicing

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bool64/ctxd"
	"golang.org/x/sync/errgroup"
)

// Service is an interface used by ServiceGroup.
type Service interface {
	WithShutdownSignal(shutdown <-chan struct{}, done chan<- struct{}) Service
	Start() error
	Name() string
	Addr() string
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

// WithGracefulSutDown returns a new ServiceGroup with GracefulShutdownFunc functions.
func WithGracefulSutDown(gracefulShutdownFuncs ...GracefulShutdownFunc) *ServiceGroup {
	return &ServiceGroup{
		gracefulShutdownFuncs: gracefulShutdownFuncs,
	}
}

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
				log(ctx, fmt.Sprintf("start %s server at addr %s", rsrv.Name(), rsrv.Addr()))
			}

			return rsrv.WithShutdownSignal(shutdownCh, shutdownDoneCh).Start()
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

func (sg *ServiceGroup) waitForSignal(
	ctx context.Context,
	shutdownCh chan struct{},
	toShutdown map[string]chan struct{},
	done chan error,
	timeout time.Duration,
) {
	<-sg.sigint

	signal.Stop(sg.sigint)

	close(shutdownCh)

	deadline := time.After(timeout)

	for srv, shutdown := range toShutdown {
		select {
		case <-shutdown:
			continue
		case <-deadline:
			done <- ctxd.NewError(ctx, fmt.Sprintf("shutdown deadline exceeded while waiting for service %s to shutdown", srv))
		}
	}

	for _, shutdownFunc := range sg.gracefulShutdownFuncs {
		shutdownFunc(ctx)
	}

	close(done)
}

// Close invokes services to termination.
func (sg *ServiceGroup) Close() (err error) {
	sg.mutex.Lock()
	defer sg.mutex.Unlock()

	if !sg.closed {
		sg.closed = true
		close(sg.sigint)
	}

	return err
}
