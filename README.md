# goservicing

[![Build Status](https://github.com/dohernandez/goservicing/workflows/test-unit/badge.svg)](https://github.com/dohernandez/goservicing/actions?query=branch%3Amain+workflow%3Atest)
[![GoDevDoc](https://img.shields.io/badge/dev-doc-00ADD8?logo=go)](https://pkg.go.dev/github.com/dohernandez/goservicing)
![Code lines](https://sloc.xyz/github/dohernandez/goservicing/?category=code)
![Comments](https://sloc.xyz/github/dohernandez/goservicing/?category=comments)

Package to manage services start and graceful shutdown synchronize.

### Example

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dohernandez/goservicing"
)

// service implementing the interface goservicing.Service
type service struct {
	// use mainly to log the start of the service and in case the failure shutdown.
	name string
	addr string

	shutdownSignal <-chan struct{}
	shutdownDone   chan<- struct{}

	srv *http.Server
}

func NewService() *service {
	return &service{
		name: "Service",
		addr: "8080",
	}
}

// WithShutdownSignal adds channels to wait for shutdown and to report shutdown finished.
func (s *service) WithShutdownSignal(shutdown <-chan struct{}, done chan<- struct{}) goservicing.Service {
	s.shutdownSignal = shutdown
	s.shutdownDone = done

	return s
}

// Name Service name.
func (s *service) Name() string {
	return s.name
}

// Addr service address.
func (s *service) Addr() string {
	return s.addr
}

// Start begins listening and serving.
func (s *service) Start() error {
	s.handleShutdown()

	router := http.NewServeMux()

	router.HandleFunc("/test", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		res.Write([]byte("Test is what we usually do"))
	})

	s.srv = &http.Server{
		Handler: router,
		Addr:    fmt.Sprintf(":%s", s.addr),
	}

	return s.srv.ListenAndServe()
}

// handleShutdown will handle the shutdown signal that comes to the server
// and shutdown the server.
func (s *service) handleShutdown() {
	if s.shutdownSignal == nil {
		return
	}

	go func() {
		<-s.shutdownSignal

		if err := s.srv.Shutdown(context.Background()); err != nil {
			_ = s.srv.Close() // nolint: errcheck
		}

		close(s.shutdownDone)
	}()
}

func main() {
	srv := NewService()

	servicing := &goservicing.ServiceGroup{}

	err := servicing.Start(
		context.Background(),
		time.Minute,
		func(ctx context.Context, msg string) {
			log.Println(msg)
		},
		srv,
	)

	log.Fatalln(err)
}

```