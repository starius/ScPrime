package client

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"

	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/node/api"
	"gitlab.com/scpcorp/ScPrime/skykey"

	"gitlab.com/NebulousLabs/errors"
)

// SkynetSkylinkGet uses the /pubaccess/skylink endpoint to download a skylink
// file.
func (c *Client) SkynetSkylinkGet(skylink string) ([]byte, modules.SkyfileMetadata, error) {
	return c.SkynetSkylinkGetWithTimeout(skylink, -1)
}

// SkynetSkylinkGetWithTimeout uses the /pubaccess/skylink endpoint to download a
// skylink file, specifying the given timeout.
func (c *Client) SkynetSkylinkGetWithTimeout(skylink string, timeout int) ([]byte, modules.SkyfileMetadata, error) {
	values := url.Values{}
	// Only set the timeout if it's valid. Seeing as 0 is a valid timeout,
	// callers need to pass -1 to ignore it.
	if timeout >= 0 {
		values.Set("timeout", fmt.Sprintf("%d", timeout))
	}

	getQuery := fmt.Sprintf("/pubaccess/skylink/%s?%s", skylink, values.Encode())
	header, fileData, err := c.getRawResponse(getQuery)
	if err != nil {
		return nil, modules.SkyfileMetadata{}, errors.AddContext(err, "error fetching api response")
	}

	var sm modules.SkyfileMetadata
	strMetadata := header.Get("Pubaccess-File-Metadata")
	if strMetadata != "" {
		err = json.Unmarshal([]byte(strMetadata), &sm)
		if err != nil {
			return nil, modules.SkyfileMetadata{}, errors.AddContext(err, "unable to unmarshal pubfile metadata")
		}
	}
	return fileData, sm, errors.AddContext(err, "unable to fetch skylink data")
}

// SkynetSkylinkHead uses the /pubaccess/skylink endpoint to get the headers that
// are returned if the pubfile were to be requested using the SkynetSkylinkGet
// method.
func (c *Client) SkynetSkylinkHead(skylink string, timeout int) (int, http.Header, error) {
	getQuery := fmt.Sprintf("/pubaccess/skylink/%s?timeout=%d", skylink, timeout)
	return c.head(getQuery)
}

// SkynetSkylinkConcatGet uses the /pubaccess/skylink endpoint to download a
// skylink file with the 'concat' format specified.
func (c *Client) SkynetSkylinkConcatGet(skylink string) ([]byte, modules.SkyfileMetadata, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatConcat))
	getQuery := fmt.Sprintf("/pubaccess/skylink/%s?%s", skylink, values.Encode())
	var reader io.Reader
	header, body, err := c.getReaderResponse(getQuery)
	if err != nil {
		return nil, modules.SkyfileMetadata{}, errors.AddContext(err, "error fetching api response")
	}
	defer body.Close()
	reader = body

	// Read the fileData.
	fileData, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, modules.SkyfileMetadata{}, err
	}

	var sm modules.SkyfileMetadata
	strMetadata := header.Get("Pubaccess-File-Metadata")
	if strMetadata != "" {
		err = json.Unmarshal([]byte(strMetadata), &sm)
		if err != nil {
			return nil, modules.SkyfileMetadata{}, errors.AddContext(err, "unable to unmarshal pubfile metadata")
		}
	}
	return fileData, sm, errors.AddContext(err, "unable to fetch skylink data")
}

// SkynetSkylinkReaderGet uses the /pubaccess/skylink endpoint to fetch a reader of
// the file data.
func (c *Client) SkynetSkylinkReaderGet(skylink string) (io.ReadCloser, error) {
	getQuery := fmt.Sprintf("/pubaccess/skylink/%s", skylink)
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch skylink data")
}

// SkynetSkylinkConcatReaderGet uses the /pubaccess/skylink endpoint to fetch a
// reader of the file data with the 'concat' format specified.
func (c *Client) SkynetSkylinkConcatReaderGet(skylink string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatConcat))
	getQuery := fmt.Sprintf("/pubaccess/skylink/%s?%s", skylink, values.Encode())
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch skylink data")
}

// SkynetSkylinkTarReaderGet uses the /pubaccess/skylink endpoint to fetch a
// reader of the file data with the 'tar' format specified.
func (c *Client) SkynetSkylinkTarReaderGet(skylink string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatTar))
	getQuery := fmt.Sprintf("/pubaccess/skylink/%s?%s", skylink, values.Encode())
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch skylink data")
}

// SkynetSkylinkTarGzReaderGet uses the /pubaccess/skylink endpoint to fetch a
// reader of the file data with the 'targz' format specified.
func (c *Client) SkynetSkylinkTarGzReaderGet(skylink string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatTarGz))
	getQuery := fmt.Sprintf("/pubaccess/skylink/%s?%s", skylink, values.Encode())
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch skylink data")
}

// SkynetSkylinkPinPost uses the /pubaccess/pin endpoint to pin the file at the
// given skylink.
func (c *Client) SkynetSkylinkPinPost(skylink string, params modules.SkyfilePinParameters) error {
	return c.SkynetSkylinkPinPostWithTimeout(skylink, params, -1)
}

// SkynetSkylinkPinPostWithTimeout uses the /pubaccess/pin endpoint to pin the file
// at the given skylink, specifying the given timeout.
func (c *Client) SkynetSkylinkPinPostWithTimeout(skylink string, params modules.SkyfilePinParameters, timeout int) error {
	// Set the url values.
	values := url.Values{}
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)
	values.Set("siapath", params.SiaPath.String())
	values.Set("timeout", fmt.Sprintf("%d", timeout))

	query := fmt.Sprintf("/pubaccess/pin/%s?%s", skylink, values.Encode())
	_, _, err := c.postRawResponse(query, nil)
	if err != nil {
		return errors.AddContext(err, "post call to "+query+" failed")
	}
	return nil
}

// SkynetSkyfilePost uses the /pubaccess/pubfile endpoint to upload a pubfile.  The
// resulting skylink is returned along with an error.
func (c *Client) SkynetSkyfilePost(params modules.SkyfileUploadParameters) (string, api.SkynetSkyfileHandlerPOST, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", params.FileMetadata.Filename)
	dryRunStr := fmt.Sprintf("%t", params.DryRun)
	values.Set("dryrun", dryRunStr)
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	modeStr := fmt.Sprintf("%o", params.FileMetadata.Mode)
	values.Set("mode", modeStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", params.SiaPath.String(), values.Encode())
	_, resp, err := c.postRawResponse(query, params.Reader)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the skylink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the skylink upload response")
	}
	return rshp.Skylink, rshp, err
}

// SkynetSkyfilePostDisableForce uses the /pubaccess/pubfile endpoint to upload a
// pubfile. This method allows to set the Disable-Force header. The resulting
// skylink is returned along with an error.
func (c *Client) SkynetSkyfilePostDisableForce(params modules.SkyfileUploadParameters, disableForce bool) (string, api.SkynetSkyfileHandlerPOST, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", params.FileMetadata.Filename)
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	modeStr := fmt.Sprintf("%o", params.FileMetadata.Mode)
	values.Set("mode", modeStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)

	// Set the headers
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	if disableForce {
		headers["Pubaccess-Disable-Force"] = strconv.FormatBool(disableForce)
	}

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", params.SiaPath.String(), values.Encode())
	_, resp, err := c.postRawResponseWithHeaders(query, params.Reader, headers)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the skylink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the skylink upload response")
	}
	return rshp.Skylink, rshp, err
}

// SkynetSkyfileMultiPartPost uses the /pubaccess/pubfile endpoint to upload a
// pubfile using multipart form data.  The resulting skylink is returned along
// with an error.
func (c *Client) SkynetSkyfileMultiPartPost(params modules.SkyfileMultipartUploadParameters) (string, api.SkynetSkyfileHandlerPOST, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", params.Filename)
	forceStr := fmt.Sprintf("%t", params.Force)
	values.Set("force", forceStr)
	redundancyStr := fmt.Sprintf("%v", params.BaseChunkRedundancy)
	values.Set("basechunkredundancy", redundancyStr)
	rootStr := fmt.Sprintf("%t", params.Root)
	values.Set("root", rootStr)

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", params.SiaPath.String(), values.Encode())

	headers := map[string]string{"Content-Type": params.ContentType}
	_, resp, err := c.postRawResponseWithHeaders(query, params.Reader, headers)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the skylink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the skylink upload response")
	}
	return rshp.Skylink, rshp, err
}

// SkynetConvertSiafileToSkyfilePost uses the /pubaccess/pubfile endpoint to
// convert an existing siafile to a pubfile. The input SiaPath 'convert' is the
// siapath of the siafile that should be converted. The siapath provided inside
// of the upload params is the name that will be used for the base sector of the
// pubfile.
func (c *Client) SkynetConvertSiafileToSkyfilePost(lup modules.SkyfileUploadParameters, convert modules.SiaPath) (string, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("filename", lup.FileMetadata.Filename)
	forceStr := fmt.Sprintf("%t", lup.Force)
	values.Set("force", forceStr)
	modeStr := fmt.Sprintf("%o", lup.FileMetadata.Mode)
	values.Set("mode", modeStr)
	redundancyStr := fmt.Sprintf("%v", lup.BaseChunkRedundancy)
	values.Set("redundancy", redundancyStr)
	values.Set("convertpath", convert.String())

	// Make the call to upload the file.
	query := fmt.Sprintf("/pubaccess/pubfile/%s?%s", lup.SiaPath.String(), values.Encode())
	_, resp, err := c.postRawResponse(query, lup.Reader)
	if err != nil {
		return "", errors.AddContext(err, "post call to "+query+" failed")
	}

	// Parse the response to get the skylink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", errors.AddContext(err, "unable to parse the skylink upload response")
	}
	return rshp.Skylink, err
}

// SkynetBlacklistGet requests the /pubaccess/blacklist Get endpoint
func (c *Client) SkynetBlacklistGet() (blacklist api.SkynetBlacklistGET, err error) {
	err = c.get("/pubaccess/blacklist", &blacklist)
	return
}

// SkynetBlacklistPost requests the /pubaccess/blacklist Post endpoint
func (c *Client) SkynetBlacklistPost(additions, removals []string) (err error) {
	sbp := api.SkynetBlacklistPOST{
		Add:    additions,
		Remove: removals,
	}
	data, err := json.Marshal(sbp)
	if err != nil {
		return err
	}
	err = c.post("/pubaccess/blacklist", string(data), nil)
	return
}

// SkynetStatsGet requests the /pubaccess/stats Get endpoint
func (c *Client) SkynetStatsGet() (stats api.SkynetStatsGET, err error) {
	err = c.get("/pubaccess/stats", &stats)
	return
}

// SkykeyGetByName requests the /pubaccess/skykey Get endpoint using the key name.
func (c *Client) SkykeyGetByName(name string) (skykey.Skykey, error) {
	values := url.Values{}
	values.Set("name", name)
	getQuery := fmt.Sprintf("/pubaccess/skykey?%s", values.Encode())

	var skykeyGet api.SkykeyGET
	err := c.get(getQuery, &skykeyGet)
	if err != nil {
		return skykey.Skykey{}, err
	}

	var sk skykey.Skykey
	err = sk.FromString(skykeyGet.Skykey)
	if err != nil {
		return skykey.Skykey{}, err
	}

	return sk, nil
}

// SkykeyGetByID requests the /pubaccess/skykey Get endpoint using the key ID.
func (c *Client) SkykeyGetByID(id skykey.SkykeyID) (skykey.Skykey, error) {
	values := url.Values{}
	values.Set("id", id.ToString())
	getQuery := fmt.Sprintf("/pubaccess/skykey?%s", values.Encode())

	var skykeyGet api.SkykeyGET
	err := c.get(getQuery, &skykeyGet)
	if err != nil {
		return skykey.Skykey{}, err
	}

	var sk skykey.Skykey
	err = sk.FromString(skykeyGet.Skykey)
	if err != nil {
		return skykey.Skykey{}, err
	}

	return sk, nil
}

// SkykeyCreateKeyPost requests the /pubaccess/createskykey POST endpoint.
func (c *Client) SkykeyCreateKeyPost(name string, ct crypto.CipherType) (skykey.Skykey, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("name", name)
	values.Set("ciphertype", ct.String())

	var skykeyGet api.SkykeyGET
	err := c.post("/pubaccess/createskykey", values.Encode(), &skykeyGet)
	if err != nil {
		return skykey.Skykey{}, errors.AddContext(err, "createskykey POST request failed")
	}

	var sk skykey.Skykey
	err = sk.FromString(skykeyGet.Skykey)
	if err != nil {
		return skykey.Skykey{}, errors.AddContext(err, "failed to decode skykey string")
	}
	return sk, nil
}

// SkykeyAddKeyPost requests the /pubaccess/addskykey POST endpoint.
func (c *Client) SkykeyAddKeyPost(sk skykey.Skykey) error {
	values := url.Values{}
	skString, err := sk.ToString()
	if err != nil {
		return errors.AddContext(err, "failed to encode skykey as string")
	}
	values.Set("skykey", skString)

	err = c.post("/pubaccess/addskykey", values.Encode(), nil)
	if err != nil {
		return errors.AddContext(err, "addskykey POST request failed")
	}

	return nil
}
