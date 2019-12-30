package backend

import (
	"testing"
)

func TestLocalCache_Get(t *testing.T) {
	const cacheKey = "testKey"
	cache := NewLocalCache()

	// test a miss
	if _, ok := cache.Get("any"); ok {
		t.Errorf("expected cache key lookup on a new local cache to have missed")
	}

	// test happy path
	appFixture := &Application{
		UnlimitedCounter: map[string]int{"test": 1},
	}

	cache.ds.Set(cacheKey, appFixture)

	cachedApp, ok := cache.Get(cacheKey)
	if !ok {
		t.Errorf("expected cache key not to have missed")
	}

	val, ok := cachedApp.UnlimitedCounter["test"]
	if !ok || val != 1 {
		t.Errorf("Unlimited counter result invalid")
	}
}

func TestLocalCache_Set(t *testing.T) {
	const cacheKey = "testKey"
	cache := NewLocalCache()

	appFixture := &Application{
		UnlimitedCounter: map[string]int{"test": 1},
	}

	cache.Set(cacheKey, appFixture)

	appIntf, ok := cache.ds.Get(cacheKey)
	if !ok {
		t.Errorf("failed to fetch item from underlying data structure")
	}

	cachedApp, ok := appIntf.(*Application)
	if !ok {
		t.Errorf("error during type assertion, wanted a pointer to App;ication but got %v", appIntf)
	}

	val, ok := cachedApp.UnlimitedCounter["test"]
	if !ok || val != 1 {
		t.Errorf("Unlimited counter result invalid")
	}
}

func TestLocalCache_Keys(t *testing.T) {
	const cacheKeyOne = "testKeyOne"
	const cacheKeyTwo = "testKeyTwo"
	cache := NewLocalCache()

	appFixture := &Application{
		UnlimitedCounter: map[string]int{"test": 1},
	}

	appFixtureTwo := &Application{
		UnlimitedCounter: map[string]int{"test": 1},
	}

	cache.Set(cacheKeyOne, appFixture)
	cache.Set(cacheKeyTwo, appFixtureTwo)

	keys := cache.Keys()
	equals(t, 2, len(keys))

	contains := func(list []string, keys ...string) bool {
		for _, element := range list {
			for _, key := range keys {
				if element == key {
					return true
				}
			}

		}
		return false
	}
	if !contains(keys, cacheKeyOne, cacheKeyTwo) {
		t.Errorf("expected both keys to be found")
	}

}

// Test the caching behaviour using both exported funcs
func TestLocalCache(t *testing.T) {
	const cacheKey = "testKey"
	cache := NewLocalCache()

	appFixture := &Application{
		UnlimitedCounter: map[string]int{"test": 1},
	}

	cache.Set(cacheKey, appFixture)

	app, ok := cache.Get(cacheKey)
	if !ok {
		t.Errorf("failed to fetch item from underlying data structure")
	}

	val, ok := app.UnlimitedCounter["test"]
	if !ok {
		t.Errorf("Unlimited counter result not found")
	}
	if val != 1 {
		t.Errorf("Unlimited counter result unexpected, wanted 1 but got %d", val)
	}
}
