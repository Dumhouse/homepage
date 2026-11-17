package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/dawidof/home-services/internal/demo"
	"github.com/dawidof/home-services/internal/inventory"
)

// demoCmd seeds a fictional example lab (hosts, services, projects, domains, costs, discovery) into an empty database.
func demoCmd(args []string) {
	fs := flag.NewFlagSet("demo", flag.ExitOnError)
	data := dataFlag(fs)
	fs.Parse(args)
	d := openDB(*data)
	defer d.Close()
	if err := demo.Seed(context.Background(), inventory.New(d)); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("demo lab seeded in %s: 11 hosts, 3 projects, 3 domains, costs and 3 new devices in Discovery\n", *data)
	fmt.Println("start the server and open / — checks and favicons fill in on their own")
}
