package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/dawidof/home-services/internal/demo"
	"github.com/dawidof/home-services/internal/inventory"
)

// demoCmd seeds a small example inventory ("acme") into an empty database.
func demoCmd(args []string) {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	data := dataFlag(fs)
	fs.Parse(args)
	d := openDB(*data)
	defer d.Close()
	if err := demo.Seed(context.Background(), inventory.New(d)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("demo inventory seeded in %s: 2 hosts, 5 services, 1 project, 1 domain, 1 cost\n", *data)
	fmt.Println("start the server and open / — checks and favicons fill in on their own")
}
