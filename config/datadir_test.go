package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAlgodArgsForDataDirNetDoesNotExist(t *testing.T) {
	_, _, _, err := AlgodArgsForDataDir("foobar")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "foobar/algod.net: ")
}

func TestAlgodArgsForDataDirTokenDoesNotExist(t *testing.T) {
	dir, err := os.MkdirTemp("", "datadir")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = os.WriteFile(filepath.Join(dir, "algod.net"), []byte("127.0.0.1:8080"), fs.ModePerm)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)
	_, _, _, err = AlgodArgsForDataDir(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%v/algod.token: ", dir))
}

func TestAlgodArgsForDataDirSuccess(t *testing.T) {
	dir, err := os.MkdirTemp("", "datadir")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = os.WriteFile(filepath.Join(dir, "algod.net"), []byte("127.0.0.1:8080"), fs.ModePerm)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = os.WriteFile(filepath.Join(dir, "algod.token"), []byte("abc123"), fs.ModePerm)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(dir)
	netAddr, token, lastmod, err := AlgodArgsForDataDir(dir)
	assert.NoError(t, err)
	assert.Equal(t, netAddr, "http://127.0.0.1:8080")
	assert.Equal(t, token, "abc123")
	assert.NotNil(t, lastmod)

}
