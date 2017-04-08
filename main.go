// 2015 - Mathieu Lonjaret

// The montorrent program serves the status of rtorrent. It uses
// github.com/mpl/rtorrentrpc to query rtorrent.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mpl/basicauth"
	"github.com/mpl/simpletls"
)

var (
	cache   = flag.Int("cache", 0, "cache the status for that many seconds. 0 means no caching")
	help    = flag.Bool("h", false, "show this help")
	host    = flag.String("host", "localhost:8080", "listening host:port")
	scgi    = flag.String("scgi", "localhost:5000", "host:port for rtorrent's scgi.")
	flagTLS = flag.Bool("tls", false, `For https.`)

	userpass = flag.String("userpass", "", "optional username:password protection")
	verbose  = flag.Bool("v", false, "verbose")
)

const (
	numRetry   = 20
	retryDelay = 2 * time.Second
	idstring   = "http://golang.org/pkg/http/#ListenAndServe"
)

func usage() {
	flag.PrintDefaults()
	os.Exit(2)
}

func rpc(args ...string) ([]byte, error) {
	var answer []byte
	for i := 0; i < numRetry; i++ {
		cmd := exec.Command("rtorrentrpc", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// TODO(mpl): diagnose better the error and return early if it's not the expected EOF one.
			if *verbose {
				log.Printf("ignoring error: %v", err)
			}
			continue
		}
		if len(output) > 0 {
			answer = output
			break
		}
		time.Sleep(retryDelay)
	}
	if len(answer) == 0 {
		return nil, fmt.Errorf("empty answer for %v", args)
	}
	return answer, nil
}

func scanAnswer(answer []byte, prefix, suffix string) ([]string, error) {
	var list []string
	scanner := bufio.NewScanner(bytes.NewReader(answer))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
			continue
		}
		list = append(list, strings.TrimSuffix(strings.TrimPrefix(line, prefix), suffix))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not scan answer: %v", err)
	}
	return list, nil
}

func downloadList() ([]string, error) {
	answer, err := rpc(*scgi, "download_list", "")
	if err != nil {
		return nil, err
	}
	return scanAnswer(answer, "<value><string>", "</string></value>")
}

func torrentName(torrentHash string) (string, error) {
	answer, err := rpc(*scgi, "d.name", torrentHash)
	if err != nil {
		return "", err
	}
	list, err := scanAnswer(answer, "<param><value><string>", "</string></value></param>")
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", fmt.Errorf("%v: name not found", torrentHash)
	}
	return list[0], nil
}

func bytesDone(torrentHash string) (int, error) {
	var n int
	answer, err := rpc(*scgi, "d.get_bytes_done", torrentHash)
	if err != nil {
		return n, err
	}
	list, err := scanAnswer(answer, "<param><value><i8>", "</i8></value></param>")
	if err != nil {
		return n, err
	}
	if len(list) == 0 {
		return n, fmt.Errorf("%v: bytes_done not found", torrentHash)
	}
	n, err = strconv.Atoi(list[0])
	if err != nil {
		return n, fmt.Errorf("could not convert bytes_done to int: %v", err)
	}
	return n, nil
}

func bytesLeft(torrentHash string) (int, error) {
	var n int
	answer, err := rpc(*scgi, "d.get_left_bytes", torrentHash)
	if err != nil {
		return n, err
	}
	list, err := scanAnswer(answer, "<param><value><i8>", "</i8></value></param>")
	if err != nil {
		return n, err
	}
	if len(list) == 0 {
		return n, fmt.Errorf("%v: bytes_left not found", torrentHash)
	}
	n, err = strconv.Atoi(list[0])
	if err != nil {
		return n, fmt.Errorf("could not convert bytes_left to int: %v", err)
	}
	return n, nil
}

type torrentStatus struct {
	Name        string
	BytesDone   int
	BytesLeft   int
	BytesTotal  int
	PercentDone int
}

func getStatus(torrentHash string) (*torrentStatus, error) {
	name, err := torrentName(torrentHash)
	if err != nil {
		return nil, err
	}
	nDone, err := bytesDone(torrentHash)
	if err != nil {
		return nil, err
	}
	nLeft, err := bytesLeft(torrentHash)
	if err != nil {
		return nil, err
	}
	total := nDone + nLeft // yay, super precision!!
	percent := nDone * 100 / total
	return &torrentStatus{
		Name:        name,
		BytesDone:   nDone,
		BytesLeft:   nLeft,
		BytesTotal:  total,
		PercentDone: percent,
	}, nil
}

func makeHandler(fn func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e, ok := recover().(error); ok {
				http.Error(w, e.Error(), http.StatusInternalServerError)
				return
			}
		}()
		w.Header().Set("Server", idstring)
		if up.IsAllowed(r) {
			fn(w, r)
		} else {
			basicauth.SendUnauthorized(w, r, "montorrent")
		}
	}
}

func serveJSON(w http.ResponseWriter, r *http.Request, data []byte) {
	w.Header().Set("Content-Type", "text/javascript")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)+1))
	w.WriteHeader(200)
	w.Write(data)
	w.Write([]byte("\n"))
}

func serveStatus(w http.ResponseWriter, r *http.Request) {
	if *cache > 0 {
		lastUpdateMu.RLock()
		// TODO(mpl): maybe use a checkLastModified. pull it out of simpleHttpd as a lib?
		if time.Now().Before(lastUpdate.Add(time.Duration(*cache) * time.Second)) {
			defer lastUpdateMu.RUnlock()
			statusMu.RLock()
			defer statusMu.RUnlock()
			data, err := json.MarshalIndent(status, "", "	")
			if err != nil {
				if *verbose {
					log.Printf("could not get json encode: %v", err)
				}
				http.Error(w, "could not json encode", http.StatusInternalServerError)
				return
			}
			serveJSON(w, r, data)
			return
		}
		lastUpdateMu.RUnlock()
	}
	list, err := downloadList()
	if err != nil {
		if *verbose {
			log.Printf("could not get torrents list: %v", err)
		}
		http.Error(w, "could not get torrents list", http.StatusInternalServerError)
		return
	}
	allStatus := make(map[string]*torrentStatus)
	for _, v := range list {
		tStatus, err := getStatus(v)
		if err != nil {
			if *verbose {
				log.Printf("could not get torrent status: %v", err)
			}
			http.Error(w, "could not get torrent status", http.StatusInternalServerError)
			return
		}
		allStatus[v] = tStatus
	}
	lastUpdateMu.Lock()
	defer lastUpdateMu.Unlock()
	statusMu.Lock()
	defer statusMu.Unlock()
	data, err := json.MarshalIndent(allStatus, "", "	")
	if err != nil {
		if *verbose {
			log.Printf("could not get json encode: %v", err)
		}
		http.Error(w, "could not json encode", http.StatusInternalServerError)
		return
	}
	status = allStatus
	lastUpdate = time.Now()
	serveJSON(w, r, data)
}

var (
	up *basicauth.UserPass

	statusMu sync.RWMutex
	status   map[string]*torrentStatus

	lastUpdateMu sync.RWMutex
	lastUpdate   time.Time
)

func main() {
	flag.Usage = usage
	flag.Parse()

	var err error
	up, err = basicauth.New(*userpass)
	if err != nil {
		log.Fatal(err)
	}
	if *verbose {
		basicauth.Verbose = true
	}

	http.Handle("/", makeHandler(serveStatus))

	if !*flagTLS && *simpletls.FlagAutocert {
		*flagTLS = true
	}

	var listener net.Listener
	if *flagTLS {
		listener, err = simpletls.Listen(*host)
	} else {
		listener, err = net.Listen("tcp", *host)
	}
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", *host, err)
	}

	log.Fatal(http.Serve(listener, nil))
}
