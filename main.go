// 2015 - Mathieu Lonjaret

// The montorrent program serves the status of rtorrent. It uses
// github.com/mpl/rtorrentrpc to query rtorrent.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	scgi = flag.String("scgi", "localhost:5000", "host:port for rtorrent's scgi.")
	host = flag.String("host", "localhost:8080", "where to serve the status")
)

const (
	numRetry = 20
	retryDelay = 2 * time.Second
	rpc = "rtorrentrpc"
)

func usage() {
	flag.PrintDefaults()
	os.Exit(2)
}

type status struct {
	bytesDone int
	bytesTotal int
	percentDone int
}

func downloadList() ([]string, error) {
	var answer []byte
	for i:=0; i<numRetry; i++ {
		cmd := exec.Command(rpc, *scgi, "download_list", "")
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		if len(output) > 0 {
			answer = output
			break
		}
		time.Sleep(retryDelay)
	}
	if len(answer) == 0 {
		return nil, errors.New("empty answer")
	}
	var list []string
	scanner := bufio.NewScanner(bytes.NewReader(answer))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "<value><string>") || !strings.HasSuffix(line, "</string></value>") {
			continue
		}
		list = append(list, strings.TrimSuffix(strings.TrimPrefix(line, "<value><string>"), "</string></value>"))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not scan answer: %v", err)
	}
	return list, nil
}

func main() {
	flag.Usage = usage
	flag.Parse()


	list, err := downloadList()
	if err != nil {
		log.Fatal(err)
	}
	for _,v := range list {
		println(v)
	}
}
