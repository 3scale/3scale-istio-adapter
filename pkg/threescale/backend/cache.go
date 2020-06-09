package backend

import "github.com/orcaman/concurrent-map"

// Cacheable defines the required behaviour of a Backend cache
// The cache key should be in the format of '<serviceID>_<applicationID>'
type Cacheable interface {
	// Get the application from the cache, return bool value declaring if the cache was hit or not
	Get(key string) (*Application, bool)
	// Set the application for the provided key, overwriting any existing entry for the key
	Set(key string, application *Application)
	// Keys returns a list of keys for all cached items
	Keys() []string
}

// LocalCache is an implementation of Cacheable providing an in-memory cache
type LocalCache struct {
	ds cmap.ConcurrentMap
}

// NewLocalCache returns a LocalCache with its data store initialised
func NewLocalCache() *LocalCache {
	return &LocalCache{ds: cmap.New()}
}

// Get entries for LocalCache
func (l LocalCache) Get(cacheKey string) (*Application, bool) {
	var cv *Application
	v, ok := l.ds.Get(cacheKey)

	if ok {
		cv = v.(*Application)
	}

	return cv, ok
}

// Set entries for LocalCache
func (l LocalCache) Set(cacheKey string, app *Application) {
	l.ds.Set(cacheKey, app)
}

// Keys returns a list of keys for all cached items
func (l LocalCache) Keys() []string {
	return l.ds.Keys()
}
