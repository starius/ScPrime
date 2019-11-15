package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"gitlab.com/SiaPrime/SiaPrime/build"
)

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the ScPrime daemon",
		Long:  "Stop the ScPrime daemon.",
		Run:   wrap(stopcmd),
	}

	updateCheckCmd = &cobra.Command{
		Use:   "check",
		Short: "Check for available updates",
		Long:  "Check for available updates.",
		Run:   wrap(updatecheckcmd),
	}

	updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Update ScPrime",
		Long:  "Check for (and/or download) available updates for ScPrime.",
		Run:   wrap(updatecmd),
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information.",
		Run:   wrap(versioncmd),
	}
)

// version prints the version of siac and siad.
func versioncmd() {
	fmt.Println("ScPrime Client")
	if build.ReleaseTag == "" {
		fmt.Println("\tVersion " + build.Version)
	} else {
		fmt.Println("\tVersion " + build.Version + "-" + build.ReleaseTag)
	}
	if build.GitRevision != "" {
		fmt.Println("\tGit Revision " + build.GitRevision)
		fmt.Println("\tBuild Time   " + build.BuildTime)
	}
	dvg, err := httpClient.DaemonVersionGet()
	if err != nil {
		fmt.Println("Could not get daemon version:", err)
		return
	}
	fmt.Println("ScPrime Daemon")
	fmt.Println("\tVersion " + dvg.Version)
	if build.GitRevision != "" {
		fmt.Println("\tGit Revision " + dvg.GitRevision)
		fmt.Println("\tBuild Time   " + dvg.BuildTime)
	}
}

// stopcmd is the handler for the command `spc stop`.
// Stops the daemon.
func stopcmd() {
	err := httpClient.DaemonStopGet()
	if err != nil {
		die("Could not stop daemon:", err)
	}
	fmt.Println("ScPrime daemon stopped.")
}

// updatecmd is the handler for the command `spc update`.
// Updates the daemon version to latest general release.
func updatecmd() {
	update, err := httpClient.DaemonUpdateGet()
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if !update.Available {
		fmt.Println("Already up to date.")
		return
	}

	err = httpClient.DaemonUpdatePost()
	if err != nil {
		fmt.Println("Could not apply update:", err)
		return
	}
	fmt.Printf("Updated to version %s! Restart spd now.\n", update.Version)
}

// updatecheckcmd is the handler for the command `siac check`.
// Checks is there is an newer daemon version available.
func updatecheckcmd() {
	update, err := httpClient.DaemonUpdateGet()
	if err != nil {
		fmt.Println("Could not check for update:", err)
		return
	}
	if update.Available {
		fmt.Printf("A new release (v%s) is available! Run 'spc update' to install it.\n", update.Version)
	} else {
		fmt.Println("Up to date.")
	}
}
