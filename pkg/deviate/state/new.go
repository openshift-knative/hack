package state

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/openshift-knative/hack/pkg/deviate/log"
)

func New(log log.Logger) State {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stop
		log.Println("Signal detected, canceling...")
		cancel()
	}()
	return State{
		Context: ctx,
		Logger:  log,
		cancel:  cancel,
	}
}
