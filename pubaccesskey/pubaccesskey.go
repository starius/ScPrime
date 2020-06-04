package pubaccesskey

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/url"

	"github.com/aead/chacha20/chacha"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/encoding"
	"gitlab.com/scpcorp/ScPrime/types"
)

const (
	// SkykeyScheme is the URI scheme for encoded skykeys.
	SkykeyScheme = "pubaccesskey"

	// SkykeyIDLen is the length of a SkykeyID
	SkykeyIDLen = 16

	// MaxKeyNameLen is the maximum length of a pubaccesskey's name.
	MaxKeyNameLen = 128

	// Define SkykeyTypes. Constants stated explicitly (instead of
	// `SkykeyType(iota)`) to avoid re-ordering mistakes in the future.

	// TypeInvalid represents an invalid skykey type.
	TypeInvalid = SkykeyType(0x00)

	// TypePublicID is a Skykey that uses XChaCha20. It reveals its
	// skykey ID in *every* skyfile it encrypts.
	TypePublicID = SkykeyType(0x01)

	// TypePrivateID is a Skykey that uses XChaCha20 that does not
	// reveal its skykey ID when encrypting Skyfiles. Instead, it marks the skykey
	// used for encryption by storing an encrypted identifier that can only be
	// successfully decrypted with the correct skykey.
	// TODO: add along with skyfile implementation TypePrivateID = SkykeyType(0x02)
)

var (
	// SkykeySpecifier is used as a prefix when hashing Pubaccesskeys to compute their
	// ID.
	SkykeySpecifier = types.NewSpecifier("Pubaccesskey")

	errUnsupportedSkykeyType          = errors.New("Unsupported Pubaccesskey type")
	errUnmarshalDataErr               = errors.New("Unable to unmarshal Pubaccesskey data")
	errCannotMarshalTypeInvalidSkykey = errors.New("Cannot marshal or unmarshal Pubaccesskey of TypeInvalid type")
	errInvalidEntropyLength           = errors.New("Invalid pubaccesskey entropy length")

	// ErrInvalidSkykeyType is returned when an invalid SkykeyType is being used.
	ErrInvalidSkykeyType = errors.New("Invalid pubaccesskey type")
)

// SkykeyID is the identifier of a pubaccesskey.
type SkykeyID [SkykeyIDLen]byte

// SkykeyType encodes the encryption scheme and method used by the Pubaccesskey.
type SkykeyType byte

// Pubaccesskey is a key used to encrypt/decrypt skyfiles.
type Pubaccesskey struct {
	Name    string
	Type    SkykeyType
	Entropy []byte
}

// compatSkykeyV148 is the original pubaccesskey format. It is defined here for
// compatibility purposes. It should only be used to convert keys of the old
// format to the new format.
type compatSkykeyV148 struct {
	name       string
	ciphertype crypto.CipherType
	entropy    []byte
}

// ToString returns the string representation of the ciphertype.
func (t SkykeyType) ToString() string {
	switch t {
	case TypePublicID:
		return "public-id"

	default:
		return "invalid"
	}
}

// FromString reads a SkykeyType from a string.
func (t *SkykeyType) FromString(s string) error {
	switch s {
	case "public-id":
		*t = TypePublicID
	default:
		return ErrInvalidSkykeyType
	}
	return nil
}

// unmarshalSia decodes the Pubaccesskey into the reader.
func (skOld *compatSkykeyV148) unmarshalSia(r io.Reader) error {
	d := encoding.NewDecoder(r, encoding.DefaultAllocLimit)
	d.Decode(&skOld.name)
	d.Decode(&skOld.ciphertype)
	d.Decode(&skOld.entropy)

	if err := d.Err(); err != nil {
		return err
	}
	if len(skOld.name) > MaxKeyNameLen {
		return errSkykeyNameToolong
	}
	if len(skOld.entropy) != chacha.KeySize+chacha.XNonceSize {
		return errInvalidEntropyLength
	}

	return nil
}

// marshalSia encodes the Pubaccesskey into the writer.
func (skOld compatSkykeyV148) marshalSia(w io.Writer) error {
	e := encoding.NewEncoder(w)
	e.Encode(skOld.name)
	e.Encode(skOld.ciphertype)
	e.Encode(skOld.entropy)
	return e.Err()
}

// fromString decodes a base64-encoded string, interpreting it as the old pubaccesskey
// format.
func (skOld *compatSkykeyV148) fromString(s string) error {
	keyBytes, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return skOld.unmarshalSia(bytes.NewReader(keyBytes))
}

// convertToUpdatedFormat converts the pubaccesskey from the old format to the updated
// format.
func (skOld compatSkykeyV148) convertToUpdatedFormat() (Pubaccesskey, error) {
	sk := Pubaccesskey{
		Name:    skOld.name,
		Type:    TypePublicID,
		Entropy: skOld.entropy,
	}

	// Sanity check that we can actually make a CipherKey with this.
	_, err := crypto.NewSiaKey(sk.CipherType(), sk.Entropy)
	if err != nil {
		return Pubaccesskey{}, errors.AddContext(err, "Unable to convert pubaccesskey from old format correctly")
	}

	return sk, nil
}

// unmarshalAndConvertFromOldFormat unmarshals data from the reader as a pubaccesskey
// using the old format, and attempts to convert it to the new format.
func (sk *Pubaccesskey) unmarshalAndConvertFromOldFormat(r io.Reader) error {
	var oldFormatSkykey compatSkykeyV148
	err := oldFormatSkykey.unmarshalSia(r)
	if err != nil {
		return err
	}
	convertedSk, err := oldFormatSkykey.convertToUpdatedFormat()
	if err != nil {
		return err
	}
	sk.Name = convertedSk.Name
	sk.Type = convertedSk.Type
	sk.Entropy = convertedSk.Entropy
	return sk.IsValid()
}

// CipherType returns the crypto.CipherType used by this Pubaccesskey.
func (t SkykeyType) CipherType() crypto.CipherType {
	switch t {
	case TypePublicID:
		return crypto.TypeXChaCha20
	default:
		return crypto.TypeInvalid
	}
}

// CipherType returns the crypto.CipherType used by this Pubaccesskey.
func (sk *Pubaccesskey) CipherType() crypto.CipherType {
	return sk.Type.CipherType()
}

// unmarshalDataOnly decodes the Pubaccesskey data into the reader.
func (sk *Pubaccesskey) unmarshalDataOnly(r io.Reader) error {
	d := encoding.NewDecoder(r, encoding.DefaultAllocLimit)
	typeByte, _ := d.ReadByte()
	sk.Type = SkykeyType(typeByte)

	var entropyLen int
	switch sk.Type {
	case TypePublicID:
		entropyLen = chacha.KeySize + chacha.XNonceSize
	case TypeInvalid:
		return errCannotMarshalTypeInvalidSkykey
	default:
		return errUnsupportedSkykeyType
	}

	sk.Entropy = make([]byte, entropyLen)
	d.ReadFull(sk.Entropy)
	if err := d.Err(); err != nil {
		return err
	}
	return nil
}

// unmarshalSia decodes the Pubaccesskey data and name into the reader.
func (sk *Pubaccesskey) unmarshalSia(r io.Reader) error {
	err := sk.unmarshalDataOnly(r)
	if err != nil {
		return errors.Compose(errUnmarshalDataErr, err)
	}
	d := encoding.NewDecoder(r, encoding.DefaultAllocLimit)
	d.Decode(&sk.Name)

	if err := d.Err(); err != nil {
		return err
	}

	return sk.IsValid()
}

// marshalDataOnly encodes the Pubaccesskey data into the writer.
func (sk Pubaccesskey) marshalDataOnly(w io.Writer) error {
	e := encoding.NewEncoder(w)

	var entropyLen int
	switch sk.Type {
	case TypePublicID:
		entropyLen = chacha.KeySize + chacha.XNonceSize
	case TypeInvalid:
		return errCannotMarshalTypeInvalidSkykey
	default:
		return errUnsupportedSkykeyType
	}

	if len(sk.Entropy) != entropyLen {
		return errInvalidEntropyLength
	}

	e.WriteByte(byte(sk.Type))
	e.Write(sk.Entropy[:entropyLen])
	return e.Err()
}

// marshalSia encodes the Pubaccesskey data and name into the writer.
func (sk Pubaccesskey) marshalSia(w io.Writer) error {
	err := sk.marshalDataOnly(w)
	if err != nil {
		return err
	}
	e := encoding.NewEncoder(w)
	e.Encode(sk.Name)
	return e.Err()
}

// toURL encodes the pubaccesskey as a URL.
func (sk Pubaccesskey) toURL() (url.URL, error) {
	var b bytes.Buffer
	err := sk.marshalDataOnly(&b)
	if err != nil {
		return url.URL{}, err
	}
	skykeyString := base64.URLEncoding.EncodeToString(b.Bytes())

	skURL := url.URL{
		Scheme: SkykeyScheme,
		Opaque: skykeyString,
	}
	if sk.Name != "" {
		skURL.RawQuery = "name=" + sk.Name
	}
	return skURL, nil
}

// ToString encodes the Pubaccesskey as a base64 string.
func (sk Pubaccesskey) ToString() (string, error) {
	skURL, err := sk.toURL()
	if err != nil {
		return "", err
	}
	return skURL.String(), nil
}

// FromString decodes the base64 string into a Pubaccesskey.
func (sk *Pubaccesskey) FromString(s string) error {
	sURL, err := url.Parse(s)
	if err != nil {
		return err
	}

	// Get the pubaccesskey data from the path/opaque data.
	var skData string
	if sURL.Scheme == SkykeyScheme {
		skData = sURL.Opaque
	} else if sURL.Scheme == "" {
		skData = sURL.Path
	} else {
		return errors.New("Unknown URI scheme for pubaccesskey")
	}

	values := sURL.Query()
	sk.Name = values.Get("name") // defaults to ""
	if len(sk.Name) > MaxKeyNameLen {
		return errSkykeyNameToolong
	}

	keyBytes, err := base64.URLEncoding.DecodeString(skData)
	if err != nil {
		return err
	}
	return sk.unmarshalDataOnly(bytes.NewReader(keyBytes))
}

// ID returns the ID for the Pubaccesskey. A master Pubaccesskey and all file-specific
// skykeys derived from it share the same ID because they only differ in nonce
// values, not key values. This fact is used to identify the master Pubaccesskey
// with which a Pubaccess file was encrypted.
func (sk Pubaccesskey) ID() (keyID SkykeyID) {
	entropy := sk.Entropy

	switch sk.Type {
	// Ignore the nonce for this type because the nonce is different for each
	// file-specific subkey.
	case TypePublicID:
		entropy = sk.Entropy[:chacha.KeySize]

	default:
		build.Critical("Computing ID with pubaccesskey of unknown type: ", sk.Type)
	}

	h := crypto.HashAll(SkykeySpecifier, sk.Type, entropy)
	copy(keyID[:], h[:SkykeyIDLen])
	return keyID
}

// ToString encodes the SkykeyID as a base64 string.
func (id SkykeyID) ToString() string {
	return base64.URLEncoding.EncodeToString(id[:])
}

// FromString decodes the base64 string into a Pubaccesskey ID.
func (id *SkykeyID) FromString(s string) error {
	idBytes, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	if len(idBytes) != SkykeyIDLen {
		return errors.New("Pubaccesskey ID has invalid length")
	}
	copy(id[:], idBytes[:])
	return nil
}

// equals returns true if and only if the two Skykeys are equal.
func (sk *Pubaccesskey) equals(otherKey Pubaccesskey) bool {
	return sk.Name == otherKey.Name && sk.equalData(otherKey)
}

// equalData returns true if and only if the two Skykeys have the same
// underlying data. They can differ in name.
func (sk *Pubaccesskey) equalData(otherKey Pubaccesskey) bool {
	return sk.Type == otherKey.Type && bytes.Equal(sk.Entropy[:], otherKey.Entropy[:])
}

// GenerateFileSpecificSubkey creates a new subkey specific to a certain file
// being uploaded/downloaded. Skykeys can only be used once with a
// given nonce, so this method is used to generate keys with new nonces when a
// new file is uploaded.
func (sk *Pubaccesskey) GenerateFileSpecificSubkey() (Pubaccesskey, error) {
	// Generate a new random nonce.
	nonce := make([]byte, chacha.XNonceSize)
	fastrand.Read(nonce[:])
	return sk.SubkeyWithNonce(nonce)
}

// DeriveSubkey is used to create Skykeys with the same key, but with a
// different nonce. This is used to create file-specific keys, and separate keys
// for Pubfile baseSector uploads and fanout uploads.
func (sk *Pubaccesskey) DeriveSubkey(derivation []byte) (Pubaccesskey, error) {
	nonce := sk.Nonce()
	derivedNonceHash := crypto.HashAll(nonce, derivation)
	derivedNonce := derivedNonceHash[:chacha.XNonceSize]

	return sk.SubkeyWithNonce(derivedNonce)
}

// SubkeyWithNonce creates a new subkey with the same key data as this key, but
// with the given nonce.
func (sk *Pubaccesskey) SubkeyWithNonce(nonce []byte) (Pubaccesskey, error) {
	if len(nonce) != chacha.XNonceSize {
		return Pubaccesskey{}, errors.New("Incorrect nonce size")
	}

	entropy := make([]byte, chacha.KeySize+chacha.XNonceSize)
	copy(entropy[:chacha.KeySize], sk.Entropy[:chacha.KeySize])
	copy(entropy[chacha.KeySize:], nonce[:])

	// Sanity check that we can actually make a CipherKey with this.
	_, err := crypto.NewSiaKey(sk.CipherType(), entropy)
	if err != nil {
		return Pubaccesskey{}, errors.AddContext(err, "error creating new pubaccesskey subkey")
	}

	subkey := Pubaccesskey{sk.Name, sk.Type, entropy}
	return subkey, nil
}

// CipherKey returns the crypto.CipherKey equivalent of this Pubaccesskey.
func (sk *Pubaccesskey) CipherKey() (crypto.CipherKey, error) {
	return crypto.NewSiaKey(sk.CipherType(), sk.Entropy)
}

// Nonce returns the nonce of this Pubaccesskey.
func (sk *Pubaccesskey) Nonce() []byte {
	nonce := make([]byte, chacha.XNonceSize)
	copy(nonce[:], sk.Entropy[chacha.KeySize:])
	return nonce
}

// IsValid returns an nil if the pubaccesskey is valid and an error otherwise.
func (sk *Pubaccesskey) IsValid() error {
	if len(sk.Name) > MaxKeyNameLen {
		return errSkykeyNameToolong
	}

	switch sk.Type {
	case TypePublicID:
		if len(sk.Entropy) != chacha.KeySize+chacha.XNonceSize {
			return errInvalidEntropyLength
		}

	default:
		return errUnsupportedSkykeyType
	}

	_, err := crypto.NewSiaKey(sk.CipherType(), sk.Entropy)
	if err != nil {
		return err
	}
	return nil
}
