package benchmarks

/*
 Benchmarking tests for various key/value store and concurrent map implementations
*/

import (
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	beego "github.com/astaxie/beego/cache"
	"github.com/muesli/cache2go"
	cm "github.com/orcaman/concurrent-map"
	cache "github.com/patrickmn/go-cache"
)

var cacheWarmOnly bool
var numClients int
var repeatNTimes int

const (
	warmOnlyDefault    = false
	numClientsDefault  = 100000
	repeatTimesDefault = 10

	readers = 10
	writers = 6
	deleter = 2

	modifiers = readers + writers + deleter
)

type genericClient interface {
	run(c []client, operations int, b *testing.B)
}

type client struct {
	staticServiceId string
}

type syncMapImplementation struct {
	ds sync.Map
}

func init() {
	flag.BoolVar(&cacheWarmOnly, "warm-only", warmOnlyDefault, "Do initial writes to cache without follow up read-writes. Default false")
	flag.IntVar(&numClients, "num-clients", numClientsDefault, "Number of initial clients to load cache with. Default 100,000")
	flag.IntVar(&repeatNTimes, "repeat", repeatTimesDefault, "Number of times to repeat each benchmark. Default 10")
	flag.Parse()
}

func valueGenerator() int32 {
	return rand.Int31()
}

func (smi syncMapImplementation) run(clients []client, ops int, b *testing.B) {
	var setWg sync.WaitGroup
	clientLen := len(clients)

	setWg.Add(clientLen)
	setAll := func() {
		for _, c := range clients {
			id := c.staticServiceId
			go func(id string) {
				defer setWg.Done()
				smi.ds.Store(id, valueGenerator())
			}(id)
		}
	}
	setAll()
	setWg.Wait()

	if !cacheWarmOnly {
		var writerWg sync.WaitGroup
		writerWg.Add(modifiers)
		write := func() {

			// 10 readers
			for i := 0; i < readers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						smi.ds.Load(randClient)
					}
				}()
			}

			// 6 writers to modify existing values
			for i := 0; i < writers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						smi.ds.Store(randClient, valueGenerator())
					}
				}()
			}

			// add one and delete one
			for i := 0; i < deleter; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						clientIndex := rand.Intn(clientLen)
						clientID := clients[clientIndex].staticServiceId
						smi.ds.Delete(clientID)

						smi.ds.Store(clientID, valueGenerator())
					}
				}()
			}

		}

		write()
		writerWg.Wait()
	}
}

type concurrentMapImplementation struct {
	ds cm.ConcurrentMap
}

func (cmi concurrentMapImplementation) run(clients []client, ops int, b *testing.B) {
	var setWg sync.WaitGroup
	clientLen := len(clients)

	setWg.Add(clientLen)
	setAll := func() {
		for _, c := range clients {
			id := c.staticServiceId
			go func(id string) {
				defer setWg.Done()
				cmi.ds.Set(id, valueGenerator())
			}(id)
		}
	}
	setAll()
	setWg.Wait()

	if !cacheWarmOnly {
		var writerWg sync.WaitGroup
		writerWg.Add(modifiers)
		write := func() {
			// 10 readers
			for i := 0; i < readers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						cmi.ds.Get(randClient)
					}
				}()
			}

			// 6 writers to modify existing values
			for i := 0; i < writers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						cmi.ds.Set(randClient, valueGenerator())
					}
				}()
			}

			// add one and delete one
			for i := 0; i < deleter; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						clientIndex := rand.Intn(clientLen)
						clientID := clients[clientIndex].staticServiceId
						cmi.ds.Remove(clientID)

						cmi.ds.Set(clientID, valueGenerator())
					}
				}()
			}

		}

		write()
		writerWg.Wait()
	}
}

type beegoCacheImplementation struct {
	ds beego.Cache
}

func (bci beegoCacheImplementation) run(clients []client, ops int, b *testing.B) {
	var setWg sync.WaitGroup
	clientLen := len(clients)

	setWg.Add(clientLen)
	setAll := func() {
		for _, c := range clients {
			id := c.staticServiceId
			go func(id string) {
				defer setWg.Done()
				bci.ds.Put(id, rand.Int31(), time.Hour)
			}(id)
		}
	}
	setAll()
	setWg.Wait()

	if !cacheWarmOnly {
		var writerWg sync.WaitGroup
		writerWg.Add(modifiers)
		write := func() {
			// 10 readers
			for i := 0; i < readers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						bci.ds.Get(randClient)
					}
				}()
			}

			// 6 writers to modify existing values
			for i := 0; i < writers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						bci.ds.Put(randClient, valueGenerator(), time.Hour)
					}
				}()
			}

			// add one and delete one
			for i := 0; i < deleter; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						clientIndex := rand.Intn(clientLen)
						clientID := clients[clientIndex].staticServiceId
						bci.ds.Delete(clientID)

						bci.ds.Put(clientID, valueGenerator(), time.Hour)
					}
				}()
			}
		}

		write()
		writerWg.Wait()
	}
}

type goCacheImplementation struct {
	ds *cache.Cache
}

func (gci goCacheImplementation) run(clients []client, ops int, b *testing.B) {
	var setWg sync.WaitGroup
	clientLen := len(clients)

	setWg.Add(clientLen)
	setAll := func() {
		for _, c := range clients {
			id := c.staticServiceId
			go func(id string) {
				defer setWg.Done()
				gci.ds.Set(id, rand.Int31(), time.Hour)
			}(id)
		}
	}
	setAll()
	setWg.Wait()

	if !cacheWarmOnly {
		var writerWg sync.WaitGroup
		writerWg.Add(modifiers)
		write := func() {
			// 10 readers
			for i := 0; i < readers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						gci.ds.Get(randClient)
					}
				}()
			}

			// 6 writers to modify existing values
			for i := 0; i < writers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						gci.ds.Set(randClient, valueGenerator(), time.Hour)
					}
				}()
			}

			// add one and delete one
			for i := 0; i < deleter; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						clientIndex := rand.Intn(clientLen)
						clientID := clients[clientIndex].staticServiceId
						gci.ds.Delete(clientID)

						gci.ds.Set(clientID, valueGenerator(), time.Hour)
					}
				}()
			}
		}

		write()
		writerWg.Wait()
	}
}

type cacheToGoImplementation struct {
	ds *cache2go.CacheTable
}

func (ctg cacheToGoImplementation) run(clients []client, ops int, b *testing.B) {
	var setWg sync.WaitGroup
	clientLen := len(clients)

	setWg.Add(clientLen)
	setAll := func() {
		for _, c := range clients {
			id := c.staticServiceId
			go func(id string) {
				defer setWg.Done()
				ctg.ds.Add(id, time.Hour, valueGenerator())
			}(id)
		}
	}
	setAll()
	setWg.Wait()

	if !cacheWarmOnly {
		var writerWg sync.WaitGroup
		writerWg.Add(modifiers)
		write := func() {
			// 10 readers
			for i := 0; i < readers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						ctg.ds.Value(randClient)
					}
				}()
			}

			// 6 writers to modify existing values
			for i := 0; i < writers; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						randClient := clients[rand.Intn(clientLen)].staticServiceId
						ctg.ds.Add(randClient, time.Hour, valueGenerator())
					}
				}()
			}

			// add one and delete one
			for i := 0; i < deleter; i++ {
				go func() {
					defer writerWg.Done()
					for n := 0; n < ops; n++ {
						clientIndex := rand.Intn(clientLen)
						clientID := clients[clientIndex].staticServiceId
						ctg.ds.Delete(clientID)

						ctg.ds.Add(clientID, time.Hour, valueGenerator())
					}
				}()
			}
		}

		write()
		writerWg.Wait()
	}

}

// create a unique key for each client based on its memory address
func (c *client) generateRandId() {
	hash := md5.New()
	hash.Write([]byte(fmt.Sprintf("%v", &c)))
	c.staticServiceId = hex.EncodeToString(hash.Sum(nil))
}

func BenchmarkConcurrentMaps(b *testing.B) {
	var operations []int

	if !cacheWarmOnly {
		operations = []int{500, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 4500, 5000}
	} else {
		operations = []int{0}
	}

	var clients []client
	for i := 0; i < numClients; i++ {
		newClient := client{}
		newClient.generateRandId()
		clients = append(clients, newClient)
	}

	inputs := []struct {
		name           string
		implementation genericClient
	}{
		{
			name:           "Benchmark sync map implementation",
			implementation: syncMapImplementation{},
		},
		{
			name: "Benchmark concurrent-map implementation",
			implementation: concurrentMapImplementation{
				ds: cm.New(),
			},
		},
		{
			name: "Benchmark Beego cache",
			implementation: beegoCacheImplementation{
				ds: beego.NewMemoryCache(),
			},
		},
		{
			name: "Benchmark go-cache cache",
			implementation: goCacheImplementation{
				ds: cache.New(time.Duration(time.Hour*1), time.Duration(time.Hour*1)),
			},
		},
		{
			name: "Benchmark cache2go cache",
			implementation: cacheToGoImplementation{
				ds: cache2go.Cache("bm"),
			},
		},
	}

	for _, input := range inputs {
		for k := 0; k < repeatNTimes; k++ {
			for _, m := range operations {
				b.Run(fmt.Sprintf("%s/%d", input.name, b.N), func(b *testing.B) {
					for n := 0; n < b.N; n++ {
						input.implementation.run(clients, m, b)
					}

				})
			}
		}
	}

}
