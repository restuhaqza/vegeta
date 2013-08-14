package main

import (
	"flag"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"syscall"
	"time"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	var (
		// Flags
		rate     = flag.Uint("rate", 50, "Requests per second")
		targetsf = flag.String("targets", "targets.txt", "Targets file")
		ordering = flag.String("ordering", "random", "Attack ordering [sequential, random]")
		duration = flag.Duration("duration", 10*time.Second, "Duration of the test")
		reporter = flag.String("reporter", "text", "Reporter to use [text]")
		output   = flag.String("output", "stdout", "Reporter output file")
	)
	flag.Parse()

	if flag.NFlag() == 0 {
		flag.Usage()
		return
	}

	if err := increaseResourceLimits(); err != nil {
		log.Fatalf("Couldn't increase resource limits: %s", err)
	}

	// Validate rate argument
	if *rate == 0 {
		log.Fatal("rate can't be zero")
	}
	// Parse targets file
	targets, err := NewTargetsFromFile(*targetsf)
	if err != nil {
		log.Fatal(err)
	}

	// Parse ordering argument
	if *ordering == "random" {
		rand.Seed(time.Now().UnixNano())
	} else if *ordering != "sequential" {
		log.Fatalf("Unknown ordering %s", *ordering)
	}

	// Parse duration
	if *duration == 0 {
		log.Fatal("Duration provided is invalid")
	}

	// Parse reporter
	var rep Reporter
	switch *reporter {
	case "text":
		rep = NewTextReporter()
	default:
		log.Println("reporter provided is not supported. using text")
		rep = NewTextReporter()
	}

	var out io.WriteCloser
	switch *output {
	case "stdout":
		out = os.Stdout
	default:
		out, err = os.Create(*output)
		if err != nil {
			log.Fatalf("Couldn't open `%s` for writing report: %s", *output, err)
		}
		defer out.Close()
	}

	log.Printf("Vegeta is attacking %d targets in %s order for %s...\n", len(targets), *ordering, *duration)
	attack(targets, *ordering, *rate, *duration, rep)
	log.Println("Done!")

	// Report results!
	log.Printf("Writing report to '%s'...", *output)
	if rep.Report(out) != nil {
		log.Println("Failed to report!")
	}
}

func attack(targets Targets, ordering string, rate uint, duration time.Duration, rep Reporter) {
	hits := make(chan *http.Request, rate*uint((duration).Seconds()))
	defer close(hits)
	responses := make(chan *Response, cap(hits))
	defer close(responses)
	client := NewClient(rate)
	go client.Drill(hits, responses) // Attack!
	for i, idxs := 0, targets.Iter(ordering); i < cap(hits); i++ {
		hits <- targets[idxs[i%len(idxs)]]
	}
	// Wait for all requests to finish
	for i := 0; i < cap(responses); i++ {
		rep.Add(<-responses)
	}
}

func increaseResourceLimits() error {
	limit := &syscall.Rlimit{Cur: syscall.RLIM_INFINITY}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, limit); err != nil {
		return err
	}
	return nil
}