package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/node/api/server"
)

// randPassword returns a random 32 character long password.
func randPassword() string {
	b := make([]byte, 16) //32 characters long
	rand.Read(b)
	return hex.EncodeToString(b)
}

// printVersionAndRevision prints the daemon's version and revision numbers.
func printVersionAndRevision() {
	if build.DEBUG {
		fmt.Println("Running with debugging enabled")
	}
	switch build.Release {
	case "dev":
		fmt.Println("ScPrime-UI v" + build.Version + "-dev")
	case "standard":
		fmt.Println("ScPrime-UI v" + build.Version)
	case "testing":
		fmt.Println("ScPrime-UI v" + build.Version + "-testing")
	default:
		fmt.Println("ScPrime-UI v" + build.Version + "-???")
	}
	if build.GitRevision == "" {
		fmt.Println("WARN: compiled without build commit or version. To compile correctly, please use the makefile")
	} else {
		fmt.Println("Git Revision " + build.GitRevision)
	}
}

// installMmapSignalHandler installs a signal handler for Mmap related signals
// and exits when such a signal is received.
func installMmapSignalHandler() {
	// NOTE: ideally we would catch SIGSEGV here too, since that signal can
	// also be thrown by an mmap I/O error. However, SIGSEGV can occur under
	// other circumstances as well, and in those cases, we will want a full
	// stack trace.
	mmapChan := make(chan os.Signal, 1)
	signal.Notify(mmapChan, syscall.SIGBUS)
	go func() {
		<-mmapChan
		fmt.Println("A fatal I/O exception (SIGBUS) has occurred.")
		fmt.Println("Please check your disk for errors.")
		os.Exit(1)
	}()
}

// installKillSignalHandler installs a signal handler for os.Interrupt, os.Kill
// and syscall.SIGTERM and returns a channel that is closed when one of them is
// caught.
func installKillSignalHandler() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, os.Kill, syscall.SIGTERM)
	return sigChan
}

// startDaemon uses the config parameters to initialize modules and start ScPrime-UI.
func startDaemon() (err error) {
	// Record startup time
	loadStart := time.Now()

	// Print the Version and GitRevision
	printVersionAndRevision()

	// Install a signal handler that will catch exceptions thrown by mmap'd
	// files.
	installMmapSignalHandler()

	// Print a startup message.
	fmt.Println("Loading ScPrime-UI...")

	// configure the the node params.
	nodeParams := configNodeParams()

	// Launch the GUI
	go launch("http://" + nodeParams.APIaddr)

	// Start and run the server.
	srv, err := server.New(nodeParams.APIaddr, "ScPrime-Agent", randPassword(), nodeParams, loadStart)
	if err != nil {
		return err
	}

	// listen for kill signals
	sigChan := installKillSignalHandler()

	// Print a 'startup complete' message.
	startupTime := time.Since(loadStart)
	fmt.Printf("Finished full startup in %.3f seconds\n", startupTime.Seconds())

	// wait for Serve to return or for kill signal to be caught
	err = func() error {
		select {
		case err := <-srv.ServeErr():
			return err
		case <-sigChan:
			fmt.Println("\rCaught stop signal, quitting...")
			return srv.Close()
		}
	}()
	if err != nil {
		build.Critical(err)
	}

	// Wait for server to complete shutdown.
	srv.WaitClose()

	return nil
}
