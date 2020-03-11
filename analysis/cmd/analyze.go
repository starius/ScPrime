package main

import (
	"gitlab.com/scpcorp/ScPrime/analysis/jsontag"
	"gitlab.com/scpcorp/ScPrime/analysis/lockcheck"
	"gitlab.com/scpcorp/ScPrime/analysis/responsewritercheck"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	multichecker.Main(
		lockcheck.Analyzer,
		responsewritercheck.Analyzer,
		jsontag.Analyzer,
	)
}
