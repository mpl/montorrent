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
)

func usage() {
	flag.PrintDefaults()
	os.Exit(2)
}

type status struct {
	name string
	bytesDone int
	bytesTotal int
	percentDone int
}

func rpc(args ...string) ([]byte, error) {
	var answer []byte
	for i:=0; i<numRetry; i++ {
		cmd := exec.Command("rtorrentrpc", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// TODO(mpl): diagnose better the error and return early if it's not the expected EOF one.
			log.Printf("ignoring error: %v", err)
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

func downloadList() ([]string, error) {
	answer, err := rpc(*scgi, "download_list", "")
	if err != nil {
		return nil, err
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

func torrentName(torrentHash string) (string, error) {
	answer, err := rpc(*scgi, "d.name", torrentHash)
	if err != nil {
		return "", err
	}
	var list []string
	scanner := bufio.NewScanner(bytes.NewReader(answer))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "<param><value><string>") || !strings.HasSuffix(line, "</string></value></param>") {
			continue
		}
		println(line)
		list = append(list, strings.TrimSuffix(strings.TrimPrefix(line, "<param><value><string>"), "</string></value></param>"))
		break
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("could not scan answer: %v", err)
	}
	if len(list) == 0 {
		return "", errors.New("name not found")
	}
	return list[0], nil
}

func torrentStatus(torrentHash string) (*status, error) {
	println("TORRENTSTATUS")
	name, err := torrentName(torrentHash)
	if err != nil {
		return nil, err
	}
	return &status {
		name: name,
	}, nil
}

func main() {
	flag.Usage = usage
	flag.Parse()


	list, err := downloadList()
	if err != nil {
		log.Fatal(err)
	}
	allStatus := make(map[string]*status)
	for _,v := range list {
		tStatus, err := torrentStatus(v)
		if err != nil {
			log.Fatal(err)
		}
		allStatus[v] = tStatus
	}
	for _,v := range allStatus {
		println(v.name)
	}
}