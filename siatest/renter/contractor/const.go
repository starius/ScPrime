package contractor

import (
	"os"

	"gitlab.com/SiaPrime/SiaPrime/siatest"
)

// contractorTestDir creates a temporary testing directory for a contractor
// test. This should only every be called once per test. Otherwise it will
// delete the directory again.
func contractorTestDir(testName string) string {
	path := siatest.TestDir("renter/contractor", testName)
	if err := os.MkdirAll(path, 0777); err != nil {
		panic(err)
	}
	return path
}
