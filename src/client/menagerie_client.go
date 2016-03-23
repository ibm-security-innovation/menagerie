package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/looplab/fsm"
)

type Config struct {
	MenagerieBase   string        `json:"menagerie_base"`
	PoolingInterval time.Duration `json:"pooling_interval"`
	PoolingAttempts int           `json:"pooling_attempts"`
}

type UploadResult struct {
	JobId string `json:"jobid"`
}

type Result struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Link    string `json:"link"`
	output  io.Writer
	Error   string
}

type UploadRequestSender interface {
	SendUploadRequest(requestBody []byte)
}

type ResultChecker interface {
	CheckForResult()
}

type ResultDownloader interface {
	DownloadResult()
}

type Finalizer interface {
	Finalize(strs ...interface{})
}

// Client example simple Chaincode implementation
type Client struct {
	cfg          Config
	uploadResult UploadResult
	result       Result
	fsm          *fsm.FSM
	notify       chan Result
	engine       string
}

func (t *Client) Finalize(strs ...interface{}) {
	if len(strs) > 0 && strs[0].(string) != "" {
		glog.Error(strs)
		if t.notify != nil {
			t.result.Error = strs[0].(string)
		}
	}
	glog.V(0).Infoln("Finalize", t.result.Error)
	t.notify <- t.result
}

// based on https://gist.githubusercontent.com/mattetti/5914158/raw/51ee58b51c43f797f200d273853f64e13aa21a8a/multipart_upload.go
func (t *Client) newfileUploadRequest(uri string, params map[string]string, bodyField string, toUpload []byte) (*http.Request, error) {
	glog.V(0).Infoln("newfileUploadRequest", uri)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	p, err := writer.CreateFormField(bodyField)

	if err != nil {
		t.Finalize("can't create form field")
		return nil, err
	}

	_, err = p.Write(toUpload)
	if err != nil {
		t.Finalize("can't write payload")
		return nil, err
	}
	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	err = writer.Close()
	if err != nil {
		t.Finalize("close failed")
		return nil, err
	}

	req, err := http.NewRequest("POST", uri, body)
	if err != nil {
		t.Finalize("new request failed")
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func (t *Client) returnUploadResponse(respBody []byte) {
	glog.V(0).Infoln("returnUploadResponse", string(respBody[:]))
	err := json.Unmarshal(respBody, &t.uploadResult)
	if err != nil {
		t.Finalize("returnUploadResponse  -can't parse response body. ", err)
		return
	}
	glog.V(0).Infoln("returnUploadResponse", t.uploadResult)
	t.fsm.Event("poll_for_result", t)
}

func (t *Client) SendUploadRequest(requestBody []byte) {
	httpRequest := func() {
		req, err := t.newfileUploadRequest(fmt.Sprintf("%s/%s/upload", t.cfg.MenagerieBase, t.engine),
			make(map[string]string),
			"upload", requestBody)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Finalize("SendUploadRequest - failed sending. ", err)
			return
		}

		defer resp.Body.Close()
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Finalize("SendUploadRequest - can't read response body. ", err)
			return
		}

		t.returnUploadResponse(respBody)
	}
	go httpRequest()
}

// the args are the mngr pointer and the file name to upload
func upload(e *fsm.Event) {
	mngr, ok := e.Args[0].(UploadRequestSender)
	if !ok {
		glog.Fatalf("upload - missing mngr object %v", e.Args[0])
	}
	uploadBody, ok := e.Args[1].([]byte)
	if !ok {
		glog.Fatalf("upload - missing upload body")
	}
	glog.V(0).Infoln("upload", len(uploadBody))
	go mngr.SendUploadRequest(uploadBody)
}

func (t *Client) CheckForResult() {
	httpRequest := func() {
		resp, err := http.Get(fmt.Sprintf("%s/result/%s", t.cfg.MenagerieBase, t.uploadResult.JobId))
		if err != nil {
			t.Finalize("CheckForResult - failed sending. ", err)
			return
		}

		defer resp.Body.Close()
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Finalize("CheckForResult - can't read response body. ", err, respBody)
			return
		}
		err = json.Unmarshal(respBody, &t.result)
		if err != nil {
			t.Finalize("CheckForResult - can't parse response. ", err, respBody)
			return
		}
		glog.V(0).Infoln("CheckForResult - status ", t.result.Status)
		switch t.result.Status {
		case "Success":
			t.fsm.Event("download", t)
		case "Running": // if the resposne is Running, issue a new 'poll_for_result'
			t.cfg.PoolingAttempts--
			if t.cfg.PoolingAttempts < 0 {
				glog.Error("CheckForResult - max amount of polling reached. Aborting")
				t.fsm.Event("failed", t, "max amount of polling reached")
			} else {
				time.AfterFunc(t.cfg.PoolingInterval, func() {
					t.fsm.Event("poll_for_result", t)
				})
			}
		case "Failed":
			// TODO - in case if a Failed status, an error file may be generated and the link points to it.
			// So, we should download it anyway
			t.fsm.Event("failed", t, t.result)
		default:
			t.Finalize("CheckForResult - unexpected status ", t.result.Status)
			return
		}

	}
	go httpRequest()
}

// if no result is available from Menagerie, we'll just pull again after some idling
// the job id is the 2nd parameter
func poll_for_result(e *fsm.Event) {
	mngr, ok := e.Args[0].(ResultChecker)
	if !ok {
		glog.Fatalf("poll_for_result - missing mngr object %v", e.Args[0])
	}

	glog.V(0).Infoln("poll_for_result")
	go mngr.CheckForResult()
}

func (t *Client) DownloadResult() {
	httpRequest := func() {

		resp, err := http.Get(fmt.Sprintf("%s/%s", t.cfg.MenagerieBase, t.result.Link))
		if err != nil {
			t.Finalize("DownloadResult - failed sending. ", err)
			return
		}

		defer resp.Body.Close()

		io.Copy(t.result.output, resp.Body)
		t.fsm.Event("download_completed", t)
	}
	go httpRequest()

}

func download(e *fsm.Event) {
	mngr, ok := e.Args[0].(ResultDownloader)
	if !ok {
		glog.Fatalf("upload - missing mngr object %v", e.Args[0])
	}

	glog.V(0).Infoln("result_available")
	go mngr.DownloadResult()
}

func download_completed(e *fsm.Event) {
	mngr, ok := e.Args[0].(Finalizer)
	if !ok {
		glog.Fatalf("download_completed - missing mngr object %v", e.Args[0])
	}

	glog.V(0).Infoln("download_completed")
	mngr.Finalize("")
}

func failed(e *fsm.Event) {
	mngr, ok := e.Args[0].(Finalizer)
	if !ok {
		glog.Fatalf("failed - missing mngr object %v", e.Args[0])
	}

	reason, ok := e.Args[1].(string)
	if !ok {
		glog.Error("failed - missing reason string")
	}

	glog.V(0).Infoln("failed")
	mngr.Finalize(reason)
}

func (t *Client) readConfig(configFile string) {
	if configFile == "" {
		return
	}
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't read config file.\n")
		return
	}
	err = json.Unmarshal(b, &t.cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't parse config file", err, "\n")
		return
	}
	t.cfg.PoolingInterval = t.cfg.PoolingInterval * time.Millisecond
	if t.cfg.PoolingAttempts < 1 {
		t.cfg.PoolingAttempts = 1
	}
	glog.V(0).Infoln("readConfig", t.cfg)
}

func (t *Client) doit(input []byte) Result {
	go func() { t.fsm.Event("upload", t, input) }()
	r := <-t.notify
	return r
}

func NewClient(configFile string, engine string, resultDest io.Writer) *Client {
	mngr := &Client{engine: engine, notify: make(chan Result)}
	mngr.result.output = resultDest
	mngr.readConfig(configFile)
	mngr.fsm = fsm.NewFSM(
		"idle",
		fsm.Events{
			{Name: "upload", Src: []string{"idle"}, Dst: "pending"},
			{Name: "poll_for_result", Src: []string{"pending"}, Dst: "pending"},
			{Name: "download", Src: []string{"pending"}, Dst: "downloading"},
			{Name: "download_completed", Src: []string{"downloading"}, Dst: "done"},
			{Name: "failed", Src: []string{"pending"}, Dst: "done"},
		},

		fsm.Callbacks{
			"upload":             upload,
			"poll_for_result":    poll_for_result,
			"download":           download,
			"download_completed": download_completed,
			"failed":             failed,
		},
	)
	return mngr
}
