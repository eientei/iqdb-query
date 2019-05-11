package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
)

const (
	IqdbCodeReady            = 000
	IqdbCodeInfo             = 100
	IqdbCodeInfoProperty     = 101
	IqdbCodeDbEntry          = 102
	IqdbCodeQueryResult      = 200
	IqdbCodeMultiQueryResult = 201
	IqdbCodeDupQueryResult   = 202
	IqdbCodeError            = 300
	IqdbCodeException        = 301
	IqdbCodeFatal            = 302
)

var ErrInvalidResponse = errors.New("invalid response")

func NewIqdbClient(remote string) (*IqdbClient, error) {
	conn, err := net.Dial("tcp", envIqdbAddr)
	if err != nil {
		return nil, err
	}
	client := &IqdbClient{
		Conn:  conn,
		Resps: make(chan IqdbResp, 4),
		Mutex: &sync.Mutex{},
	}

	go iqdbRespParser(client.Conn, client.Resps)

	err = client.awaitReady()
	if err != nil {
		return nil, err
	}

	return client, nil
}

type IqdbClient struct {
	Conn  net.Conn
	Resps chan IqdbResp
	Mutex *sync.Mutex
}

func (iqdb *IqdbClient) awaitReady() error {
	resp := <-iqdb.Resps
	switch resp.(type) {
	case *IqdbRespReady:
	default:
		_ = iqdb.Conn.Close()
		return ErrInvalidResponse
	}
	return nil
}

type QueryResult struct {
	ImgId  uint64
	Score  float64
	Width  int64
	Height int64
}

func (iqdb *IqdbClient) readResults() ([]*QueryResult, error) {
	var results []*QueryResult
	for {
		resp := <-iqdb.Resps
		switch ev := resp.(type) {
		case *IqdbRespQueryResult:
			results = append(results, &QueryResult{
				ImgId:  ev.ImgId,
				Score:  ev.Score,
				Width:  ev.Width,
				Height: ev.Height,
			})
		case *IqdbRespMultiQueryResult:
			results = append(results, &QueryResult{
				ImgId:  ev.ImgId,
				Score:  ev.Score,
				Width:  ev.Width,
				Height: ev.Height,
			})
		case *IqdbRespInfo, *IqdbRespInfoProperty:
		case *IqdbRespError:
			return nil, errors.New(fmt.Sprintf("error: %s", ev.Text))
		case *IqdbRespException:
			return nil, errors.New(fmt.Sprintf("exception %s: %s", ev.Name, ev.Text))
		case *IqdbRespFatal:
			return nil, errors.New(fmt.Sprintf("fatal %s: %s", ev.Name, ev.Text))
		case *IqdbRespReady:
			return results, nil
		default:
			_ = iqdb.Conn.Close()
			return nil, ErrInvalidResponse
		}
	}
}

func (iqdb *IqdbClient) QueryFilename(dbid string, flags, numres int, filename string) ([]*QueryResult, error) {
	iqdb.Mutex.Lock()
	defer iqdb.Mutex.Unlock()

	_, err := iqdb.Conn.Write([]byte(fmt.Sprintf("query %s %d %d %s\n", dbid, flags, numres, filename)))
	if err != nil {
		return nil, err
	}
	return iqdb.readResults()
}

func (iqdb *IqdbClient) QueryData(dbid string, flags, numres int, data []byte) ([]*QueryResult, error) {
	iqdb.Mutex.Lock()
	defer iqdb.Mutex.Unlock()

	_, err := iqdb.Conn.Write([]byte(fmt.Sprintf("query %s %d %d :%d\n", dbid, flags, numres, len(data))))
	if err != nil {
		return nil, err
	}
	_, err = iqdb.Conn.Write(data)
	if err != nil {
		return nil, err
	}
	return iqdb.readResults()
}

func QueryFilename(remote string, dbid string, flags, numres int, filename string) ([]*QueryResult, error) {
	client, err := NewIqdbClient(remote)
	if err != nil {
		return nil, err
	}
	res, err := client.QueryFilename(dbid, flags, numres, filename)
	_ = client.Conn.Close()
	return res, err
}

func QueryData(remote string, dbid string, flags, numres int, data []byte) ([]*QueryResult, error) {
	client, err := NewIqdbClient(remote)
	if err != nil {
		return nil, err
	}
	res, err := client.QueryData(dbid, flags, numres, data)
	_ = client.Conn.Close()
	return res, err
}

func iqdbRespParser(conn net.Conn, results chan IqdbResp) {
	rdr := bufio.NewReader(conn)
	var line string
	for {
		part, prefix, err := rdr.ReadLine()
		if err != nil {
			return
		}
		line += string(part)
		if !prefix {
			parts := splitter.Split(line, -1)
			line = ""
			if len(parts) < 1 {
				continue
			}
			code, err := strconv.ParseUint(parts[0], 10, 64)
			if err != nil {
				continue
			}
			switch code {
			case IqdbCodeReady:
				results <- &IqdbRespReady{}
			case IqdbCodeInfo:
				results <- &IqdbRespInfo{Text: strings.Join(parts[1:], " ")}
			case IqdbCodeInfoProperty:
				if len(parts) < 2 {
					continue
				}
				kv := strings.Split(parts[1], "=")
				if len(kv) < 2 {
					continue
				}
				results <- &IqdbRespInfoProperty{Key: kv[0], Value: kv[1]}
			case IqdbCodeDbEntry:
				if len(parts) < 3 {
					continue
				}
				results <- &IqdbRespDbEntry{DbId: parts[1], DbFile: parts[2]}
			case IqdbCodeQueryResult:
				if len(parts) < 5 {
					continue
				}
				imgid, err := strconv.ParseUint(parts[1], 10, 64)
				if err != nil {
					continue
				}
				score, err := strconv.ParseFloat(parts[2], 64)
				if err != nil {
					continue
				}
				width, err := strconv.ParseInt(parts[3], 10, 64)
				if err != nil {
					continue
				}
				height, err := strconv.ParseInt(parts[4], 10, 64)
				if err != nil {
					continue
				}
				results <- &IqdbRespQueryResult{ImgId: imgid, Score: score, Width: width, Height: height}
			case IqdbCodeMultiQueryResult:
				if len(parts) < 6 {
					continue
				}
				imgid, err := strconv.ParseUint(parts[2], 10, 64)
				if err != nil {
					continue
				}
				score, err := strconv.ParseFloat(parts[3], 64)
				if err != nil {
					continue
				}
				width, err := strconv.ParseInt(parts[4], 10, 64)
				if err != nil {
					continue
				}
				height, err := strconv.ParseInt(parts[5], 10, 64)
				if err != nil {
					continue
				}
				results <- &IqdbRespMultiQueryResult{DbId: parts[1], ImgId: imgid, Score: score, Width: width, Height: height}
			case IqdbCodeError:
				results <- &IqdbRespError{Text: strings.Join(parts[1:], " ")}
			case IqdbCodeException:
				if len(parts) < 2 {
					continue
				}
				results <- &IqdbRespException{Name: parts[2], Text: strings.Join(parts[2:], " ")}
			case IqdbCodeFatal:
				if len(parts) < 2 {
					continue
				}
				results <- &IqdbRespFatal{Name: parts[2], Text: strings.Join(parts[2:], " ")}
			}
		}
	}
}

type IqdbResp interface {
	Code() int
}

type IqdbRespReady struct{}

func (resp *IqdbRespReady) Code() int {
	return IqdbCodeReady
}

type IqdbRespInfo struct {
	Text string
}

func (resp *IqdbRespInfo) Code() int {
	return IqdbCodeInfo
}

type IqdbRespInfoProperty struct {
	Key   string
	Value string
}

func (resp *IqdbRespInfoProperty) Code() int {
	return IqdbCodeInfoProperty

}

type IqdbRespDbEntry struct {
	DbId   string
	DbFile string
}

func (resp *IqdbRespDbEntry) Code() int {
	return IqdbCodeDbEntry
}

type IqdbRespQueryResult struct {
	ImgId  uint64
	Score  float64
	Width  int64
	Height int64
}

func (resp *IqdbRespQueryResult) Code() int {
	return IqdbCodeQueryResult
}

type IqdbRespMultiQueryResult struct {
	DbId   string
	ImgId  uint64
	Score  float64
	Width  int64
	Height int64
}

func (resp *IqdbRespMultiQueryResult) Code() int {
	return IqdbCodeMultiQueryResult
}

type IqdbRespDup struct {
	ImgId      uint64
	Similarity float64
}

type IqdbRespDupQueryResult struct {
	OrigImgId uint64
	Deviation float64
	Dups      []*IqdbRespDup
}

func (resp *IqdbRespDupQueryResult) Code() int {
	return IqdbCodeDupQueryResult
}

type IqdbRespError struct {
	Text string
}

func (resp *IqdbRespError) Code() int {
	return IqdbCodeError
}

type IqdbRespException struct {
	Name string
	Text string
}

func (resp *IqdbRespException) Code() int {
	return IqdbCodeException
}

type IqdbRespFatal struct {
	Name string
	Text string
}

func (resp *IqdbRespFatal) Code() int {
	return IqdbCodeFatal
}
