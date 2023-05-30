package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gitlab.com/NebulousLabs/errors"
	"golang.org/x/crypto/ssh/terminal"

	"gitlab.com/scpcorp/ScPrime/build"
	fileConfig "gitlab.com/scpcorp/ScPrime/config"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/node/api/server"
	"gitlab.com/scpcorp/ScPrime/profile"
)

// passwordPrompt securely reads a password from stdin.
func passwordPrompt(prompt string) (string, error) {
	fmt.Print(prompt)
	pw, err := terminal.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	return string(pw), err
}

// verifyAPISecurity checks that the security values are consistent with a
// sane, secure system.
func verifyAPISecurity(config Config) error {
	// Make sure that only the loopback address is allowed unless the
	// --disable-api-security flag has been used.
	if !config.Spd.AllowAPIBind {
		addr := modules.NetAddress(config.Spd.APIaddr)
		if !addr.IsLoopback() {
			if addr.Host() == "" {
				return fmt.Errorf("a blank host will listen on all interfaces, did you mean localhost:%v?\nyou must pass --disable-api-security to bind Spd to a non-localhost address", addr.Port())
			}
			return errors.New("you must pass --disable-api-security to bind Spd to a non-localhost address")
		}
		return nil
	}

	// If the --disable-api-security flag is used, enforce that
	// --authenticate-api must also be used.
	if config.Spd.AllowAPIBind && !config.Spd.AuthenticateAPI {
		return errors.New("cannot use --disable-api-security without setting an api password")
	}
	return nil
}

// processNetAddr adds a ':' to a bare integer, so that it is a proper port
// number.
func processNetAddr(addr string) string {
	_, err := strconv.Atoi(addr)
	if err == nil {
		return ":" + addr
	}
	return addr
}

// processModules makes the modules string lowercase to make checking if a
// module in the string easier, and returns an error if the string contains an
// invalid module character.
func processModules(modules string) (string, error) {
	modules = strings.ToLower(modules)
	validModules := "cghmrtwefpsi"
	invalidModules := modules
	for _, m := range validModules {
		invalidModules = strings.Replace(invalidModules, string(m), "", 1)
	}
	if len(invalidModules) > 0 {
		return "", errors.New("Unable to parse --modules flag, unrecognized or duplicate modules: " + invalidModules)
	}
	return modules, nil
}

// processProfileFlags checks that the flags given for profiling are valid.
func processProfileFlags(profile string) (string, error) {
	profile = strings.ToLower(profile)
	validProfiles := "cmt"

	invalidProfiles := profile
	for _, p := range validProfiles {
		invalidProfiles = strings.Replace(invalidProfiles, string(p), "", 1)
	}
	if len(invalidProfiles) > 0 {
		return "", errors.New("Unable to parse --profile flags, unrecognized or duplicate flags: " + invalidProfiles)
	}
	return profile, nil
}

// processConfig checks the configuration values and performs cleanup on
// incorrect-but-allowed values.
func processConfig(config Config) (Config, error) {
	var err1, err2 error
	config.Spd.APIaddr = processNetAddr(config.Spd.APIaddr)
	config.Spd.RPCaddr = processNetAddr(config.Spd.RPCaddr)
	config.Spd.HostAddr = processNetAddr(config.Spd.HostAddr)
	config.Spd.Modules, err1 = processModules(config.Spd.Modules)
	config.Spd.Profile, err2 = processProfileFlags(config.Spd.Profile)
	err3 := verifyAPISecurity(config)
	err := build.JoinErrors([]error{err1, err2, err3}, ", and ")
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

// loadAPIPassword determines whether to use an API password from disk or a
// temporary one entered by the user according to the provided config.
func loadAPIPassword(config Config) (_ Config, err error) {
	if config.Spd.AuthenticateAPI {
		if config.Spd.TempPassword {
			config.APIPassword, err = passwordPrompt("Enter API password: ")
			if err != nil {
				return Config{}, err
			} else if config.APIPassword == "" {
				return Config{}, errors.New("password cannot be blank")
			}
		} else {
			// load API password from environment variable or file.
			config.APIPassword, err = build.APIPassword()
			if err != nil {
				return Config{}, err
			}
		}
	}
	return config, nil
}

// printVersionAndRevision prints the daemon's version and revision numbers.
func printVersionAndRevision() {
	fmt.Println("ScPrime Daemon v" + build.Version)
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

// tryAutoUnlock will try to automatically unlock the server's wallet if the
// environment variable is set.
func tryAutoUnlock(srv *server.Server) {
	if password := build.WalletPassword(); password != "" {
		fmt.Println("ScPrime Wallet Password found, attempting to auto-unlock wallet")
		err := srv.Unlock(password)
		if err != nil {
			if err == modules.ErrAlreadyUnlocked {
				fmt.Println("Wallet unlocked.")
			} else {
				fmt.Println("Auto-unlock failed:", err)
			}
		} else {
			fmt.Println("Auto-unlock successful.")
		}
	}
}

// startDaemon uses the config parameters to initialize modules and start spd.
func startDaemon(config Config) (err error) {
	// Process the config variables after they are parsed by cobra.
	config, err = processConfig(config)
	if err != nil {
		return errors.AddContext(err, "failed to parse input parameter")
	}

	// Check for Metadata directory presence and try to migrate if it does not
	// exist before continuing with anything else

	// Load API password.
	config, err = loadAPIPassword(config)
	if err != nil {
		return errors.AddContext(err, "failed to get API password")
	}

	// Print the spd Version and GitRevision
	printVersionAndRevision()

	// Install a signal handler that will catch exceptions thrown by mmap'd
	// files.
	installMmapSignalHandler()

	// Print a startup message.
	fmt.Println("Loading spd...")
	loadStart := time.Now()

	// Create the node params by parsing the modules specified in the config.
	nodeParams := parseModules(config)

	// Add MiningPoolConfig previously read by readFileConfig(globalConfig) in
	// startDaemonCmd(cmd *cobra.Command, _ []string).
	nodeParams.PoolConfig = config.MiningPoolConfig

	// Start and run the server.
	srv, err := server.New(config.Spd.APIaddr, config.Spd.RequiredUserAgent, config.APIPassword, nodeParams, loadStart)
	if err != nil {
		return err
	}

	// Attempt to auto-unlock the wallet using the env variable
	tryAutoUnlock(srv)

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

// startDaemonCmd is a passthrough function for startDaemon.
func startDaemonCmd(cmd *cobra.Command, _ []string) {
	var profileCPU, profileMem, profileTrace bool

	profileCPU = strings.Contains(globalConfig.Spd.Profile, "c")
	profileMem = strings.Contains(globalConfig.Spd.Profile, "m")
	profileTrace = strings.Contains(globalConfig.Spd.Profile, "t")

	if build.DEBUG {
		profileCPU = true
		profileMem = true
	}

	if profileCPU || profileMem || profileTrace {
		var profileDir string
		if cmd.Root().Flag("profile-directory").Changed {
			profileDir = globalConfig.Spd.ProfileDir
		} else {
			profileDir = filepath.Join(globalConfig.Spd.DataDir, globalConfig.Spd.ProfileDir)
		}
		go profile.StartContinuousProfile(profileDir, profileCPU, profileMem, profileTrace)
	}

	configErr := readFileConfig(globalConfig)
	if configErr != nil {
		fmt.Println("Configuration error: ", configErr.Error())
		os.Exit(exitCodeGeneral)
	}

	// Start spd. startDaemon will only return when it is shutting down.
	err := startDaemon(globalConfig)
	if err != nil {
		die(err)
	}

	// Daemon seems to have closed cleanly. Print a 'closed' message.
	fmt.Println("Shutdown complete.")
}

func readFileConfig(config Config) error {
	viper.SetConfigType("yaml")
	viper.SetConfigName("siaprime")
	viper.AddConfigPath(".")

	if strings.Contains(config.Spd.Modules, "p") {
		err := viper.ReadInConfig() // Find and read the config file
		if err != nil {             // Handle errors reading the config file
			return err
		}
		poolViper := viper.Sub("miningpool")
		poolViper.SetDefault("name", "")
		poolViper.SetDefault("id", "")
		poolViper.SetDefault("acceptingcontracts", false)
		poolViper.SetDefault("operatorpercentage", 0.0)
		poolViper.SetDefault("operatorwallet", "")
		poolViper.SetDefault("networkport", 3355)
		poolViper.SetDefault("dbaddress", "127.0.0.1")
		poolViper.SetDefault("dbname", "miningpool")
		poolViper.SetDefault("dbport", "3306")
		if !poolViper.IsSet("poolwallet") {
			return errors.New("Must specify a poolwallet")
		}
		if !poolViper.IsSet("dbuser") {
			return errors.New("Must specify a dbuser")
		}
		if !poolViper.IsSet("dbpass") {
			return errors.New("Must specify a dbpass")
		}
		dbUser := poolViper.GetString("dbuser")
		dbPass := poolViper.GetString("dbpass")
		dbAddress := poolViper.GetString("dbaddress")
		dbPort := poolViper.GetString("dbport")
		dbName := poolViper.GetString("dbname")
		dbConnection := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPass, dbAddress, dbPort, dbName)
		if poolViper.IsSet("dbsocket") {
			dbSocket := poolViper.GetString("dbsocket")
			dbConnection = fmt.Sprintf("%s:%s@unix(%s)/%s", dbUser, dbPass, dbSocket, dbName)
		}
		poolConfig := fileConfig.MiningPoolConfig{
			PoolNetworkPort:  int(poolViper.GetInt("networkport")),
			PoolName:         poolViper.GetString("name"),
			PoolID:           uint64(poolViper.GetInt("id")),
			PoolDBConnection: dbConnection,
			PoolWallet:       poolViper.GetString("poolwallet"),
		}
		globalConfig.MiningPoolConfig = poolConfig
	}
	if strings.Contains(config.Spd.Modules, "i") {
		err := viper.ReadInConfig() // Find and read the config file
		if err != nil {             // Handle errors reading the config file
			return err
		}
		poolViper := viper.Sub("index")
		poolViper.SetDefault("dbaddress", "127.0.0.1")
		poolViper.SetDefault("dbname", "siablocks")
		poolViper.SetDefault("dbport", "3306")
		if !poolViper.IsSet("dbuser") {
			return errors.New("Must specify a dbuser")
		}
		if !poolViper.IsSet("dbpass") {
			return errors.New("Must specify a dbpass")
		}
		dbUser := poolViper.GetString("dbuser")
		dbPass := poolViper.GetString("dbpass")
		dbAddress := poolViper.GetString("dbaddress")
		dbPort := poolViper.GetString("dbport")
		dbName := poolViper.GetString("dbname")
		dbConnection := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPass, dbAddress, dbPort, dbName)
		globalConfig.IndexConfig = fileConfig.IndexConfig{
			PoolDBConnection: dbConnection,
		}
	}
	return nil
}
