package util

import (
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base32"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/algorand/go-algorand-sdk/encoding/json"
	sdk "github.com/algorand/go-algorand-sdk/types"
	"github.com/algorand/go-codec/codec"
)

const (
	checksumLength = 4
)

var base32Encoder = base32.StdEncoding.WithPadding(base32.NoPadding)

// EncodeToFile is used to encode an object to a file. If the file ends in .gz it will be gzipped.
func EncodeToFile(filename string, v interface{}, pretty bool) error {
	var writer io.Writer

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("EncodeToFile(): failed to create %s: %w", filename, err)
	}
	defer file.Close()

	if strings.HasSuffix(filename, ".gz") {
		gz := gzip.NewWriter(file)
		gz.Name = filename
		defer gz.Close()
		writer = gz
	} else {
		writer = file
	}

	handle := json.CodecHandle
	if pretty {
		handle.Indent = 2
	} else {
		handle.Indent = 0
	}
	enc := codec.NewEncoder(writer, handle)
	return enc.Encode(v)
}

// DecodeFromFile is used to decode a file to an object.
func DecodeFromFile(filename string, v interface{}) error {
	// Streaming into the decoder was slow.
	fileBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("DecodeFromFile(): failed to read %s: %w", filename, err)
	}

	var reader io.Reader = bytes.NewReader(fileBytes)

	if strings.HasSuffix(filename, ".gz") {
		gz, err := gzip.NewReader(reader)
		defer gz.Close()
		if err != nil {
			return fmt.Errorf("DecodeFromFile(): failed to make gzip reader: %w", err)
		}
		reader = gz
	}

	enc := codec.NewDecoder(reader, json.CodecHandle)
	return enc.Decode(v)
}

// PrintableUTF8OrEmpty checks to see if the entire string is a UTF8 printable string.
// If this is the case, the string is returned as is. Otherwise, the empty string is returned.
func PrintableUTF8OrEmpty(in string) string {
	// iterate throughout all the characters in the string to see if they are all printable.
	// when range iterating on go strings, go decode each element as a utf8 rune.
	for _, c := range in {
		// is this a printable character, or invalid rune ?
		if c == utf8.RuneError || !unicode.IsPrint(c) {
			return ""
		}
	}
	return in
}

// KeysStringBool returns all of the keys in the map joined by a comma.
func KeysStringBool(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}

// MaybeFail exits if there was an error.
func MaybeFail(err error, errfmt string, params ...interface{}) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errfmt, params...)
	fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
	os.Exit(1)
}

var oneLineJSONCodecHandle *codec.JsonHandle

// JSONOneLine converts an object into JSON
func JSONOneLine(obj interface{}) string {
	var b []byte
	enc := codec.NewEncoderBytes(&b, oneLineJSONCodecHandle)
	enc.MustEncode(obj)
	return string(b)
}

// FileExists checks to see if the specified file (or directory) exists
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	fileExists := err == nil
	return fileExists
}

// IsDir returns true if the specified directory is valid
func IsDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// UnmarshalChecksumAddress tries to unmarshal the checksummed address string.
// Algorand strings addresses ( base32 encoded ) have a postamble which serves as the checksum of the address.
// When converted to an Address object representation, that checksum is dropped (after validation).
func UnmarshalChecksumAddress(address string) (sdk.Address, error) {
	decoded, err := base32Encoder.DecodeString(address)

	if err != nil {
		return sdk.Address{}, fmt.Errorf("failed to decode address %s to base 32", address)
	}
	var short sdk.Address
	if len(decoded) < len(short) {
		return sdk.Address{}, fmt.Errorf("decoded bad addr: %s", address)
	}

	copy(short[:], decoded[:len(short)])
	incomingchecksum := decoded[len(decoded)-checksumLength:]

	calculatedchecksum := getChecksum(short[:])
	isValid := bytes.Equal(incomingchecksum, calculatedchecksum)

	if !isValid {
		return sdk.Address{}, fmt.Errorf("address %s is malformed, checksum verification failed", address)
	}

	// Validate that we had a canonical string representation
	if short.String() != address {
		return sdk.Address{}, fmt.Errorf("address %s is non-canonical", address)
	}

	return short, nil
}

// getChecksum returns the checksum as []byte
// Checksum in Algorand are the last 4 bytes of the shortAddress Hash. H(Address)[28:]
func getChecksum(addr []byte) []byte {
	shortAddressHash := sha512.Sum512_256(addr[:])
	checksum := shortAddressHash[len(shortAddressHash)-checksumLength:]
	return checksum
}

func init() {
	oneLineJSONCodecHandle = new(codec.JsonHandle)
	oneLineJSONCodecHandle.ErrorIfNoField = true
	oneLineJSONCodecHandle.ErrorIfNoArrayExpand = true
	oneLineJSONCodecHandle.Canonical = true
	oneLineJSONCodecHandle.RecursiveEmptyCheck = true
	oneLineJSONCodecHandle.HTMLCharsAsIs = true
	oneLineJSONCodecHandle.Indent = 0
	oneLineJSONCodecHandle.MapKeyAsString = true
}
