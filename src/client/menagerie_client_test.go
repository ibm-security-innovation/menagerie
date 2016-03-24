package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	//	"gitlab.haifa.ibm.com/trail/obc-mngr.git"

	"github.com/golang/glog"
	"github.com/zenazn/goji/web"
)

const (
	MenagerieBase = "http://127.0.0.1:3012"
)

var (
	server *httptest.Server
)

type ExpectedCallback func(requestData map[string]interface{}) bool

func fatalWithLog(t *testing.T, strs ...interface{}) {
	glog.V(0).Infoln(strs)
	t.Fatal(strs)
}

func setup(handlers [3]func(web.C, http.ResponseWriter, *http.Request)) *Client {
	mux := web.New()
	mux.Post("/engine/upload", handlers[0])
	mux.Get("/result/:id", handlers[1])
	mux.Get("/link/:id", handlers[2])

	mngr := NewClient("", "engine", ioutil.Discard)
	mngr.cfg.MenagerieBase = MenagerieBase
	glog.V(0).Infoln("setup", server)
	if server != nil {
		server.Close()
		glog.V(0).Infoln("closing server")
		time.Sleep(time.Second * 2) // this is kludgy but easy
	}
	server = httptest.NewUnstartedServer(mux)
	go func() { server.Start() }()
	return mngr
}

func uploader(t *testing.T, expected ExpectedCallback, jobid int) func(web.C, http.ResponseWriter, *http.Request) {
	return func(c web.C, w http.ResponseWriter, r *http.Request) {
		glog.V(0).Infoln("uploadHandler")
		reader, err := r.MultipartReader()
		if err != nil {
			fatalWithLog(t, "can't get multipartreader", err)
		}
		for {
			p, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				fatalWithLog(t, "nextpart", err)
			}

			part, err := ioutil.ReadAll(p)
			if err != nil {
				fatalWithLog(t, "can't read part")
			}
			var requestData map[string]interface{}
			err = json.Unmarshal(part, &requestData)
			if err != nil {
				fatalWithLog(t, "can't parse part", err, string(part))
			}

			if !expected(requestData) {
				fatalWithLog(t, "expected failed", string(part))
			}
		}
		w.Write([]byte(fmt.Sprintf("{\"jobid\": \"%d\"}", jobid)))
	}

}

func TestSimple(t *testing.T) {

	var uploadHandler = uploader(t,
		func(requestData map[string]interface{}) bool {
			return requestData["foo"] == "bar"
		},
		1833)

	var resultHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		if c.URLParams["id"] != "1833" {
			fatalWithLog(t, "1833 expected")
		}

		w.Write([]byte(`{"status": "Success", "summary": "short and sweet", "link": "/link/14"}`))
	}
	var linkHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		if c.URLParams["id"] != "14" {
			fatalWithLog(t, "14 expected")
		}
		w.Write([]byte(`Hello cruel world`))
	}
	mngr := setup([3]func(web.C, http.ResponseWriter, *http.Request){uploadHandler, resultHandler, linkHandler})

	res := mngr.doit([]byte("{ \"foo\": \"bar\"}"))
	if res.Error != "" {
		fatalWithLog(t, "no error expected")
	}
	if res.Summary != "short and sweet" {
		fatalWithLog(t, "no error expected")
	}

	time.Sleep(time.Second * 3) // this is kludgy but easy
	glog.V(0).Infoln("bye")
}

func TestRetries(t *testing.T) {

	var retries = 0
	var uploadHandler = uploader(t,
		func(requestData map[string]interface{}) bool {
			return requestData["chano"] == "pozo"
		},
		925)

	var resultHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		if c.URLParams["id"] != "925" {
			fatalWithLog(t, "925 expected")
		}

		retries++
		if retries < 3 {
			w.Write([]byte(`{"status": "Running"}`))
		} else {
			w.Write([]byte(`{"status": "Success", "summary": "", "link": "/link/14"}`))
		}
	}
	var linkHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		if c.URLParams["id"] != "14" {
			fatalWithLog(t, "14 expected")
		}
		w.Write([]byte(`Hello cruel world`))
	}

	mngr := setup([3]func(web.C, http.ResponseWriter, *http.Request){uploadHandler, resultHandler, linkHandler})
	mngr.cfg.PollingAttempts = 5
	res := mngr.doit([]byte("{ \"chano\": \"pozo\"}"))
	if res.Link != "/link/14" {
		fatalWithLog(t, "no link found")
	}

	time.Sleep(time.Second * 3) // this is kludgy but easy
}

func TestTooManyRetries(t *testing.T) {

	var retries = 0
	var uploadHandler = uploader(t,
		func(requestData map[string]interface{}) bool {
			return requestData["chano"] == "pozo"
		},
		925)

	var resultHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		if c.URLParams["id"] != "925" {
			fatalWithLog(t, "925 expected")
		}

		retries++
		if retries < 4 {
			w.Write([]byte(`{"status": "Running"}`))
		} else {
			fatalWithLog(t, "Shouldn't have landed here")
		}
	}
	var linkHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		fatalWithLog(t, "Shouldn't have landed here")
	}

	mngr := setup([3]func(web.C, http.ResponseWriter, *http.Request){uploadHandler, resultHandler, linkHandler})
	mngr.cfg.PollingAttempts = 2
	res := mngr.doit([]byte("{ \"chano\": \"pozo\"}"))
	if res.Status != "Running" {
		fatalWithLog(t, "Should have been running for ever")
	}

	time.Sleep(time.Second * 3) // this is kludgy but easy
}

func TestFailedJob(t *testing.T) {
	var retries = 0
	var uploadHandler = uploader(t,
		func(requestData map[string]interface{}) bool {
			return requestData["chano"] == "pozo"
		},
		925)

	var resultHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		if c.URLParams["id"] != "925" {
			fatalWithLog(t, "925 expected")
		}

		retries++
		if retries < 3 {
			w.Write([]byte(`{"status": "Running"}`))
		} else {
			w.Write([]byte(`{"status": "Failed", "": "", "link": "/link/53"}`))
		}
	}
	var linkHandler = func(c web.C, w http.ResponseWriter, r *http.Request) {
		fatalWithLog(t, "Shouldn't have landed here")
	}

	mngr := setup([3]func(web.C, http.ResponseWriter, *http.Request){uploadHandler, resultHandler, linkHandler})
	mngr.cfg.PollingAttempts = 8
	res := mngr.doit([]byte("{ \"chano\": \"pozo\"}"))
	if res.Status != "Failed" {
		fatalWithLog(t, "Should have failed")
	}

	time.Sleep(time.Second * 3) // this is kludgy but easy
}
