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
package cfg

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"stats"

	"encoding/json"
	"github.com/golang/glog"
	"github.com/streadway/amqp"
)

var (
	ConfigFile       string
	EngineConfigFile string
	Rabbitmq         string
	MqUser           string
	MqPass           string
	Frontend         string
	Mysql            string
	MysqlUser        string
	MysqlPass        string
	MysqlDatabase    string
)

func init() {
	flag.StringVar(&ConfigFile, "cfg", "./confs/default.json", "config file name")
	flag.StringVar(&EngineConfigFile, "engine-cfg", "./confs/engines.json", "config file name")
	flag.StringVar(&Rabbitmq, "rabbitmq", "127.0.0.1:5672", "address of rabbitmq API")
	flag.StringVar(&MqUser, "mq_user", "", "User of the rabbitmq server")
	flag.StringVar(&MqPass, "mq_pass", "", "Pass of the rabbitmq server")
	flag.StringVar(&Frontend, "frontend", "127.0.0.1:8080", "address of the frontend")

	flag.StringVar(&Mysql, "mysql", "menagerie_mysql:3306", "Address of the mysql server")
	flag.StringVar(&MysqlUser, "mysql_user", "", "User of the mysql server")
	flag.StringVar(&MysqlPass, "mysql_pass", "", "Pass of the mysql server")
	flag.StringVar(&MysqlDatabase, "mysql_database", "", "Database of the mysql server")
}

type Cfg struct {
	Flags   map[string]map[string]string
	Engines []Engine
}

type Engine struct {
	Name          string
	Workers       int
	Image         string
	Cmd           string
	MountPoint    string
	SizeLimit     int
	InputFileName string
	Timeout       int
	User          int
	RunFlags      []string
}

func NewRabbitmqConn() (*amqp.Connection, error) {
	return amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s/", MqUser, MqPass, Rabbitmq))
}

func NewMysqlDb() (*sql.DB, error) {
	return sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true", MysqlUser, MysqlPass, Mysql, MysqlDatabase))
}

var Config *Cfg

func read() {
	// TODO check hosts
	if !flag.Parsed() {
		panic("config read before parsing flags")
	}
	f, err := ioutil.ReadFile(ConfigFile)
	if err != nil {
		fmt.Println("ERROR: couldn't open config file", err)
	}
	err = json.Unmarshal(f, &Config)
	if err != nil {
		fmt.Println("ERROR: couldn't unmarshal config file", err)
	}
	f, err = ioutil.ReadFile(EngineConfigFile)
	if err != nil {
		fmt.Println("ERROR: couldn't open engine config file", err)
	}
	err = json.Unmarshal(f, &Config)
	if err != nil {
		fmt.Printf("ERROR: couldn't unmarshal engine config file (%s): %s", EngineConfigFile, err)
	}
	fmt.Printf("Config file read: %+v", Config)

	alreadySet := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { alreadySet[f.Name] = true })
	override := func(k, v string) {
		if alreadySet[k] {
			return
		}
		f := flag.Lookup(k)
		if f == nil {
			fmt.Println("unexpected flag in json file", k, v)
			os.Exit(1)
		}
		f.Value.Set(v)
	}

	for k, v := range Config.Flags["all"] {
		override(k, v)
	}
	for k, v := range Config.Flags[path.Base(os.Args[0])] {
		override(k, v)
	}
}

func GetEngine(name string) *Engine {
	for _, e := range Config.Engines {
		if e.Name == name {
			return &e
		}
	}
	return nil
}

func Init() {
	flag.Parse()
	read()
}

func Finalize() {
	stats.Flush()
	glog.Flush()
}
