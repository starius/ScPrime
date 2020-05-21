package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/node/api/client"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
)

var (
	skykeyCmd = &cobra.Command{
		Use:   "pubaccesskey",
		Short: "Perform actions related to Pubaccesskeys",
		Long:  `Perform actions related to Pubaccesskeys, the encryption keys used for Skyfiles.`,
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
		Use:   "get-id [name]",
		Short: "Get the pubaccesskey id by its name",
		Long:  `Get the base64-encoded pubaccesskey id by its name`,
		Run:   wrap(skykeygetidcmd),
	}
)

// skykeycmd displays the usage info for the command.
func skykeycmd(cmd *cobra.Command, args []string) {
	cmd.UsageFunc()(cmd)
	os.Exit(exitCodeUsage)
}

// skykeycreatecmd is a wrapper for skykeyCreate used to handle pubaccesskey creation.
func skykeycreatecmd(name string) {
	skykeyStr, err := skykeyCreate(httpClient, name)
	if err != nil {
		die(errors.AddContext(err, "Failed to create new pubaccesskey"))
	}
	fmt.Printf("Created new pubaccesskey: %v\n", skykeyStr)
}

// skykeyCreate creates a new pubaccesskey with the given name and cipher type
// as set by flag.
func skykeyCreate(c client.Client, name string) (string, error) {
	var cipherType crypto.CipherType
	err := cipherType.FromString(skykeyCipherType)
	if err != nil {
		return "", errors.AddContext(err, "Could not decode cipher-type")
	}

	sk, err := c.SkykeyCreateKeyPost(name, cipherType)
	if err != nil {
		return "", errors.AddContext(err, "Could not create pubaccesskey")
	}
	return sk.ToString()
}

// skykeyaddcmd is a wrapper for skykeyAdd used to handle the addition of new skykeys.
func skykeyaddcmd(skykeyString string) {
	err := skykeyAdd(httpClient, skykeyString)
	if err != nil && strings.Contains(err.Error(), pubaccesskey.ErrSkykeyWithNameAlreadyExists.Error()) {
		die("Pubaccesskey name already used. Try using the --rename-as parameter with a different name.")
	}
	if err != nil {
		die(errors.AddContext(err, "Failed to add pubaccesskey"))
	}

	fmt.Printf("Successfully added new pubaccesskey: %v\n", skykeyString)
}

// skykeyAdd adds the given pubaccesskey to the renter's pubaccesskey manager.
func skykeyAdd(c client.Client, skykeyString string) error {
	var sk pubaccesskey.Pubaccesskey
	err := sk.FromString(skykeyString)
	if err != nil {
		return errors.AddContext(err, "Could not decode pubaccesskey string")
	}

	// Rename the pubaccesskey if the --rename-as flag was provided.
	if skykeyRenameAs != "" {
		sk.Name = skykeyRenameAs
	}

	err = c.SkykeyAddKeyPost(sk)
	if err != nil {
		return errors.AddContext(err, "Could not add pubaccesskey")
	}

	return nil
}

// skykeygetcmd is a wrapper for skykeyGet that handles pubaccesskey get commands.
func skykeygetcmd() {
	skykeyStr, err := skykeyGet(httpClient, skykeyName, skykeyID)
	if err != nil {
		die(err)
	}

	fmt.Printf("Found pubaccesskey: %v\n", skykeyStr)
}

// skykeyGet retrieves the pubaccesskey using a name or id flag.
func skykeyGet(c client.Client, name, id string) (string, error) {
	if name == "" && id == "" {
		return "", errors.New("Cannot get pubaccesskey without using --name or --id flag")
	}
	if name != "" && id != "" {
		return "", errors.New("Use only one flag to get the pubaccesskey: --name or --id flag")
	}

	var sk pubaccesskey.Pubaccesskey
	var err error
	if name != "" {
		sk, err = c.SkykeyGetByName(name)
	} else {
		var skykeyID pubaccesskey.SkykeyID
		err = skykeyID.FromString(id)
		if err != nil {
			return "", errors.AddContext(err, "Could not decode pubaccesskey ID")
		}

		sk, err = c.SkykeyGetByID(skykeyID)
	}

	if err != nil {
		return "", errors.AddContext(err, "Failed to retrieve pubaccesskey")
	}

	return sk.ToString()
}

// skykeygetidcmd retrieves the pubaccesskey id using its name.
func skykeygetidcmd(skykeyName string) {
	sk, err := httpClient.SkykeyGetByName(skykeyName)
	if err != nil {
		die("Failed to retrieve pubaccesskey:", err)
	}
	fmt.Printf("Found pubaccesskey ID: %v\n", sk.ID().ToString())
}
