package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"io"
	"log"
	"net/http"

	// "net/url"
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	quic "github.com/lucas-clemente/quic-go"
	"golang.org/x/net/http2"

	// "github.com/lucas-clemente/quic-go/internal/protocol"

	"github.com/gabriel-vasile/mimetype"
	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

func sendTrainSignal(wg *sync.WaitGroup) {
	defer wg.Done()
	data := map[string]interface{}{
		"train_flag": true,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		fmt.Println("Error encoding JSON:", err)
		return
	}

	_, err = http.Post("http://10.0.0.20:8080/flag_training", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error sending POST request:", err)
	}
}
func main() {
	verbose := flag.Bool("v", false, "verbose")
	multipath := flag.Bool("m", false, "multipath")
	output := flag.String("o", "", "logging output")
	cache := flag.Bool("c", false, "cache handshake information")
	bindAddr := flag.String("b", "0.0.0.0", "bind address")
	pathScheduler := flag.String("ps", "LowLatency", "path scheduler")
	streamScheduler := flag.String("ss", "RoundRobin", "stream scheduler")
	browser := flag.String("bs", "safari", "Brower client")

	flag.Parse()
	urls_tmp := flag.Args()
	urls := []string{}
	// Open the file for reading
	file, err1 := os.Open(urls_tmp[0])
	if err1 != nil {
		fmt.Println("Error opening file:", err1)
		return
	}
	defer file.Close()

	// Create a new scanner to read the file
	scanner := bufio.NewScanner(file)

	// Read the file line by line
	for scanner.Scan() {
		line := scanner.Text()
		urls = append(urls, line)
	}
	// fmt.Println(urls)

	// Check for errors during scanning
	if err1 := scanner.Err(); err1 != nil {
		fmt.Println("Error reading file:", err1)
	}
	if *verbose {
		utils.SetLogLevel(utils.LogLevelDebug)
	} else {
		utils.SetLogLevel(utils.LogLevelInfo)
	}

	if *output != "" {
		logfile, err := os.Create(*output)
		if err != nil {
			panic(err)
		}
		defer logfile.Close()
		log.SetOutput(logfile)
	}

	quicConfig := &quic.Config{
		CreatePaths:     *multipath,
		CacheHandshake:  *cache,
		BindAddr:        *bindAddr,
		PathScheduler:   *pathScheduler,
		StreamScheduler: *streamScheduler,
	}

	// Using modified http API (allows http priorities)
	hclient := &h2quic.Client{
		Transport: h2quic.RoundTripper{QuicConfig: quicConfig, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}

	// Using standard (unmodified) http API
	// hclient := &http.Client{
	// 	Transport: &h2quic.RoundTripper{QuicConfig: quicConfig, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	// }

	priority := &http2.PriorityParam{
		Weight:    0x3,
		StreamDep: 0x3,
		Exclusive: false,
	}
	// fmt.Println(*pathScheduler, *streamScheduler, *browser)

	//Create first connection (GOAL: ignore initial time when statistics)
	var wgUrlsini sync.WaitGroup
	urlsini := [...]string{"https://10.0.0.20:6121/initialfiles/index1.html", "https://10.0.0.20:6121/initialfiles/index2.html", "https://10.0.0.20:6121/initialfiles/index3.html", "https://10.0.0.20:6121/initialfiles/index4.html"}
	utils.Infof("GET %s", urlsini[0])
	priority.Weight = 240 //maybe stream 5
	priority.StreamDep = 3
	_, errini := hclient.Get(urlsini[0], priority)
	if errini != nil {
		panic(errini)
	}
	utils.Infof("GET %s", urlsini[1])
	priority.Weight = 200 //maybe stream 7
	priority.StreamDep = 3
	_, errini = hclient.Get(urlsini[1], priority)
	if errini != nil {
		panic(errini)
	}
	utils.Infof("GET %s", urlsini[2])
	priority.Weight = 100 //maybe stream 9
	priority.StreamDep = 3
	_, errini = hclient.Get(urlsini[2], priority)
	if errini != nil {
		panic(errini)
	}
	utils.Infof("GET %s", urlsini[3])
	priority.Weight = 1 //maybe stream 11
	priority.StreamDep = 7
	_, errini = hclient.Get(urlsini[3], priority)
	if errini != nil {
		panic(errini)
	}
	wgUrlsini.Wait()

	//Main browser
	timeObject1 := []string{}
	byteObject1 := []string{}

	var wg sync.WaitGroup
	wg.Add(len(urls))
	for _, addr := range urls {
		utils.Infof("GET %s", addr)
		prioritycpy := &http2.PriorityParam{
			Weight:    0x3,
			StreamDep: 0x3,
			Exclusive: false,
		}

		prioritycpy.Weight, prioritycpy.StreamDep = getPrioritynew(*browser, addr)
		// fmt.Println("Weight: ", prioritycpy)

		go func(addr string, pri *http2.PriorityParam) {
			start := time.Now()
			rsp, err := hclient.Get(addr, pri)
			if err != nil {
				panic(err)
			}

			body := &bytes.Buffer{}
			_, err = io.Copy(body, rsp.Body)
			if err != nil {
				panic(err)
			}
			// fmt.Println(start)
			// elapsed := time.Since(start)
			// utils.Infof("%f", float64(elapsed.Nanoseconds())/1000000)
			elapsed := strconv.FormatFloat(time.Since(start).Seconds(), 'f', -1, 64)
			utils.Infof("%s", elapsed)
			utils.Infof("%d", body.Len())
			timeObject1 = append(timeObject1, elapsed)
			byteObject1 = append(byteObject1, strconv.FormatInt(int64(body.Len()), 10))

			wg.Done()
		}(addr, prioritycpy)
	}
	wg.Wait()
	file1, err1 := os.OpenFile("./logs/data-time.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err1 != nil {
		panic(err1)
	}
	defer file1.Close()

	writer := csv.NewWriter(file1)
	defer writer.Flush()
	err1 = writer.Write(timeObject1)
	if err1 != nil {
		panic(err1)
	}

	file2, err2 := os.OpenFile("./logs/data-byte.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err2 != nil {
		panic(err2)
	}
	defer file2.Close()

	writer2 := csv.NewWriter(file2)
	defer writer2.Flush()
	err2 = writer2.Write(byteObject1)
	if err2 != nil {
		panic(err2)
	}
	wg.Add(1)
	go sendTrainSignal(&wg)
	wg.Wait()

	file3, err3 := os.OpenFile("./logs/result.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err3 != nil {
		panic(err3)
	}
	defer file3.Close()

	writer3 := csv.NewWriter(file3)
	defer writer3.Flush()
	err3 = writer3.Write([]string{timeObject1[len(timeObject1)-1]})
	if err3 != nil {
		panic(err3)
	}
}

func containsAny(addr string, extensions []string) bool {
	for _, ext := range extensions {
		if strings.Contains(addr, ext) {
			return true
		}
	}
	return false
}

func getPrioritynew(browser, addr string) (weight uint8, streamDep uint32) {
	var htmlExtensions = []string{"text/html; charset=utf-8"}
	var cssExtensions = []string{"text/plain; charset=utf-8"}
	var imageExtensions = []string{"image/"}
	var fontExtensions = []string{"font/"}
	var xhrExtensions = []string{"xhr"}
	var jsExtensions = []string{"application/javascript"}

	addr_tmp := addr[23:]
	addr_tmp = "./www/" + addr_tmp
	mtype, _ := mimetype.DetectFile(addr_tmp)
	// fmt.Println(mtype.String(), mtype.Extension())
	check_type := mtype.String() + mtype.Extension()

	switch browser {
	/*
		Safari Weight
		pushed assets - 16, images - 8, font xhr - 16, js css - 24, html - 255
	*/
	case "safari":
		if containsAny(check_type, htmlExtensions) {
			weight = 255
		} else if containsAny(check_type, cssExtensions) {
			weight = 23
		} else if containsAny(check_type, imageExtensions) {
			weight = 7
		} else {
			weight = 15
		}
	/*
		FireFox Weight
		other, - 32/9, images font xhr - 42/11, css script - 32/7, html - 255/5
	*/
	case "firefox":
		if containsAny(check_type, htmlExtensions) {
			weight = 254
			streamDep = 5
		} else if containsAny(check_type, cssExtensions) || containsAny(check_type, jsExtensions) {
			weight = 31
			streamDep = 7
		} else if containsAny(check_type, imageExtensions) {
			weight = 41
			streamDep = 11
		} else if containsAny(check_type, fontExtensions) {
			weight = 21
			streamDep = 11
		} else {
			weight = 31
			streamDep = 9
		}
	case "chrome":
		if containsAny(check_type, htmlExtensions) {
			weight = 255
		} else if containsAny(check_type, cssExtensions) {
			weight = 255
		} else if containsAny(check_type, fontExtensions) {
			weight = 255
		} else if containsAny(check_type, imageExtensions) {
			weight = 146
		} else if containsAny(check_type, xhrExtensions) {
			weight = 219
		} else if containsAny(check_type, jsExtensions) {
			weight = 182
		} else {
			weight = 109
		}
	}

	return weight, streamDep
}

// func getPriority(browser, addr string) (weight uint8, streamDep uint32) {
// 	var htmlExtensions = []string{".html", ".htm", ".php", ".jsp", ".asp", ".aspx", ".shtml", ".xhtml"}
// 	var cssExtensions = []string{".css", ".scss"}
// 	var imageExtensions = []string{".ico", ".jpg", ".jpeg", ".png", ".svg", ".gif", ".bmp", ".tiff", ".tif", ".webp", ".raw", ".heic", ".heif", ".psd", ".eps"}
// 	var fontExtensions = []string{".ttf", ".eot", ".woff", ".woff2", ".otf", ".otc", ".ttc", ".pfb", ".pfm", ".pfa", ".fon"}
// 	var xhrExtensions = []string{".xhr"}
// 	var jsExtensions = []string{".js", ".mjs", ".ts"}

// 	switch browser {
// 	/*
// 		Safari Weight
// 		pushed assets - 16, images - 8, font xhr - 16, js css - 24, html - 255
// 	*/
// 	case "safari":
// 		if containsAny(addr, htmlExtensions) {
// 			weight = 255
// 		} else if containsAny(addr, cssExtensions) {
// 			weight = 24
// 		} else if containsAny(addr, imageExtensions) {
// 			weight = 8
// 		} else {
// 			weight = 16
// 		}
// 	/*
// 		FireFox Weight
// 		other, - 32/9, images font xhr - 42/11, css script - 32/7, html - 255/5
// 	*/
// 	case "firefox":
// 		if containsAny(addr, htmlExtensions) {
// 			weight = 255
// 			streamDep = 5
// 		} else if containsAny(addr, cssExtensions) || containsAny(addr, jsExtensions) {
// 			weight = 32
// 			streamDep = 7
// 		} else if containsAny(addr, imageExtensions) || containsAny(addr, fontExtensions) {
// 			weight = 42
// 			streamDep = 11
// 		} else {
// 			weight = 32
// 			streamDep = 9
// 		}
// 	case "chrome":
// 		if containsAny(addr, htmlExtensions) {
// 			weight = 255
// 		} else if containsAny(addr, cssExtensions) {
// 			weight = 255
// 		} else if containsAny(addr, fontExtensions) {
// 			weight = 255
// 		} else if containsAny(addr, imageExtensions) {
// 			weight = 147
// 		} else if containsAny(addr, xhrExtensions) {
// 			weight = 220
// 		} else if containsAny(addr, jsExtensions) {
// 			weight = 183
// 		} else {
// 			weight = 110
// 		}
// 	}

// 	return weight, streamDep
// }
