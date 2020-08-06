package renter

import (
	"bytes"
	"fmt"
	"net/url"
	"testing"

	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/node/api"
	"gitlab.com/scpcorp/ScPrime/node/api/client"
	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
	"gitlab.com/scpcorp/ScPrime/siatest"
)

// TestSkykey verifies the functionality of the Pubaccesskeys.
func TestSkykey(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// Create a testgroup.
	groupParams := siatest.GroupParams{
		Hosts:   3,
		Miners:  1,
		Renters: 1,
	}
	groupDir := renterTestDir(t.Name())

	// Specify subtests to run
	subTests := []siatest.SubTest{
		{Name: "AddSkykey", Test: testAddSkykey},
		{Name: "CreateSkykey", Test: testCreateSkykey},
		{Name: "DeleteSkykey", Test: testDeleteSkykey},
		{Name: "EncryptionTypePrivateID", Test: testSkynetEncryptionWithType(pubaccesskey.TypePrivateID)},
		{Name: "EncryptionTypePublicID", Test: testSkynetEncryptionWithType(pubaccesskey.TypePublicID)},
		{Name: "LargeFilePrivateID", Test: testSkynetEncryptionLargeFileWithType(pubaccesskey.TypePrivateID)},
		{Name: "LargeFilePublicID", Test: testSkynetEncryptionLargeFileWithType(pubaccesskey.TypePublicID)},
		{Name: "UnsafeClient", Test: testUnsafeClient},
	}

	// Run tests
	if err := siatest.RunSubTests(t, groupParams, groupDir, subTests); err != nil {
		t.Fatal(err)
	}
}

// testAddSkykey tests the Add functionality of the Pubaccesskey manager.
func testAddSkykey(t *testing.T, tg *siatest.TestGroup) {
	r := tg.Renters()[0]

	// The renter should be initialized with 0 pubaccesskeys.
	//
	// NOTE: This is order dependent. Since this is the first test run on the
	// renter we can test for this.
	pubaccesskeys, err := r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(pubaccesskeys) != 0 {
		t.Log(pubaccesskeys)
		t.Fatal("Expected 0 pubaccesskeys")
	}

	// Create a testkey from a hard-coded pubaccesskey string.
	testSkykeyString := "pubaccesskey:AbAc7Uz4NxBrVIzR2lY-LsVs3VWsuCA0D01jxYjaHdRwrfVUuo8DutiGD7OF1B1b3P1olWPXZO1X?name=hardcodedtestkey"
	var testSkykey pubaccesskey.Pubaccesskey
	err = testSkykey.FromString(testSkykeyString)
	if err != nil {
		t.Fatal(err)
	}

	// Add the pubaccesskey
	err = r.SkykeyAddKeyPost(testSkykey)
	if err != nil {
		t.Fatal(err)
	}

	// Check that the newly added pubaccesskey shows up.
	pubaccesskeys, err = r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(pubaccesskeys) != 1 {
		t.Log(pubaccesskeys)
		t.Fatal("Expected 1 pubaccesskey")
	}
	if pubaccesskeys[0].ID() != testSkykey.ID() || pubaccesskeys[0].Name != testSkykey.Name {
		t.Log(pubaccesskeys[0])
		t.Log(testSkykey)
		t.Fatal("Expected same pubaccesskey")
	}

	// Adding the same key should return an error.
	err = r.SkykeyAddKeyPost(testSkykey)
	if err == nil {
		t.Fatal("Expected error", err)
	}

	// Verify the pubaccesskey information through the API
	sk2, err := r.SkykeyGetByName(testSkykey.Name)
	if err != nil {
		t.Fatal(err)
	}
	skStr, err := testSkykey.ToString()
	if err != nil {
		t.Fatal(err)
	}
	sk2Str, err := sk2.ToString()
	if err != nil {
		t.Fatal(err)
	}
	if skStr != sk2Str {
		t.Fatal("Expected same Pubaccesskey string")
	}

	// Check byte equality and string equality.
	skID := testSkykey.ID()
	sk2ID := sk2.ID()
	if !bytes.Equal(skID[:], sk2ID[:]) {
		t.Fatal("Expected byte level equality in IDs")
	}
	if sk2.ID().ToString() != testSkykey.ID().ToString() {
		t.Fatal("Expected to get same key")
	}

	// Check the GetByID endpoint
	sk3, err := r.SkykeyGetByID(testSkykey.ID())
	if err != nil {
		t.Fatal(err)
	}
	sk3Str, err := sk3.ToString()
	if err != nil {
		t.Fatal(err)
	}
	if skStr != sk3Str {
		t.Fatal("Expected same Pubaccesskey string")
	}
}

// testCreateSkykey tests the Create functionality of the Pubaccesskey manager.
func testCreateSkykey(t *testing.T, tg *siatest.TestGroup) {
	r := tg.Renters()[0]

	// Check for any keys already with the renter
	pubaccesskeys, err := r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	numInitialKeys := len(pubaccesskeys)

	// Create a new pubaccesskey using the name of the test to avoid conflicts
	sk, err := r.SkykeyCreateKeyPost(t.Name(), pubaccesskey.TypePrivateID)
	if err != nil {
		t.Fatal(err)
	}
	totalKeys := numInitialKeys + 1

	// Check that the newly created pubaccesskey shows up.
	pubaccesskeys, err = r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(pubaccesskeys) != totalKeys {
		t.Log(pubaccesskeys)
		t.Fatalf("Expected %v pubaccesskeys, got %v", totalKeys, len(pubaccesskeys))
	}
	found := false
	for _, pubaccesskey := range pubaccesskeys {
		if pubaccesskey.ID() != sk.ID() && pubaccesskey.Name != sk.Name {
			found = true
			break
		}
	}
	if !found {
		siatest.PrintJSON(pubaccesskeys)
		t.Fatal("Pubaccesskey not found in pubaccesskeys")
	}

	// Creating the same key should return an error.
	_, err = r.SkykeyCreateKeyPost(t.Name(), pubaccesskey.TypePrivateID)
	if err == nil {
		t.Fatal("Expected error", err)
	}

	// Verify the pubaccesskey by getting by name
	sk2, err := r.SkykeyGetByName(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	skStr, err := sk.ToString()
	if err != nil {
		t.Fatal(err)
	}
	sk2Str, err := sk2.ToString()
	if err != nil {
		t.Fatal(err)
	}
	if skStr != sk2Str {
		t.Fatal("Expected same Pubaccesskey string")
	}

	// Check byte equality and string equality.
	skID := sk.ID()
	sk2ID := sk2.ID()
	if !bytes.Equal(skID[:], sk2ID[:]) {
		t.Fatal("Expected byte level equality in IDs")
	}
	if sk2.ID().ToString() != sk.ID().ToString() {
		t.Fatal("Expected to get same key")
	}

	// Check the GetByID endpoint
	sk3, err := r.SkykeyGetByID(sk.ID())
	if err != nil {
		t.Fatal(err)
	}
	sk3Str, err := sk3.ToString()
	if err != nil {
		t.Fatal(err)
	}
	if skStr != sk3Str {
		t.Fatal("Expected same Pubaccesskey string")
	}

	// Create a set with the strings of every pubaccesskey in the test.
	skykeySet := make(map[string]struct{})
	skykeySet[sk2Str] = struct{}{}
	for _, pubaccesskey := range pubaccesskeys {
		skykeyStr, err := pubaccesskey.ToString()
		if err != nil {
			t.Fatal(err)
		}
		skykeySet[skykeyStr] = struct{}{}
	}

	// Create a bunch of pubaccesskeys and check that they all get returned.
	nKeys := 10
	for i := 0; i < nKeys; i++ {
		nextSk, err := r.SkykeyCreateKeyPost(fmt.Sprintf(t.Name()+"-%d", i), pubaccesskey.TypePrivateID)
		if err != nil {
			t.Fatal(err)
		}
		nextSkStr, err := nextSk.ToString()
		if err != nil {
			t.Fatal(err)
		}
		skykeySet[nextSkStr] = struct{}{}
	}

	// Check that the expected number of keys was created.
	pubaccesskeys, err = r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	totalKeys += nKeys
	if len(pubaccesskeys) != totalKeys {
		t.Log(len(pubaccesskeys), totalKeys)
		t.Fatal("Wrong number of keys")
	}

	// Check that getting all the keys returns all the keys we just created.
	for _, skFromList := range pubaccesskeys {
		skStrFromList, err := skFromList.ToString()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := skykeySet[skStrFromList]; !ok {
			t.Log(skStrFromList, pubaccesskeys)
			t.Fatal("Didn't find key")
		}
	}
}

// testDeleteSkykey tests the delete functionality of the Pubaccesskey manager.
func testDeleteSkykey(t *testing.T, tg *siatest.TestGroup) {
	r := tg.Renters()[0]

	// Add any pubaccesskeys the renter already has to a map.
	pubaccesskeys, err := r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	skykeySet := make(map[string]struct{})
	for _, pubaccesskey := range pubaccesskeys {
		skykeyStr, err := pubaccesskey.ToString()
		if err != nil {
			t.Fatal(err)
		}
		skykeySet[skykeyStr] = struct{}{}
	}

	// Create a bunch of pubaccesskeys and check that they all get returned.
	nKeys := 10
	for i := 0; i < nKeys; i++ {
		nextSk, err := r.SkykeyCreateKeyPost(fmt.Sprintf(t.Name()+"-%d", i), pubaccesskey.TypePrivateID)
		if err != nil {
			t.Fatal(err)
		}
		nextSkStr, err := nextSk.ToString()
		if err != nil {
			t.Fatal(err)
		}
		skykeySet[nextSkStr] = struct{}{}
	}

	// Check that the expected number of keys was created.
	pubaccesskeys, err = r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(pubaccesskeys) != len(skykeySet) {
		t.Log(len(pubaccesskeys), len(skykeySet))
		t.Fatal("Wrong number of keys")
	}

	// Check that getting all the keys returns all the keys we just created.
	for _, skFromList := range pubaccesskeys {
		skStrFromList, err := skFromList.ToString()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := skykeySet[skStrFromList]; !ok {
			t.Log(skStrFromList, pubaccesskeys)
			t.Fatal("Didn't find key")
		}
	}

	// Test deletion endpoints by deleting half of the keys.
	pubaccesskeys, err = r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}

	deletedKeys := make(map[pubaccesskey.PubaccesskeyID]struct{})
	nKeys = len(pubaccesskeys)
	nToDelete := nKeys / 2
	for i, sk := range pubaccesskeys {
		if i >= nToDelete {
			break
		}

		if i%2 == 0 {
			err = r.SkykeyDeleteByNamePost(sk.Name)
		} else {
			err = r.SkykeyDeleteByIDPost(sk.ID())
		}
		if err != nil {
			t.Fatal(err)
		}
		deletedKeys[sk.ID()] = struct{}{}
	}

	// Check that the pubaccesskeys were deleted.
	pubaccesskeys, err = r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(pubaccesskeys) != nKeys-nToDelete {
		t.Fatalf("Expected %d keys, got %d", nKeys-nToDelete, len(pubaccesskeys))
	}

	// Sanity check: Make sure deleted keys are not still around.
	for _, sk := range pubaccesskeys {
		if _, ok := deletedKeys[sk.ID()]; ok {
			t.Fatal("Found a key that should have been deleted")
		}
	}
}

// testUnsafeClient tests the Pubaccesskey manager functionality using an unsafe
// client.
func testUnsafeClient(t *testing.T, tg *siatest.TestGroup) {
	r := tg.Renters()[0]

	// Check to make sure the renter has at least 1 pubaccesskey.
	pubaccesskeys, err := r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	if len(pubaccesskeys) == 0 {
		// Add a pubaccesskey
		_, err = r.SkykeyCreateKeyPost(persist.RandomSuffix(), pubaccesskey.TypePrivateID)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create a set with the strings of every pubaccesskey in the test.
	skykeySet := make(map[string]struct{})
	pubaccesskeys, err = r.SkykeySkykeysGet()
	if err != nil {
		t.Fatal(err)
	}
	for _, pubaccesskey := range pubaccesskeys {
		skykeyStr, err := pubaccesskey.ToString()
		if err != nil {
			t.Fatal(err)
		}
		skykeySet[skykeyStr] = struct{}{}
	}

	// Test misuse of the /skynet/pubaccesskey endpoint using an UnsafeClient.
	uc := client.NewUnsafeClient(r.Client)

	// Passing in 0 params shouild return an error.
	baseQuery := "/skynet/pubaccesskey"
	var skykeyGet api.SkykeyGET
	err = uc.Get(baseQuery, &skykeyGet)
	if err == nil {
		t.Fatal("Expected an error")
	}

	// Passing in 2 params shouild return an error.
	sk := pubaccesskeys[0]
	skID := sk.ID()
	values := url.Values{}
	values.Set("name", "testkey1")
	values.Set("id", skID.ToString())
	err = uc.Get(fmt.Sprintf("%s?%s", baseQuery, values.Encode()), &skykeyGet)
	if err == nil {
		t.Fatal("Expected an error")
	}

	// Sanity check: uc.Get should return the same value as the safe client
	// method.
	values = url.Values{}
	values.Set("name", sk.Name)
	err = uc.Get(fmt.Sprintf("%s?%s", baseQuery, values.Encode()), &skykeyGet)
	if err != nil {
		t.Fatal(err)
	}
	skStr, err := sk.ToString()
	if err != nil {
		t.Fatal(err)
	}
	if skykeyGet.Pubaccesskey != skStr {
		t.Fatal("Expected same result from  unsafe client")
	}

	// Use the unsafe client to check the Name and ID parameters are set in the
	// GET response.
	values = url.Values{}
	values.Set("name", sk.Name)
	getQuery := fmt.Sprintf("/skynet/pubaccesskey?%s", values.Encode())

	skykeyGet = api.SkykeyGET{}
	err = uc.Get(getQuery, &skykeyGet)
	if err != nil {
		t.Fatal(err)
	}
	if skykeyGet.Name != sk.Name {
		t.Log(skykeyGet)
		t.Fatal("Wrong pubaccesskey name")
	}
	if skykeyGet.ID != sk.ID().ToString() {
		t.Log(skykeyGet)
		t.Fatal("Wrong pubaccesskey ID")
	}
	if skykeyGet.Pubaccesskey != skStr {
		t.Log(skykeyGet)
		t.Fatal("Wrong pubaccesskey string")
	}

	// Check the Name and ID params from the /skynet/pubaccesskeys GET response.
	var skykeysGet api.SkykeysGET
	err = uc.Get("/skynet/pubaccesskeys", &skykeysGet)
	if err != nil {
		t.Fatal(err)
	}
	if len(skykeysGet.Pubaccesskeys) != len(skykeySet) {
		t.Fatalf("Got %d pubaccesskeys, expected %d", len(skykeysGet.Pubaccesskeys), len(skykeySet))
	}
	for _, skGet := range skykeysGet.Pubaccesskeys {
		if _, ok := skykeySet[skGet.Pubaccesskey]; !ok {
			t.Fatal("pubaccesskey not in test set")
		}

		var nextSk pubaccesskey.Pubaccesskey
		err = nextSk.FromString(skGet.Pubaccesskey)
		if err != nil {
			t.Fatal(err)
		}
		if nextSk.Name != skGet.Name {
			t.Fatal("Wrong pubaccesskey name")
		}
		if nextSk.ID().ToString() != skGet.ID {
			t.Fatal("Wrong pubaccesskey id")
		}
		if nextSk.Type.ToString() != skGet.Type {
			t.Fatal("Wrong pubaccesskey type")
		}
	}
}

// testSkynetEncryptionWithType returns the encryption test with the given
// skykeyType set.
func testSkynetEncryptionWithType(skykeyType pubaccesskey.SkykeyType) func(t *testing.T, tg *siatest.TestGroup) {
	return func(t *testing.T, tg *siatest.TestGroup) {
		testSkynetEncryption(t, tg, skykeyType)
	}
}

// testSkynetEncryptionLargeFileWithType returns the large-file encryption test with the given
// skykeyType.
func testSkynetEncryptionLargeFileWithType(skykeyType pubaccesskey.SkykeyType) func(t *testing.T, tg *siatest.TestGroup) {
	return func(t *testing.T, tg *siatest.TestGroup) {
		testSkynetEncryptionLargeFile(t, tg, skykeyType)
	}
}

// testSkynetEncryption tests the uploading and pinning of small skyfiles using
// encryption with the given skykeyType.
func testSkynetEncryption(t *testing.T, tg *siatest.TestGroup, skykeyType pubaccesskey.SkykeyType) {
	r := tg.Renters()[0]
	encKeyName := "encryption-test-key-" + skykeyType.ToString()

	// Create some data to upload as a skyfile.
	data := fastrand.Bytes(100 + siatest.Fuzz())
	// Need it to be a reader.
	reader := bytes.NewReader(data)
	// Call the upload skyfile client call.
	filename := "testEncryptSmall-" + skykeyType.ToString()
	uploadSiaPath, err := modules.NewSiaPath(filename)
	if err != nil {
		t.Fatal(err)
	}
	sup := modules.SkyfileUploadParameters{
		SiaPath:             uploadSiaPath,
		Force:               false,
		Root:                false,
		BaseChunkRedundancy: 2,
		FileMetadata: modules.SkyfileMetadata{
			Filename: filename,
			Mode:     0640, // Intentionally does not match any defaults.
		},
		Reader:     reader,
		SkykeyName: encKeyName,
	}

	_, _, err = r.SkynetSkyfilePost(sup)
	if err == nil {
		t.Fatal("Expected error for using unknown key")
	}

	// Try again after adding a key.
	// Note we must create a new reader in the params!
	sup.Reader = bytes.NewReader(data)

	_, err = r.SkykeyCreateKeyPost(encKeyName, skykeyType)
	if err != nil {
		t.Fatal(err)
	}

	skylink, sfMeta, err := r.SkynetSkyfilePost(sup)
	if err != nil {
		t.Fatal(err)
	}

	// Try to download the file behind the skylink.
	fetchedData, metadata, err := r.SkynetPublinkGet(sfMeta.Publink)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fetchedData, data) {
		t.Error("upload and download doesn't match")
		t.Log(data)
		t.Log(fetchedData)
	}
	if metadata.Mode != 0640 {
		t.Error("bad mode")
	}
	if metadata.Filename != filename {
		t.Error("bad filename")
	}

	if sfMeta.Publink != skylink {
		t.Log(sfMeta.Publink)
		t.Log(skylink)
		t.Fatal("Expected metadata skylink to match returned skylink")
	}

	// Pin the encrypted Skyfile.
	pinSiaPath, err := modules.NewSiaPath("testSmallEncryptedPinPath" + skykeyType.ToString())
	if err != nil {
		t.Fatal(err)
	}
	pinLUP := modules.SkyfilePinParameters{
		SiaPath:             pinSiaPath,
		Force:               false,
		Root:                false,
		BaseChunkRedundancy: 3,
	}
	err = r.SkynetPublinkPinPost(skylink, pinLUP)
	if err != nil {
		t.Fatal(err)
	}

	// See if the file is present.
	fullPinSiaPath, err := modules.SkynetFolder.Join(pinSiaPath.String())
	if err != nil {
		t.Fatal(err)
	}
	pinnedFile, err := r.RenterFileRootGet(fullPinSiaPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(pinnedFile.File.Publinks) != 1 {
		t.Fatal("expecting 1 skylink")
	}
	if pinnedFile.File.Publinks[0] != skylink {
		t.Fatal("skylink mismatch")
	}
}

// testSkynetEncryption tests the uploading and pinning of large skyfiles using
// encryption.
func testSkynetEncryptionLargeFile(t *testing.T, tg *siatest.TestGroup, skykeyType pubaccesskey.SkykeyType) {
	r := tg.Renters()[0]
	encKeyName := "large-file-encryption-test-key-" + skykeyType.ToString()

	// Create some data to upload as a skyfile.
	data := fastrand.Bytes(5 * int(modules.SectorSize))
	// Need it to be a reader.
	reader := bytes.NewReader(data)
	// Call the upload skyfile client call.
	filename := "testEncryptLarge-" + skykeyType.ToString()
	uploadSiaPath, err := modules.NewSiaPath(filename)
	if err != nil {
		t.Fatal(err)
	}
	sup := modules.SkyfileUploadParameters{
		SiaPath:             uploadSiaPath,
		Force:               false,
		Root:                false,
		BaseChunkRedundancy: 2,
		FileMetadata: modules.SkyfileMetadata{
			Filename: filename,
			Mode:     0640, // Intentionally does not match any defaults.
		},
		Reader:     reader,
		SkykeyName: encKeyName,
	}

	_, err = r.SkykeyCreateKeyPost(encKeyName, skykeyType)
	if err != nil {
		t.Fatal(err)
	}

	skylink, sfMeta, err := r.SkynetSkyfilePost(sup)
	if err != nil {
		t.Fatal(err)
	}

	if sfMeta.Publink != skylink {
		t.Log(sfMeta.Publink)
		t.Log(skylink)
		t.Fatal("Expected metadata skylink to match returned skylink")
	}

	// Try to download the file behind the skylink.
	fetchedData, metadata, err := r.SkynetPublinkGet(sfMeta.Publink)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fetchedData, data) {
		t.Error("upload and download doesn't match")
		t.Log(data)
		t.Log(fetchedData)
	}
	if metadata.Mode != 0640 {
		t.Error("bad mode")
	}
	if metadata.Filename != filename {
		t.Error("bad filename")
	}

	// Pin the encrypted Skyfile.
	pinSiaPath, err := modules.NewSiaPath("testEncryptedPinPath" + skykeyType.ToString())
	if err != nil {
		t.Fatal(err)
	}
	pinLUP := modules.SkyfilePinParameters{
		SiaPath:             pinSiaPath,
		Force:               false,
		Root:                false,
		BaseChunkRedundancy: 2,
	}
	err = r.SkynetPublinkPinPost(skylink, pinLUP)
	if err != nil {
		t.Fatal(err)
	}

	// See if the file is present.
	fullPinSiaPath, err := modules.SkynetFolder.Join(pinSiaPath.String())
	if err != nil {
		t.Fatal(err)
	}
	pinnedFile, err := r.RenterFileRootGet(fullPinSiaPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(pinnedFile.File.Publinks) != 1 {
		t.Fatal("expecting 1 skylink")
	}
	if pinnedFile.File.Publinks[0] != skylink {
		t.Fatal("skylink mismatch")
	}
}
