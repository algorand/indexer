package accounting

import (
	"github.com/algorand/go-algorand-sdk/types"
	log "github.com/sirupsen/logrus"
)

type subCache map[[32]byte]string

type accountTypeCache struct {
	generations []subCache
	l           *log.Logger
}

const generationSize = 1000
const numGenerations = 5

func (cache *accountTypeCache) set(addr [32]byte, ktype string) (isNew bool) {
	for _, sc := range cache.generations {
		oldvalue, hit := sc[addr]
		if hit {
			if oldvalue == ktype {
				// Value already set properly
				return false
			}

			// otherwise, warn about the unexpected change (edge case related to rekeyed lsig transactions)
			addrObject := types.Address{}
			copy(addrObject[:], addr[:])
			cache.l.Warnf("accountTypeCache.set(): previously had type %s but got %s for sender %s", oldvalue, ktype, addrObject.String())
			break
		}
	}
	isNew = true
	if len(cache.generations) == 0 {
		cache.generations = make([]subCache, 1, numGenerations)
		cache.generations[0] = make(map[[32]byte]string, generationSize)
		cache.generations[0][addr] = ktype
		return
	}
	sc := cache.generations[len(cache.generations)-1]
	if len(sc) >= generationSize {
		if len(cache.generations) >= numGenerations {
			cache.generations = cache.generations[1:]
		}
		sc = make(map[[32]byte]string, generationSize)
		cache.generations = append(cache.generations, sc)
	}
	sc[addr] = ktype
	return
}
