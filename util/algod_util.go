package util

import (
	"fmt"
	"io/ioutil"
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

func algodStat(netpath, tokenpath string) (lastmod time.Time, err error) {
	nstat, err := os.Stat(netpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", netpath, err)
		return
	}
	tstat, err := os.Stat(tokenpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", tokenpath, err)
		return
	}
	if nstat.ModTime().Before(tstat.ModTime()) {
		lastmod = tstat.ModTime()
	}
	lastmod = nstat.ModTime()
	return
}

func LookupNetAndToken(algodDataDir string) (string, string, error) {
	netpath, tokenpath := algodPaths(algodDataDir)
	var netaddrbytes []byte

	netaddrbytes, err := ioutil.ReadFile(netpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", netpath, err)
		return "", "", err
	}
	netaddr := strings.TrimSpace(string(netaddrbytes))
	if !strings.HasPrefix(netaddr, "http") {
		netaddr = "http://" + netaddr
	}

	tokenbytes, err := ioutil.ReadFile(tokenpath)
	if err != nil {
		err = fmt.Errorf("%s: %v", tokenpath, err)
		return "", "", err
	}
	token := strings.TrimSpace(string(tokenbytes))

	return netaddr, token, nil
}
