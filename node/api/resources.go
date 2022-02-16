package api

import (
	_ "embed" // blank import is a compile-time dependency
)

//go:embed resources/logo.png
var logo []byte

//go:embed resources/favicon.ico
var favicon []byte

//go:embed resources/styles.css
var cssStyleSheet []byte

//go:embed resources/scripts.js
var javascript []byte

//go:embed resources/downloading.html
var downloadingHtml string

//go:embed resources/loading.html
var loadingHtml []byte

//go:embed resources/wallet_template.html
var walletHtmlTemplate string

//go:embed resources/alert_template.html
var alertHtmlTemplate string

//go:embed resources/transaction_templates/history_line_template.html
var transactionHistoryLineHtmlTemplate string

//go:embed resources/transaction_templates/history_template.html
var transactionsHistoryHtmlTemplate string

//go:embed resources/transaction_templates/info_template.html
var transactionInfoTemplate string

//go:embed resources/transaction_templates/input_template.html
var transactionInputTemplate string

//go:embed resources/transaction_templates/output_template.html
var transactionOutputTemplate string

//go:embed resources/transaction_templates/transaction_pagination_template.html
var transactionPaginationTemplate string

//go:embed resources/forms/close_alert.html
var closeAlertForm string

//go:embed resources/forms/initialize_seed.html
var intializeSeedForm string

//go:embed resources/forms/initialize_wallet.html
var initializeWalletForm string

//go:embed resources/forms/restore_from_seed.html
var restoreFromSeedForm string

//go:embed resources/forms/scanning_wallet.html
var scanningWalletForm string

//go:embed resources/forms/send_coins.html
var sendCoinsForm string

//go:embed resources/forms/unlock_wallet.html
var unlockWalletForm string

//go:embed resources/forms/change_lock.html
var changeLockForm string

//go:embed resources/forms/collapsed_menu.html
var collapsedMenuForm string

//go:embed resources/forms/expanded_menu.html
var expandedMenuForm string

//go:embed resources/fonts/open-sans-v27-latin/open-sans-v27-latin-regular.woff2
var openSansLatinRegularWoff2 []byte

//go:embed resources/fonts/open-sans-v27-latin/open-sans-v27-latin-700.woff2
var openSansLatin700Woff2 []byte

// Logo returns the Logo.
func (api *API) Logo() []byte {
	return logo
}

// Favicon returns the favicon.
func (api *API) Favicon() []byte {
	return favicon
}

// CssStyleSheet returns the css style sheet.
func (api *API) CssStyleSheet() []byte {
	return cssStyleSheet
}

// Javascript returns the javascript.
func (api *API) Javascript() []byte {
	return javascript
}

// DownloadingHtml returns an html page
func (api *API) DownloadingHtml() string {
	return downloadingHtml
}

// LoadingHtml returns an html page
func (api *API) LoadingHtml() []byte {
	return loadingHtml
}

// WalletHtmlTemplate returns the wallet html template
func (api *API) WalletHtmlTemplate() string {
	return walletHtmlTemplate
}

// AlertHtmlTemplate returns the alert html template
func (api *API) AlertHtmlTemplate() string {
	return alertHtmlTemplate
}

// TransactionHistoryLineHtmlTemplate returns an HTML template
func (api *API) TransactionHistoryLineHtmlTemplate() string {
	return transactionHistoryLineHtmlTemplate
}

// TransactionsHistoryHtmlTemplate returns an HTML template
func (api *API) TransactionsHistoryHtmlTemplate() string {
	return transactionsHistoryHtmlTemplate
}

// TransactionInfoTemplate returns an HTML template
func (api *API) TransactionInfoTemplate() string {
	return transactionInfoTemplate
}

// TransactionInputTemplate returns an HTML template
func (api *API) TransactionInputTemplate() string {
	return transactionInputTemplate
}

// TransactionOutputTemplate returns an HTML template
func (api *API) TransactionOutputTemplate() string {
	return transactionOutputTemplate
}

// TransactionPaginationTemplate returns an HTML template
func (api *API) TransactionPaginationTemplate() string {
	return transactionPaginationTemplate
}

// CloseAlertForm returns the close alert form
func (api *API) CloseAlertForm() string {
	return closeAlertForm
}

// IntializeSeedForm returns the initialize seed form
func (api *API) IntializeSeedForm() string {
	return intializeSeedForm
}

// InitializeWalletForm returns the initialize wallet form
func (api *API) InitializeWalletForm() string {
	return initializeWalletForm
}

// RestoreFromSeedForm returns the restore from seed form
func (api *API) RestoreFromSeedForm() string {
	return restoreFromSeedForm
}

// ScanningWalletForm returns the scanning wallet alert form
func (api *API) ScanningWalletForm() string {
	return scanningWalletForm
}

// SendCoinsForm returns the send coins form
func (api *API) SendCoinsForm() string {
	return sendCoinsForm
}

// UnlockWalletForm returns the unlock wallet form
func (api *API) UnlockWalletForm() string {
	return unlockWalletForm
}

// ChangeLockForm returns the change lock form
func (api *API) ChangeLockForm() string {
	return changeLockForm
}

// ExpandedMenuForm returns the HTML form
func (api *API) ExpandedMenuForm() string {
	return expandedMenuForm
}

// CollapsedMenuForm returns the HTML form
func (api *API) CollapsedMenuForm() string {
	return collapsedMenuForm
}

// OpenSansLatinRegularWoff2 returns the open sans font
func (api *API) OpenSansLatinRegularWoff2() []byte {
	return openSansLatinRegularWoff2
}

// OpenSansLatin700Woff2 returns the open sans font
func (api *API) OpenSansLatin700Woff2() []byte {
	return openSansLatin700Woff2
}
