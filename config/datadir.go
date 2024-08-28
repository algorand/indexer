package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func algodPaths(datadir string) (netpath, tokenpath string) {
	netpath = filepath.Join(datadir, "algod.net")
	tokenpath = filepath.Join(datadir, "algod.token")
	return
}

func algodStat(netpath string) (lastmod time.Time, err error) {
	nstat, err := os.Stat(netpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", netpath, err)
		return
	}
	lastmod = nstat.ModTime()
	return
}

// AlgodArgsForDataDir opens the token and network files in the data directory, returning data for constructing client
func AlgodArgsForDataDir(datadir string) (netAddr string, token string, lastmod time.Time, err error) {
	netpath, tokenpath := algodPaths(datadir)
	var netaddrbytes []byte
	netaddrbytes, err = os.ReadFile(netpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", netpath, err)
		return
	}
	netAddr = strings.TrimSpace(string(netaddrbytes))
	if !strings.HasPrefix(netAddr, "http") {
		netAddr = "http://" + netAddr
	}

	tokenBytes, err := os.ReadFile(tokenpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", tokenpath, err)
		return
	}
	token = strings.TrimSpace(string(tokenBytes))

	if err == nil {
		lastmod, err = algodStat(netpath)
	}

	return
}
