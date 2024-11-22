package goservicing_test

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/dohernandez/goservicing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type service struct {
	name string
	addr string

	err error

	shutdownSignal <-chan struct{}
	shutdownDone   chan<- struct{}

	WithShutdownSignalOK bool
	StartOK              bool
	ServiceShutdownOK    bool

	sleepOnShutdownFunc func()
}

func newService(err error) *service {
	return &service{
		name: "service",
		addr: "::0",
		err:  err,
	}
}

func (s *service) WithSleepOnShutdown(sleepOnShutdownFunc func()) *service {
	s.sleepOnShutdownFunc = sleepOnShutdownFunc

	return s
}

func (s *service) WithShutdownSignal(shutdown <-chan struct{}, done chan<- struct{}) goservicing.Service {
	s.shutdownSignal = shutdown
	s.shutdownDone = done

	s.WithShutdownSignalOK = true

	return s
}

func (s *service) Start() error {
	if s.err != nil {
		return s.err
	}

	s.StartOK = true

	s.handleShutdown()

	return nil
}

func (s *service) handleShutdown() {
	if s.shutdownSignal == nil {
		return
	}

	go func() {
		<-s.shutdownSignal

		if s.sleepOnShutdownFunc != nil {
			s.sleepOnShutdownFunc()

			// to avoid WARNING: DATA RACE during. The intent is to simulate that the shutdown is taking more than expected
			return
		}

		s.ServiceShutdownOK = true

		close(s.shutdownDone)
	}()
}

func (s *service) Name() string {
	return s.name
}

func (s *service) Addr() string {
	return s.addr
}

var errFailedStart = errors.New("service failed to start")

func TestServiceGroup_Start(t *testing.T) {
	type args struct {
		log func(ctx context.Context, msg string)
		srv goservicing.Service
	}

	tests := []struct {
		name        string
		args        args
		syscallKill syscall.Signal
		wantErr     bool
		err         error
	}{
		{
			name: "start successfully, killed SIGINT",
			args: args{
				log: func(_ context.Context, msg string) {
					assert.Equal(t, "start service server at addr ::0", msg)
				},
				srv: newService(nil),
			},
			syscallKill: syscall.SIGINT,
			wantErr:     false,
			err:         nil,
		},
		{
			name: "start successfully, killed SIGTERM",
			args: args{
				log: func(_ context.Context, msg string) {
					assert.Equal(t, "start service server at addr ::0", msg)
				},
				srv: newService(nil),
			},
			syscallKill: syscall.SIGTERM,
			wantErr:     false,
			err:         nil,
		},
		{
			name: "start failed",
			args: args{
				log: func(_ context.Context, msg string) {
					assert.Equal(t, "start service server at addr ::0", msg)
				},
				srv: newService(errFailedStart),
			},
			syscallKill: 0,
			wantErr:     true,
			err:         errFailedStart,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sg := &goservicing.ServiceGroup{}

			done := make(chan error, 1)

			go func() {
				err := sg.Start(context.Background(), time.Minute, tt.args.log, tt.args.srv)
				if (err != nil) != tt.wantErr {
					t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
				}

				assert.ErrorIsf(t, err, tt.err, "Start() error = %v, want %v", err, tt.err)

				close(done)
			}()

			srv := tt.args.srv.(*service) //nolint:errcheck

			// wait for the service start
			select {
			case <-done:
				// Start failed with error
				assert.Truef(t, srv.WithShutdownSignalOK, "Start() WithShutdownSignalOK got = %t, want true", srv.WithShutdownSignalOK)
				assert.Falsef(t, srv.StartOK, "Start() StartOK got = %t, want false", srv.StartOK)
				assert.Falsef(t, srv.ServiceShutdownOK, "Start() ServiceShutdownOK got = %t, want false", srv.ServiceShutdownOK)

				return
			case <-time.After(time.Second):
				// Start successfully, proceed to kill
				require.NoError(t, syscall.Kill(os.Getpid(), tt.syscallKill))
			}

			select {
			case <-done:
			case <-time.After(time.Second):
				assert.Fail(t, "Start() failed to shutdown in reasonable time")
			}

			assert.Truef(t, srv.WithShutdownSignalOK, "Start() WithShutdownSignalOK got = %t, want true", srv.WithShutdownSignalOK)
			assert.Truef(t, srv.StartOK, "Start() StartOK got = %t, want true", srv.StartOK)
			assert.Truef(t, srv.ServiceShutdownOK, "Start() ServiceShutdownOK got = %t, want true", srv.ServiceShutdownOK)
		})
	}
}

func TestServiceGroup_Start_timeout(t *testing.T) {
	srv := newService(nil).WithSleepOnShutdown(func() {
		time.Sleep(time.Second)
	})

	sg := &goservicing.ServiceGroup{}

	done := make(chan error, 1)

	go func() {
		err := sg.Start(context.Background(), time.Millisecond, nil, srv)
		assert.EqualError(t, err, "shutdown deadline exceeded while waiting for service service to shutdown")

		close(done)
	}()

	// wait for the service start
	select {
	case <-done:
		// Start failed with error
		assert.Fail(t, "failed to start")

		return
	case <-time.After(time.Second):
		// Start successfully, proceed to close
		sg.Close() //nolint:errcheck,gosec
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		assert.Fail(t, "Start_timeout() failed to shutdown in reasonable time")
	}

	assert.Truef(t, srv.WithShutdownSignalOK, "Start_timeout() WithShutdownSignalOK got = %t, want true", srv.WithShutdownSignalOK)
	assert.Truef(t, srv.StartOK, "Start_timeout() StartOK got = %t, want true", srv.StartOK)
	assert.Falsef(t, srv.ServiceShutdownOK, "Start_timeout() ServiceShutdownOK got = %t, want false", srv.ServiceShutdownOK)
}

func TestServiceGroup_Close(t *testing.T) {
	srv := newService(nil)

	sg := &goservicing.ServiceGroup{}

	done := make(chan error, 1)

	go func() {
		err := sg.Start(context.Background(), time.Minute, nil, srv)
		assert.NoError(t, err, "Close() got error = %v", err)

		close(done)
	}()

	// wait for the service start
	select {
	case <-done:
		// Start failed with error
		assert.Fail(t, "failed to start")

		return
	case <-time.After(time.Second):
		// Start successfully, proceed to close
		sg.Close() //nolint:errcheck,gosec
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		assert.Fail(t, "Close() failed to shutdown in reasonable time")
	}

	assert.Truef(t, srv.WithShutdownSignalOK, "Close() WithShutdownSignalOK got = %t, want true", srv.WithShutdownSignalOK)
	assert.Truef(t, srv.StartOK, "Close() StartOK got = %t, want true", srv.StartOK)
	assert.Truef(t, srv.ServiceShutdownOK, "Close() ServiceShutdownOK got = %t, want true", srv.ServiceShutdownOK)
}

func TestWithGracefulSutDown(t *testing.T) {
	srv := newService(nil)

	var gracefulShutdownOK bool

	gracefulShutdownFunc := func(_ context.Context) {
		gracefulShutdownOK = true
	}

	sg := goservicing.WithGracefulSutDown(gracefulShutdownFunc)

	done := make(chan error, 1)

	go func() {
		err := sg.Start(context.Background(), time.Minute, nil, srv)
		assert.NoError(t, err, "Close() got error = %v", err)

		close(done)
	}()

	// wait for the service start
	select {
	case <-done:
		// Start failed with error
		assert.Fail(t, "failed to start")

		return
	case <-time.After(time.Second):
		// Start successfully, proceed to close
		require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		assert.Fail(t, "failed to shutdown in reasonable time")
	}

	assert.Truef(t, gracefulShutdownOK, "Close() gracefulShutdownOK got = %t, want true", gracefulShutdownOK)
}
