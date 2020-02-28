package main

import (
	"gitlab.com/SiaPrime/SiaPrime/analysis/jsontag"
	"gitlab.com/SiaPrime/SiaPrime/analysis/responsewritercheck"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(
		responsewritercheck.Analyzer,
		jsontag.Analyzer,
	)
}
