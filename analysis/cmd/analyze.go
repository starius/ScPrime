package main

import (
	"gitlab.com/scpcorp/ScPrime/analysis/jsontag"
	"gitlab.com/scpcorp/ScPrime/analysis/responsewritercheck"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(
		responsewritercheck.Analyzer,
		jsontag.Analyzer,
	)
}
