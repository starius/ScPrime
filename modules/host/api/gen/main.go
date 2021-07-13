package main

import (
	"github.com/starius/api2"
	"gitlab.com/scpcorp/ScPrime/modules/host/api"
)

func main() {
	api2.GenerateClient(api.GetRoutes)
}
