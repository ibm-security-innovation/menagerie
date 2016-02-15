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
// +build integration

package test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

const host = "127.0.0.1:9007"
const baseUrl = "http://" + host + "/"

func cmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

type checkResult func([]byte) bool

type ResultResponse struct {
	Status  string `json:"status"`
	Link    string `json:"link"`
	Summary string `json:"summary"`
}

func offsetSubstr(b []byte, offset, env int64) []byte {
	from, to := int64(0), int64(len(b))
	if offset-env >= 0 {
		from = offset - env
	}
	if offset+env < int64(len(b)) {
		to = offset + env
	}
	return b[from:to]
}

func checkResultResponse(body []byte, t *testing.T) (*ResultResponse, error) {
	var data ResultResponse
	err := json.Unmarshal(body, &data)
	if err != nil {
		t.Error("unmarshaling upload response")
		t.Error(err)
		switch v := err.(type) {
		case *json.SyntaxError:
			t.Error(string(offsetSubstr(body, v.Offset, 40)))
		}
		return nil, err
	}
	return &data, nil
}

func doGet(url string, t *testing.T) (body []byte) {
	t.Log("GET", url)
	res, err := http.Get(url)
	if err != nil {
		t.Fatal("Error sending request")
	}
	if res.StatusCode != http.StatusOK {
		t.Fatal("Bad HTTP code from URL", url, res.StatusCode)
	}
	b, _ := ioutil.ReadAll(res.Body)
	return b
}

func waitForResult(getUrl string, timeout time.Duration, fn checkResult, t *testing.T) *ResultResponse {
	now := time.Now()
	for {
		if time.Since(now) > (timeout) {
			t.Error("test timeout")
			return nil
		}
		b := doGet(getUrl, t)
		s := string(b)
		t.Log("got response:", s)
		res, err := checkResultResponse(b, t)
		if err != nil || (res.Status != "Running" && res.Status != "Received") {
			if !fn(b) {
				t.Error("unexpected response", s)
			} else if err == nil {
				t.Log("got valid result. Status:", res.Status)
			}
			return res
		}
		time.Sleep(time.Second)
	}
}

func callAndExtractJobId(req *http.Request, t *testing.T) string {
	type UploadResponse struct {
		JobId string `json:"jobid"`
	}
	res, err := http.DefaultClient.Do(req)

	if err != nil {
		t.Fatal("Test Error sending request")
	}

	defer res.Body.Close()
	body, rerr := ioutil.ReadAll(res.Body)
	if rerr != nil {
		t.Error("problem reading response body ")
		t.Error(rerr)
	}
	var data UploadResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		t.Error("unmarshaling upload response ")
		t.Error(err)
		switch v := err.(type) {
		case *json.SyntaxError:
			t.Error(string(offsetSubstr(body, v.Offset, 40)))
		}
	}
	if data.JobId == "" {
		t.Fatal("Test  Error getting id of upload")
	}
	t.Log("Job ID:", data.JobId)
	return data.JobId
}

func TestMain(m *testing.M) {
	flag.Parse()

	for i := 0; i < 60; i++ {
		fmt.Println("dialing frontend")
		_, err := net.Dial("tcp", host)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("Main frontend up - going to tests")
	os.Exit(m.Run())
}

func TestWorker(t *testing.T) {
	req, _ := http.NewRequest("PUT", baseUrl+"testengine/upload", bytes.NewBufferString("foo"))

	id := callAndExtractJobId(req, t)

	res := waitForResult(baseUrl+"result/"+id, 60*time.Second, func(s []byte) bool {
		res, err := checkResultResponse(s, t)
		return err == nil && res.Status == "Success"
	}, t)

	if res == nil {
		t.Error("No result from query")
	} else {
		b := doGet(baseUrl+res.Link, t)
		exp := "acbd18db4cc2f85cedef654fccc4a4d8\n"
		if string(b) != exp {
			t.Errorf("Unexpected md5: expected <%q> got <%q>", exp, string(b))
		}
	}
}
