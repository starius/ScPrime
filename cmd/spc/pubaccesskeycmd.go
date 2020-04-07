package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
)

var (
	skykeyCmd = &cobra.Command{
		Use:   "pubaccesskey",
		Short: "Perform actions related to Pubaccesskeys",
		Long:  `Perform actions related to Pubaccesskeys, the encryption keys used for Pubfiles.`,
		Run:   skykeycmd,
	}

	skykeyCreateCmd = &cobra.Command{
		Use:   "create [name]",
		Short: "Create a pubaccesskey with the given name.",
		Long: `Create a pubaccesskey  with the given name. The --cipher-type flag can be
		used to specify the cipher type. Its default is XChaCha20.`,
		Run: wrap(skykeycreatecmd),
	}

	skykeyAddCmd = &cobra.Command{
		Use:   "add [pubaccesskey base64-encoded pubaccesskey]",
		Short: "Add a base64-encoded pubaccesskey to the key manager.",
		Long:  `Add a base64-encoded pubaccesskey to the key manager.`,
		Run:   wrap(skykeyaddcmd),
	}

	skykeyGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get the pubaccesskey by its name or id",
		Long:  `Get the base64-encoded pubaccesskey using either its name with --name or id with --id`,
		Run:   wrap(skykeygetcmd),
	}

	skykeyGetIDCmd = &cobra.Command{
		Use:   "get-id",
		Short: "Get the pubaccesskey id by its name",
		Long:  `Get the base64-encoded pubaccesskey id`,
		Run:   wrap(skykeygetidcmd),
	}
)

// skykeycmd displays the usage info for the command.
func skykeycmd(cmd *cobra.Command, args []string) {
	cmd.UsageFunc()(cmd)
	os.Exit(exitCodeUsage)
}

// skykeycreatecmd creates a new Pubaccesskey with the given name and cipher type
// as set by flag.
func skykeycreatecmd(name string) {
	var cipherType crypto.CipherType
	err := cipherType.FromString(skykeyCipherType)
	if err != nil {
		die("could not decode cipher-type:", err)
	}

	sk, err := httpClient.SkykeyCreateKeyPost(name, cipherType)
	if err != nil {
		die("could not create pubaccesskey:", err)
	}

	skykeyStr, err := sk.ToString()
	if err != nil {
		die("Could not print pubaccesskey string:", err)
	}
	fmt.Printf("Created new pubaccesskey: %v\n", skykeyStr)
}

// skykeyaddcmd adds the given pubaccesskey to the renter's pubaccesskey manager.
func skykeyaddcmd(skykeyString string) {
	var sk pubaccesskey.Pubaccesskey
	err := sk.FromString(skykeyString)
	if err != nil {
		die("Could not decode pubaccesskey string:", err)
	}

	err = httpClient.SkykeyAddKeyPost(sk)
	if err != nil {
		die("could not add pubaccesskey:", err)
	}

	fmt.Printf("Successfully added new pubaccesskey: %v\n", skykeyString)
}

// skykeygetcmd retrieves the pubaccesskey using a name or id flag.
func skykeygetcmd() {
	if skykeyName == "" && skykeyID == "" {
		die("Cannot get pubaccesskey without using --name or --id flag")
	}
	if skykeyName != "" && skykeyID != "" {
		die("Use only one flag to get the pubaccesskey: --name or --id flag")
	}

	var sk pubaccesskey.Pubaccesskey
	var err error
	if skykeyName != "" {
		sk, err = httpClient.SkykeyGetByName(skykeyName)
	} else {
		var id pubaccesskey.SkykeyID
		err = id.FromString(skykeyID)
		if err != nil {
			die("Could not decode pubaccesskey ID")
		}

		sk, err = httpClient.SkykeyGetByID(id)
	}

	if err != nil {
		die("Failed to retrieve pubaccesskey:", err)
	}

	skykeyStr, err := sk.ToString()
	if err != nil {
		die("Could not print pubaccesskey string:", err)
	}
	fmt.Printf("Found pubaccesskey: %v\n", skykeyStr)
}

// skykeygetidcmd retrieves the pubaccesskey id using its name.
func skykeygetidcmd(skykeyName string) {
	sk, err := httpClient.SkykeyGetByName(skykeyName)
	if err != nil {
		die("Failed to retrieve pubaccesskey:", err)
	}
	fmt.Printf("Found pubaccesskey ID: %v\n", sk.ID().ToString())
}
