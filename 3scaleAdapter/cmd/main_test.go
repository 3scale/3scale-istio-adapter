// +build integration

package main

import (
	"os"
	"testing"
)

func TestRunMain(t *testing.T) {
	os.Setenv("THREESCALE_LISTEN_ADDR", "3333")
	main()
}
