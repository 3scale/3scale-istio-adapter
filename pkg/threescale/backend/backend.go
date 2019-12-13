package backend

import (
	"sync"

	"github.com/3scale/3scale-go-client/threescale"
	"github.com/3scale/3scale-go-client/threescale/api"
)

// Backend defines the connection to a single backend and maintains a cache
// for multiple services and applications per backend. It implements the 3scale Client interface
type Backend struct {
	client threescale.Client
	cache  Cacheable
}

// Application defined under a 3scale service
type Application struct {
	RemoteState      LimitCounter
	LimitCounter     LimitCounter
	UnlimitedCounter UnlimitedCounter
	sync.RWMutex
	metricHierarchy api.Hierarchy
	auth            api.ClientAuth
	params          api.Params
}

// Limit captures the current state of the rate limit for a particular time period
type Limit struct {
	current int
	max     int
}

// LimitCounter keeps a count of limits for a given period
type LimitCounter map[string]map[api.Period]*Limit

// UnlimitedCounter keeps a count of metrics without limits
type UnlimitedCounter map[string]int
