package filewriter

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/algorand/go-codec/codec"

	"github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand/protocol"
)

var prettyHandle *codec.JsonHandle

func init() {
	prettyHandle = new(codec.JsonHandle)
	prettyHandle.ErrorIfNoField = json.CodecHandle.ErrorIfNoField
	prettyHandle.ErrorIfNoArrayExpand = json.CodecHandle.ErrorIfNoArrayExpand
	prettyHandle.Canonical = json.CodecHandle.Canonical
	prettyHandle.RecursiveEmptyCheck = json.CodecHandle.RecursiveEmptyCheck
	prettyHandle.Indent = json.CodecHandle.Indent
	prettyHandle.HTMLCharsAsIs = json.CodecHandle.HTMLCharsAsIs
	prettyHandle.MapKeyAsString = true
	prettyHandle.Indent = 2
}

// EncodeJSONToFile is used to encode an object to a file. If the file ends in .gz it will be gzipped.
func EncodeJSONToFile(filename string, v interface{}, pretty bool) error {
	var writer io.Writer

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("EncodeJSONToFile(): failed to create %s: %w", filename, err)
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

	var handle *codec.JsonHandle
	if pretty {
		handle = prettyHandle
	} else {
		handle = protocol.JSONStrictHandle
	}
	enc := codec.NewEncoder(writer, handle)
	return enc.Encode(v)
}

// DecodeJSONFromFile is used to decode a file to an object.
func DecodeJSONFromFile(filename string, v interface{}, strict bool) error {
	// Streaming into the decoder was slow.
	fileBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("DecodeJSONFromFile(): failed to read %s: %w", filename, err)
	}

	var reader io.Reader = bytes.NewReader(fileBytes)

	if strings.HasSuffix(filename, ".gz") {
		gz, err := gzip.NewReader(reader)
		defer gz.Close()
		if err != nil {
			return fmt.Errorf("DecodeJSONFromFile(): failed to make gzip reader: %w", err)
		}
		reader = gz
	}
	var handle *codec.JsonHandle
	if strict {
		handle = json.CodecHandle
	} else {
		handle = json.LenientCodecHandle
	}

	enc := codec.NewDecoder(reader, handle)
	return enc.Decode(v)
}
