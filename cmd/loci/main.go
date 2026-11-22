// Command loci hands an agent's session over at git boundaries.
package main

import (
	"os"

	"github.com/cmj0121/loci/internal/loci"
)

func main() {
	os.Exit(loci.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
