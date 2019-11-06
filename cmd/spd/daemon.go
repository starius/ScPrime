package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	mnemonics "gitlab.com/NebulousLabs/entropy-mnemonics"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/SiaPrime/SiaPrime/build"
	fileConfig "gitlab.com/SiaPrime/SiaPrime/config"
	"gitlab.com/SiaPrime/SiaPrime/crypto"
	"gitlab.com/SiaPrime/SiaPrime/modules"
	"gitlab.com/SiaPrime/SiaPrime/node/api/server"
	"gitlab.com/SiaPrime/SiaPrime/profile"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
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
	validModules := "cghmrtwepsi"
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

// apiPassword discovers the API password, which may be specified in an
// environment variable, stored in a file on disk, or supplied by the user via
// stdin.
func apiPassword(siaDir string) (string, error) {
	// Check the environment variable.
	pw := os.Getenv("SCPRIME_API_PASSWORD")
	if pw != "" {
		fmt.Println("Using SCPRIME_API_PASSWORD environment variable")
		return pw, nil
	}

	pw = os.Getenv("SIAPRIME_API_PASSWORD")
	if pw != "" {
		fmt.Println("Warning: Using SIAPRIME_API_PASSWORD environment variable.")
		fmt.Println("Using it will not be supported in future versions, please update \n your configuration to use the environment variable 'SCPRIME_API_PASSWORD'")
		return pw, nil
	}

	// Try to read the password from disk.
	path := build.APIPasswordFile(siaDir)
	pwFile, err := ioutil.ReadFile(path)
	if err == nil {
		// This is the "normal" case, so don't print anything.
		return strings.TrimSpace(string(pwFile)), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	// No password file; generate a secure one.
	// Generate a password file.
	if err := os.MkdirAll(siaDir, 0700); err != nil {
		return "", err
	}
	pw = hex.EncodeToString(fastrand.Bytes(16))
	if err := ioutil.WriteFile(path, []byte(pw+"\n"), 0600); err != nil {
		return "", err
	}
	fmt.Println("A secure API password has been written to", path)
	fmt.Println("This password will be used automatically the next time you run spd.")
	return pw, nil
}

// loadAPIPassword determines whether to use an API password from disk or a
// temporary one entered by the user according to the provided config.
func loadAPIPassword(config Config, siaDir string) (_ Config, err error) {
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
			config.APIPassword, err = apiPassword(siaDir)
			if err != nil {
				return Config{}, err
			}
		}
	}
	return config, nil
}

// printVersionAndRevision prints the daemon's version and revision numbers.
func printVersionAndRevision() {
	fmt.Println("SiaPrime Daemon v" + build.Version)
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
	if password := os.Getenv("SCPRIME_WALLET_PASSWORD"); password != "" {
		fmt.Println("ScPrime Wallet Password found, attempting to auto-unlock wallet")
		if err := srv.Unlock(password); err != nil {
			fmt.Println("Auto-unlock failed:", err)
		} else {
			fmt.Println("Auto-unlock successful.")
		}
		return
	}
	if password := os.Getenv("SIAPRIME_WALLET_PASSWORD"); password != "" {
		fmt.Println("SiaPrime Wallet Password found, attempting to auto-unlock wallet")
		fmt.Println("Warning: Using SIAPRIME_WALLET_PASSWORD is deprecated.")
		fmt.Println("Using it will not be supported in future versions, please update \n your configuration to use the environment variable 'SCPRIME_WALLET_PASSWORD'")

		if err := srv.Unlock(password); err != nil {
			fmt.Println("Auto-unlock failed:", err)
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

	// Load API password.
	config, err = loadAPIPassword(config, build.DefaultSiaDir())
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
	// startDaemonCmd(cmd *cobra.Command, _ []string)
	nodeParams.PoolConfig = config.MiningPoolConfig

	// Start and run the server.
	srv, err := server.New(config.Spd.APIaddr, config.Spd.RequiredUserAgent, config.APIPassword, nodeParams)
	if err != nil {
		return err
	}

	// Attempt to auto-unlock the wallet using the env variable
	tryAutoUnlock(srv)

	// listen for kill signals
	sigChan := installKillSignalHandler()

	// Print a 'startup complete' message.
	startupTime := time.Since(loadStart)
	fmt.Println("Finished loading in", startupTime.Seconds(), "seconds")

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

	return nil
}

// startDaemonCmd is a passthrough function for startDaemon.
func startDaemonCmd(cmd *cobra.Command, _ []string) {
	var profileCPU, profileMem, profileTrace bool

	configErr := readFileConfig(globalConfig)
	if configErr != nil {
		fmt.Println("Configuration error: ", configErr.Error())
		os.Exit(exitCodeGeneral)
	}

	profileCPU = strings.Contains(globalConfig.Spd.Profile, "c")
	profileMem = strings.Contains(globalConfig.Spd.Profile, "m")
	profileTrace = strings.Contains(globalConfig.Spd.Profile, "t")

	if build.DEBUG {
		profileCPU = true
		profileMem = true
		profileTrace = true
	}

	if profileCPU || profileMem || profileTrace {
		var profileDir string
		if cmd.Root().Flag("profile-directory").Changed {
			profileDir = globalConfig.Spd.ProfileDir
		} else {
			profileDir = filepath.Join(globalConfig.Spd.SiaDir, globalConfig.Spd.ProfileDir)
		}
		go profile.StartContinuousProfile(profileDir, profileCPU, profileMem, profileTrace)
	}

	// Start spd. startDaemon will only return when it is shutting down.
	err := startDaemon(globalConfig)
	if err != nil {
		die(err)
	}

	// Daemon seems to have closed cleanly. Print a 'closed' mesasge.
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

// unlockWallet is called on spd startup and attempts to automatically
// unlock the wallet with the given password string.
func unlockWallet(w modules.Wallet, password string) error {
	// TODO: Check if this is the right place to have it!
	// NOTE: Look at the func tryAutoUnlock(srv *server.Server) in this file
	var validKeys []crypto.CipherKey
	dicts := []mnemonics.DictionaryID{"english", "german", "japanese"}
	for _, dict := range dicts {
		seed, err := modules.StringToSeed(password, dict)
		if err != nil {
			continue
		}
		validKeys = append(validKeys, crypto.NewWalletKey(crypto.HashObject(seed)))
	}
	validKeys = append(validKeys, crypto.NewWalletKey(crypto.HashObject(password)))
	for _, key := range validKeys {
		if err := w.Unlock(key); err == nil {
			return nil
		}
	}
	return modules.ErrBadEncryptionKey
}
