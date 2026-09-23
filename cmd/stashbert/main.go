// Command stashbert is the StashBert server.
package main

import "fmt"

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fmt.Printf("stashbert %s\n", version)
}
