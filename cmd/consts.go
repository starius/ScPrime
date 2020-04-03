package cmd

var (
	// SiaAPIPassword is the environment variable that sets a custom API
	// password if the default is not used
	SiaAPIPassword = "SCPRIME_API_PASSWORD"

	// SiaDataDir is the environment variable that tells siad where to put the
	// sia data
	SiaDataDir = "SCPRIME_DATA_DIR"

	// SiaWalletPassword is the environment variable that can be set to enable
	// auto unlocking the wallet
	SiaWalletPassword = "SCPRIME_WALLET_PASSWORD"
)
