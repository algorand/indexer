package accounting

import (
	"fmt"
)

type subCache map[[32]byte]string

type accountTypeCache struct {
	generations []subCache
}

const generationSize = 1000
const numGenerations = 5

func (cache *accountTypeCache) set(addr [32]byte, ktype string) (isNew bool, err error) {
	for _, sc := range cache.generations {
		oldvalue, hit := sc[addr]
		if hit {
			isNew = false
			if oldvalue != ktype {
				err = fmt.Errorf("previously had type %s but got %s", oldvalue, ktype)
			}
			return
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
