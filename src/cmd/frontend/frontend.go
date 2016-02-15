//
// Copyright 2015 IBM Corp. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
package main

import (
	"bufio"
	"cfg"
	"crypto/sha1"
	"db"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/streadway/amqp"
)

var (
	addr  = flag.String("listen", "127.0.0.1:8080", "address to bind to")
	store = flag.String("store", "", "location to store files")
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const link = "/link/"

var engineNames = []string{}

func seed() {
	rand.Seed(time.Now().UnixNano())
}

func init() {
	seed()
}

func RandStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func error500(w http.ResponseWriter) {
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

type queueServer struct {
	cfg.Engine
}

func getJobDir(jid int64) string {
	return filepath.Join(*store, fmt.Sprint(jid))
}

func openJobFile(jid int64, name string) (*os.File, error) {
	return os.OpenFile(filepath.Join(getJobDir(jid), name), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0666)
}

func createJobInputFile(jid int64) (*os.File, error) {
	if err := os.Mkdir(getJobDir(jid), 0777); err != nil {
		return nil, err
	}
	return openJobFile(jid, "input")
}

func logPanic() {
	if r := recover(); r != nil {
		glog.Errorln("Panic:", r, string(debug.Stack()))
	}
}

// TODO pool connections

// TODO gc old files
type UploadRensponse struct {
	JobId string `json:"jobid"`
}

func (s *queueServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	defer logPanic()

	jid, err := db.JobCreate(s.Name)
	if err != nil {
		glog.Errorln("Couldn't create job", err)
		error500(w)
		return
	}

	if err = s.doUpload(w, r, jid); err != nil {
		if err = db.JobSetError(jid, "Error starting job"); err != nil {
			glog.Errorf("Error setting job %d error: %s", jid, err)
		}
	} else {
		if err = db.JobSetStarted(jid); err != nil {
			glog.Errorf("Error setting job %d to running: %s", jid, err)
		}
	}
	glog.Infof("Job %d (%s) created successfully", jid, s.Name)
}

// TODO gc old files
func (s *queueServer) doUpload(w http.ResponseWriter, r *http.Request, jid int64) error {
	conn, err := cfg.NewRabbitmqConn()
	if err != nil {
		glog.Errorln("Couldn't connect to rabbitmq", err)
		error500(w)
		return err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		glog.Errorln("Couldn't open rabbitmq channel", err)
		error500(w)
		return err
	}
	defer ch.Close()

	f, err := createJobInputFile(jid)
	if err != nil {
		glog.Errorln("couldn't create file", err)
		error500(w)
		return err
	}
	defer f.Close()
	fpath := f.Name()

	h := sha1.New()
	limit := int64(s.SizeLimit)
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r.Body, limit))
	if n == limit {
		glog.Errorln("Error body too long", err)
		http.Error(w, "Request body too long", http.StatusBadRequest)
		os.Remove(fpath)
		return errors.New("Error body too long")
	}
	if err != nil {
		glog.Errorln("Error writing request to file", fpath, err)
		error500(w)
		return err
	}
	err = f.Close()
	if err != nil {
		glog.Errorln("Error closing file", fpath, err)
		error500(w)
		return err
	}

	glog.Infoln("job file", s.Name, "wrote bytes", n)
	hash := hex.EncodeToString(h.Sum(nil))
	glog.Infoln("Wrote job with hash", hash, "to file", fpath)
	// TODO check if hash is already queued and if it is abort and return the job id
	// TODO store hash state in db
	// TODO configurable ttr?
	q, err := ch.QueueDeclare(
		s.Name, // name
		true,   // durable
		false,  // delete when unused
		false,  // exclusive
		false,  // no-wait
		nil,    // arguments
	)
	if err != nil {
		glog.Errorln("Couldn't open rabbitmq queue", s.Name, err)
		error500(w)
		return err
	}
	err = ch.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "text/plain",
			Body:         []byte(fmt.Sprint(jid)),
		})
	if err != nil {
		glog.Errorln("Error storing job for file", fpath, "with hash", hash, "to rabbitmq")
		return err
	}
	response := UploadRensponse{JobId: fmt.Sprint(jid)}
	json_response, err := json.Marshal(response)
	if err != nil {
		glog.Errorln("json response", err)
		error500(w)
		return err
	}
	glog.Infoln("json response", string(json_response))
	w.Write(json_response)
	w.Header().Set("Content-Type", "application/json")
	return nil
}

func parseJobIdOrBadRequest(w http.ResponseWriter, s string) (jid int64, err error) {
	jid, err = strconv.ParseInt(s, 0, 64)
	if err != nil {
		glog.Errorf("Bad job id (%s): %s", s, err)
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return -1, err
	}
	return jid, err
}

var jobStatus2StringMap = map[db.JobStatus]string{
	db.JobReceived: "Received",
	db.JobRunning:  "Running",
	db.JobFail:     "Fail",
	db.JobSuccess:  "Success",
}

type ResultRensponse struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Link    string `json:"link"`
}

func handleResult(w http.ResponseWriter, r *http.Request) {
	defer logPanic()

	jid, err := parseJobIdOrBadRequest(w, r.URL.Path)
	if err != nil {
		return
	}

	status, err := db.JobGetStatus(jid)
	if err != nil {
		glog.Errorf("Error getting job status for job %d: %s", jid, err)
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case "GET":
		var respath string
		response := ResultRensponse{}
		if ret, exists := jobStatus2StringMap[status]; exists {
			response.Status = ret
		} else {
			error500(w)
			return
		}
		switch status {
		case db.JobSuccess:
			respath = filepath.Join(getJobDir(jid), "result")
			response.Link = fmt.Sprint(link, jid)
		case db.JobFail:
			respath = filepath.Join(getJobDir(jid), "error")
			response.Link = fmt.Sprint(link, jid)
		}
		if respath != "" {
			//read first few lines
			f, errf := os.Open(respath)
			if errf == nil {
				r4 := bufio.NewReader(f)
				b4, errf := r4.Peek(80)
				if errf == nil {
					response.Summary = string(b4)
				}
			}
		}
		json_response, err := json.Marshal(response)
		if err != nil {
			glog.Errorln("json response", err)
			error500(w)
			return
		}
		glog.Infoln("result json response", string(json_response))
		w.Write(json_response)
		w.Header().Set("Content-Type", "application/json")
	case "PUT":
		statusType := "result"
		if r.URL.Query().Get("status") == "error" {
			statusType = "error"
		}
		resultFile, err := openJobFile(jid, statusType)
		if err != nil {
			glog.Errorf("Error opening result file for job %d: %s", jid, err)
			error500(w)
			return
		}
		defer resultFile.Close()
		if _, err = io.Copy(resultFile, r.Body); err != nil {
			glog.Errorf("Error opening result file for job %d: %s", jid, err)
			error500(w)
			return
		}
		if statusType == "error" {
			if err = db.JobSetError(jid, "Error"); err != nil {
				glog.Errorf("Error marking failed job %d: %s", jid, err)
			}
		} else {
			if err = db.JobSetSuccess(jid); err != nil {
				glog.Errorf("Error marking successful job %d: %s", jid, err)
			}
		}
		if err == nil {
			glog.Infof("Job #%d finished with %s", jid, statusType)
		}
	}
}

func handleLink(w http.ResponseWriter, r *http.Request) {
	defer logPanic()

	jid, err := parseJobIdOrBadRequest(w, r.URL.Path)
	if err != nil {
		return
	}

	status, err := db.JobGetStatus(jid)
	if err != nil {
		glog.Errorf("Error getting job status for job %d: %s", jid, err)
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case "GET":
		var respath string
		switch status {
		case db.JobSuccess:
			respath = filepath.Join(getJobDir(jid), "result")
		case db.JobFail:
			respath = filepath.Join(getJobDir(jid), "error")
		default:
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, respath)
		return
	}
}

type queueInfo struct {
	Name     string
	Urgent   string
	Ready    string
	Reserved string
	Delayed  string
	Buried   string
	Total    string
}

func handleQueues(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		qStats, err := db.GetEngineStats(engineNames)
		if err != nil {
			glog.Errorln("Error getting engine stats:", err)
			error500(w)
			return
		}
		var res []byte
		if res, err = json.Marshal(map[string]interface{}{"Queues": qStats}); err != nil {
			glog.Errorln("Error getting queues info:", err)
			error500(w)
			return
		}
		w.Write(res)
	}
}

func getJobsQueryParams(r *http.Request) (engine string, statuses []string) {
	s := r.URL.Query().Get("st")
	if s != "" {
		statuses = strings.Split(s, ",")
	}
	return r.URL.Query().Get("eng"), statuses
}

func handleGetPagination(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		engine, statuses := getJobsQueryParams(r)
		var minID int64
		if s := r.URL.Query().Get("min-id"); s != "" {
			if minID, err = strconv.ParseInt(s, 0, 64); err != nil {
				glog.Errorf("Failed to parse min-id (%s): %s", s, err)
				http.Error(w, "Invalid Param(s)", http.StatusBadRequest)
				return
			}
		}
		pagination, err := db.GetPagination(engine, statuses, minID)
		if err != nil || len(pagination) != 1 {
			glog.Errorln("Error getting pagination info from DB", err)
			error500(w)
			return
		}
		if res, err := json.Marshal(pagination[0]); err != nil {
			glog.Errorln("Error Marshalling pagination JSON:", err)
			error500(w)
			return
		} else {
			w.Write(res)
		}
	}
}

func parseGetJobsParamsOrBadRequest(w http.ResponseWriter, s string) (maxIdx int64, limit int64, page int64, err error) {
	params := strings.Split(s, "/")
	if len(params) != 3 {
		err = errors.New("Bad number of parameters")
	} else {
		if maxIdx, err = strconv.ParseInt(params[0], 0, 64); err == nil {
			if limit, err = strconv.ParseInt(params[1], 0, 64); err == nil {
				if page, err = strconv.ParseInt(params[2], 0, 64); err == nil {
					return maxIdx, limit, page, nil
				}
			}
		}
	}
	glog.Errorf("Failed to parse one of the numeric parameters (%s): %s", s, err)
	http.Error(w, "Invalid Param(s)", http.StatusBadRequest)
	return 0, 0, 0, err
}

func handleGetJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		var err error
		maxIdx, limit, page := int64(0), int64(0), int64(0)
		if r.URL.Path != "" {
			if maxIdx, limit, page, err = parseGetJobsParamsOrBadRequest(w, r.URL.Path); err != nil {
				return
			}
		}
		engine, statuses := getJobsQueryParams(r)
		jobs, err := db.GetJobs(maxIdx, limit, page, engine, statuses)
		if err != nil {
			glog.Errorln("Error getting jobs from DB:", err)
			error500(w)
			return
		}
		if res, err := json.Marshal(map[string]interface{}{"jobs": jobs}); err != nil {
			glog.Errorln("Error Marshalling jobs JSON:", err)
			error500(w)
			return
		} else {
			w.Write(res)
		}
	}
}

func registerQueue(serverMux *http.ServeMux, e cfg.Engine) {
	qs := &queueServer{e}

	prefix := "/" + e.Name

	serverMux.HandleFunc(fmt.Sprint(prefix, "/upload"), qs.handleUpload)

	glog.Infoln("Registered queue", e.Name)
}

func httpHandleStripped(prefix string, h http.Handler) {
	http.Handle(prefix, http.StripPrefix(prefix, h))
}

func main() {
	cfg.Init()
	defer cfg.Finalize()

	for _, e := range cfg.Config.Engines {
		registerQueue(http.DefaultServeMux, e)
		engineNames = append(engineNames, e.Name) // Add to list of engines
	}

	httpHandleStripped("/files/", http.FileServer(http.Dir(*store)))
	httpHandleStripped("/console/", http.FileServer(http.Dir("./console")))

	httpHandleStripped("/result/", http.HandlerFunc(handleResult))
	httpHandleStripped(link, http.HandlerFunc(handleLink))
	httpHandleStripped("/monitor/jobs/paginate", http.HandlerFunc(handleGetPagination))
	httpHandleStripped("/monitor/jobs/", http.HandlerFunc(handleGetJobs))
	httpHandleStripped("/monitor/queues", http.HandlerFunc(handleQueues))

	glog.Infoln("Listening on", *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		glog.Errorln("Couldn't listen", err)
	}
}
