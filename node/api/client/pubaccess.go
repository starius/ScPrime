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
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"

	"gitlab.com/NebulousLabs/errors"
)

// SkynetPublinkGet uses the /pubaccess/publink endpoint to download a publink
// file.
func (c *Client) SkynetPublinkGet(publink string) ([]byte, modules.SkyfileMetadata, error) {
	return c.SkynetPublinkGetWithTimeout(publink, -1)
}

// SkynetPublinkGetWithTimeout uses the /pubaccess/publink endpoint to download a
// publink file, specifying the given timeout.
func (c *Client) SkynetPublinkGetWithTimeout(publink string, timeout int) ([]byte, modules.SkyfileMetadata, error) {
	values := url.Values{}
	// Only set the timeout if it's valid. Seeing as 0 is a valid timeout,
	// callers need to pass -1 to ignore it.
	if timeout >= 0 {
		values.Set("timeout", fmt.Sprintf("%d", timeout))
	}

	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
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
	return fileData, sm, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkHead uses the /pubaccess/publink endpoint to get the headers that
// are returned if the pubfile were to be requested using the SkynetPublinkGet
// method.
func (c *Client) SkynetPublinkHead(publink string, timeout int) (int, http.Header, error) {
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?timeout=%d", publink, timeout)
	return c.head(getQuery)
}

// SkynetPublinkConcatGet uses the /pubaccess/publink endpoint to download a
// publink file with the 'concat' format specified.
func (c *Client) SkynetPublinkConcatGet(publink string) ([]byte, modules.SkyfileMetadata, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatConcat))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
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
	return fileData, sm, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkReaderGet uses the /pubaccess/publink endpoint to fetch a reader of
// the file data.
func (c *Client) SkynetPublinkReaderGet(publink string) (io.ReadCloser, error) {
	getQuery := fmt.Sprintf("/pubaccess/publink/%s", publink)
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkConcatReaderGet uses the /pubaccess/publink endpoint to fetch a
// reader of the file data with the 'concat' format specified.
func (c *Client) SkynetPublinkConcatReaderGet(publink string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatConcat))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkTarReaderGet uses the /pubaccess/publink endpoint to fetch a
// reader of the file data with the 'tar' format specified.
func (c *Client) SkynetPublinkTarReaderGet(publink string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatTar))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkTarGzReaderGet uses the /pubaccess/publink endpoint to fetch a
// reader of the file data with the 'targz' format specified.
func (c *Client) SkynetPublinkTarGzReaderGet(publink string) (io.ReadCloser, error) {
	values := url.Values{}
	values.Set("format", string(modules.SkyfileFormatTarGz))
	getQuery := fmt.Sprintf("/pubaccess/publink/%s?%s", publink, values.Encode())
	_, reader, err := c.getReaderResponse(getQuery)
	return reader, errors.AddContext(err, "unable to fetch publink data")
}

// SkynetPublinkPinPost uses the /pubaccess/pin endpoint to pin the file at the
// given publink.
func (c *Client) SkynetPublinkPinPost(publink string, params modules.SkyfilePinParameters) error {
	return c.SkynetPublinkPinPostWithTimeout(publink, params, -1)
}

// SkynetPublinkPinPostWithTimeout uses the /pubaccess/pin endpoint to pin the file
// at the given publink, specifying the given timeout.
func (c *Client) SkynetPublinkPinPostWithTimeout(publink string, params modules.SkyfilePinParameters, timeout int) error {
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

	query := fmt.Sprintf("/pubaccess/pin/%s?%s", publink, values.Encode())
	_, _, err := c.postRawResponse(query, nil)
	if err != nil {
		return errors.AddContext(err, "post call to "+query+" failed")
	}
	return nil
}

// SkynetSkyfilePost uses the /pubaccess/pubfile endpoint to upload a pubfile.  The
// resulting publink is returned along with an error.
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

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, rshp, err
}

// SkynetSkyfilePostDisableForce uses the /pubaccess/pubfile endpoint to upload a
// pubfile. This method allows to set the Disable-Force header. The resulting
// publink is returned along with an error.
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

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, rshp, err
}

// SkynetSkyfileMultiPartPost uses the /pubaccess/pubfile endpoint to upload a
// pubfile using multipart form data.  The resulting publink is returned along
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

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", api.SkynetSkyfileHandlerPOST{}, errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, rshp, err
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

	// Parse the response to get the publink.
	var rshp api.SkynetSkyfileHandlerPOST
	err = json.Unmarshal(resp, &rshp)
	if err != nil {
		return "", errors.AddContext(err, "unable to parse the publink upload response")
	}
	return rshp.Publink, err
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

// SkynetPortalsGet requests the /pubaccess/portals Get endpoint.
func (c *Client) SkynetPortalsGet() (portals api.SkynetPortalsGET, err error) {
	err = c.get("/pubaccess/portals", &portals)
	return
}

// SkynetPortalsPost requests the /pubaccess/portals Post endpoint.
func (c *Client) SkynetPortalsPost(additions []modules.SkynetPortal, removals []modules.NetAddress) (err error) {
	spp := api.SkynetPortalsPOST{
		Add:    additions,
		Remove: removals,
	}
	data, err := json.Marshal(spp)
	if err != nil {
		return err
	}
	err = c.post("/pubaccess/portals", string(data), nil)
	return
}

// SkynetStatsGet requests the /pubaccess/stats Get endpoint
func (c *Client) SkynetStatsGet() (stats api.SkynetStatsGET, err error) {
	err = c.get("/pubaccess/stats", &stats)
	return
}

// SkykeyGetByName requests the /pubaccess/pubaccesskey Get endpoint using the key name.
func (c *Client) SkykeyGetByName(name string) (pubaccesskey.Pubaccesskey, error) {
	values := url.Values{}
	values.Set("name", name)
	getQuery := fmt.Sprintf("/pubaccess/pubaccesskey?%s", values.Encode())

	var skykeyGet api.SkykeyGET
	err := c.get(getQuery, &skykeyGet)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	var sk pubaccesskey.Pubaccesskey
	err = sk.FromString(skykeyGet.Pubaccesskey)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	return sk, nil
}

// SkykeyGetByID requests the /pubaccess/pubaccesskey Get endpoint using the key ID.
func (c *Client) SkykeyGetByID(id pubaccesskey.SkykeyID) (pubaccesskey.Pubaccesskey, error) {
	values := url.Values{}
	values.Set("id", id.ToString())
	getQuery := fmt.Sprintf("/pubaccess/pubaccesskey?%s", values.Encode())

	var skykeyGet api.SkykeyGET
	err := c.get(getQuery, &skykeyGet)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	var sk pubaccesskey.Pubaccesskey
	err = sk.FromString(skykeyGet.Pubaccesskey)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, err
	}

	return sk, nil
}

// SkykeyCreateKeyPost requests the /pubaccess/createpubaccesskey POST endpoint.
func (c *Client) SkykeyCreateKeyPost(name string, ct crypto.CipherType) (pubaccesskey.Pubaccesskey, error) {
	// Set the url values.
	values := url.Values{}
	values.Set("name", name)
	values.Set("ciphertype", ct.String())

	var skykeyGet api.SkykeyGET
	err := c.post("/pubaccess/createpubaccesskey", values.Encode(), &skykeyGet)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "createpubaccesskey POST request failed")
	}

	var sk pubaccesskey.Pubaccesskey
	err = sk.FromString(skykeyGet.Pubaccesskey)
	if err != nil {
		return pubaccesskey.Pubaccesskey{}, errors.AddContext(err, "failed to decode pubaccesskey string")
	}
	return sk, nil
}

// SkykeyAddKeyPost requests the /pubaccess/addpubaccesskey POST endpoint.
func (c *Client) SkykeyAddKeyPost(sk pubaccesskey.Pubaccesskey) error {
	values := url.Values{}
	skString, err := sk.ToString()
	if err != nil {
		return errors.AddContext(err, "failed to encode pubaccesskey as string")
	}
	values.Set("pubaccesskey", skString)

	err = c.post("/pubaccess/addpubaccesskey", values.Encode(), nil)
	if err != nil {
		return errors.AddContext(err, "addpubaccesskey POST request failed")
	}

	return nil
}
