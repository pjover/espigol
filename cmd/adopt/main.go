// Command adopt transforms the legacy espigol-java SQLite database into the new
// espigol schema. One-off cutover tool; not part of the espigol binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pjover/espigol/cmd/adopt/transform"
)

func main() {
	from := flag.String("from", "", "path to the legacy espigol-java SQLite DB")
	to := flag.String("to", "", "destination path for the new espigol.db")
	force := flag.Bool("force", false, "overwrite the destination if it exists")
	flag.Parse()

	if *from == "" || *to == "" {
		log.Fatal("adopt: --from and --to are required")
	}
	if *force {
		_ = os.Remove(*to)
	}
	counts, err := transform.Run(context.Background(), *from, *to)
	if err != nil {
		log.Fatalf("adopt: %v", err)
	}
	fmt.Printf("adopted: %+v\n", counts)
}
