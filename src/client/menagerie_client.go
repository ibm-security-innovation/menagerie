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
)

type Config struct {
	MenagerieBase   string        `json:"menagerie_base"`
	PollingInterval time.Duration `json:"polling_interval"`
	PollingAttempts int           `json:"polling_attempts"`
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

// Client example simple Chaincode implementation
type Client struct {
	cfg          Config
	uploadResult UploadResult
	result       Result
	engine       string
}

func (t *Client) Finalize(label string, err error, strs ...interface{}) error {
	if err != nil {
		glog.Error(label, err, strs)
		t.result.Error = fmt.Sprint(label, err, strs)
	}
	glog.V(0).Infoln(label, err, t.result.Error)
	return err
}

// based on https://gist.githubusercontent.com/mattetti/5914158/raw/51ee58b51c43f797f200d273853f64e13aa21a8a/multipart_upload.go
func (t *Client) newfileUploadRequest(uri string, params map[string]string, bodyField string, toUpload []byte) (*http.Request, error) {
	glog.V(0).Infoln("newfileUploadRequest", uri)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	p, err := writer.CreateFormField(bodyField)

	if err != nil {
		return nil, t.Finalize("can't create form field", err)
	}

	_, err = p.Write(toUpload)
	if err != nil {
		return nil, t.Finalize("can't write payload", err)
	}
	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	err = writer.Close()
	if err != nil {
		return nil, t.Finalize("close failed", err)
	}

	req, err := http.NewRequest("POST", uri, body)
	if err != nil {
		return nil, t.Finalize("new request failed", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

func (t *Client) SendUploadRequest(requestBody []byte) (*UploadResult, error) {
	req, err := t.newfileUploadRequest(fmt.Sprintf("%s/%s/upload", t.cfg.MenagerieBase, t.engine),
		make(map[string]string),
		"upload", requestBody)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, t.Finalize("SendUploadRequest - failed sending. ", err)
	}

	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, t.Finalize("SendUploadRequest - can't read response body. ", err)
	}

	err = json.Unmarshal(respBody, &t.uploadResult)
	if err != nil {
		return nil, t.Finalize("SendUploadRequest  - can't parse response body. ", err)
	}
	glog.V(0).Infoln("returnUploadResponse", t.uploadResult)
	return &t.uploadResult, nil
}

func (t *Client) CheckForResult() (bool, error) {
	for i := 0; i < t.cfg.PollingAttempts; i++ {
		resp, err := http.Get(fmt.Sprintf("%s/result/%s", t.cfg.MenagerieBase, t.uploadResult.JobId))
		if err != nil {
			return false, t.Finalize("CheckForResult - failed sending. ", err)
		}

		defer resp.Body.Close()
		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, t.Finalize("CheckForResult - can't read response body. ", err, respBody)
		}
		err = json.Unmarshal(respBody, &t.result)
		if err != nil {
			return false, t.Finalize("CheckForResult - can't parse response. ", err, respBody)
		}
		glog.V(0).Infoln("CheckForResult - status ", t.result.Status, i)

		switch t.result.Status {
		case "Success":
			return true, nil
		case "Running": // if the resposne is Running, issue a new 'poll_for_result'
			time.Sleep(t.cfg.PollingInterval)
		case "Failed":
			// TODO - in case if a Failed status, an error file may be generated and the link points to it.
			// So, we should download it anyway
			return false, nil
		default:
			return false, t.Finalize("CheckForResult - unexpected status ", nil, t.result.Status)
		}
	}

	return false, t.Finalize("CheckForResult - max amount of polling reached. Aborting", nil)
}

func (t *Client) DownloadResult() (bool, error) {
	resp, err := http.Get(fmt.Sprintf("%s/%s", t.cfg.MenagerieBase, t.result.Link))
	if err != nil {
		return false, t.Finalize("DownloadResult - failed sending. ", err)
	}

	defer resp.Body.Close()

	io.Copy(t.result.output, resp.Body)
	return true, nil
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
	t.cfg.PollingInterval = t.cfg.PollingInterval * time.Millisecond
	glog.V(0).Infoln("readConfig", t.cfg)
}

func (t *Client) doit(input []byte) Result {
	_, err := t.SendUploadRequest(input)
	if err != nil {
		return t.result
	}
	success, err := t.CheckForResult()
	if !success || err != nil {
		return t.result
	}
	success, err = t.DownloadResult()
	if !success || err != nil {
		return t.result
	}
	return t.result
}

func NewClient(configFile string, engine string, resultDest io.Writer) *Client {
	mngr := &Client{engine: engine}
	mngr.result.output = resultDest
	mngr.readConfig(configFile)
	if mngr.cfg.PollingAttempts < 1 {
		mngr.cfg.PollingAttempts = 1
	}
	return mngr
}
