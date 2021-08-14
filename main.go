package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	// Parse config.
	if len(os.Args) != 2 {
		log.Fatal("please specify the path to a config file, an example config is available at https://github.com/beefsack/git-mirror/blob/master/example-config.toml")
	}
	cfg, repos, err := parseConfig(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(cfg.BasePath, 0755); err != nil {
		log.Fatalf("failed to create %s, %s", cfg.BasePath, err)
	}
	// Run background threads to keep mirrors up to date.
	Counter, _ := strconv.Atoi(cfg.Counter)
	for _, r := range repos {
		go func(r repo) {
			for {
				if Counter > 0 {
					Counter--
					fmt.Println("No. of Available Threads :", Counter)
					//fmt.Println("counter before",Counter)
					log.Printf("updating %s", r.Name)
					if err := mirror(cfg, r); err != nil {
						Retries, _ := strconv.Atoi(cfg.Retries)
						for i := 1; i < Retries+1; i++ {
							time.Sleep(time.Duration(i) * time.Second)
							log.Printf("error updating %d, %s, %s", i, r.Name, err)
							if err := mirror(cfg, r); err == nil {
								break
							}
						}
					} else {
						log.Printf("updated %s", r.Name)
					}
					Counter++
					//fmt.Println("counter after",Counter)
					time.Sleep(r.Interval.Duration)
				} else {
					time.Sleep(30 * time.Second)
				}
			}
		}(r)
	}

	// Run HTTP server to serve mirrors.
	flag.Parse()

	http.Handle("/", http.StripPrefix("/", requestHandler()))
	//http.Handle("/", http.FileServer(http.Dir(cfg.BasePath)))
	log.Printf("starting web server on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, nil); err != nil {
		log.Fatalf("failed to start server, %s", err)
	}
}
