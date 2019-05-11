package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
)

var envListen = stringResolve("LISTEN_ADDR", ":8080")
var envIqdbAddr = stringResolve("IQDB_ADDR", "iqdb:5566")
var envServiceName = stringResolve("SERVICE_NAME", "iibooru")
var envTreshold = stringResolve("MATCH_TRESHOLD", "60")
var splitter *regexp.Regexp

func init() {
	splitter, _ = regexp.Compile("\\s+")
}

func stringResolve(name, def string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return def
}

func iqdbHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(1024 * 1024 * 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	var bss []byte
	h, ok := r.MultipartForm.File["file"]
	if !ok || len(h) != 1 {
		if url, ok := r.MultipartForm.Value["url"]; !ok || len(url) != 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		} else {
			req, err := http.NewRequest("GET", url[0], nil)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			body, err := req.GetBody()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			bs, err := ioutil.ReadAll(body)
			defer func() {
				_ = body.Close()
			}()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
				return
			}
			bss = bs
		}
	} else {
		f, err := h[0].Open()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		bs, err := ioutil.ReadAll(f)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		bss = bs
	}
	res, err := QueryData(envIqdbAddr, "0", 0, 10, bss)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	matches := &strings.Builder{}
	for _, r := range res {
		matches.Write([]byte(fmt.Sprintf("  <match id='%d' service='%s' sim='%f' width='%d' height='%d'><image id='%d'/></match>\n", r.ImgId, envServiceName, r.Score, r.Width, r.Height, r.ImgId)))
	}

	w.Header().Set("content-type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(fmt.Sprintf("<?xml version='1.0' encoding='UTF-8'?>\n<matches threshold='%s'>\n%s</matches>", envTreshold, matches)))
}

func main() {
	http.HandleFunc("/iqdb", iqdbHandler)
	err := http.ListenAndServe(envListen, nil)
	if err != nil {
		panic(err)
	}
}
