/*
Copyright 2019 The Kruise Authors
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/xyproto/simpleredis"
)

var (
	// For when Redis is used
	masterPool *simpleredis.ConnectionPool
	slavePool  *simpleredis.ConnectionPool

	// For when Redis is not used, we just keep it in memory
	lists = map[string][]string{}
)

// GetList gets list by key
func GetList(key string) ([]string, error) {
	// Using Redis
	if slavePool != nil {
		list := simpleredis.NewList(slavePool, key)
		if result, err := list.GetAll(); err == nil {
			return result, err
		}
		// if we can't talk to the slave then assume its not running yet
		// so just try to use the master instead
	}

	// if the slave doesn't exist, read from the master
	if masterPool != nil {
		list := simpleredis.NewList(masterPool, key)
		return list.GetAll()
	}

	// if neither exist, we're probably in "in-memory" mode
	return lists[key], nil
}

// AppendToList put item into list
func AppendToList(item string, key string) ([]string, error) {
	var err error
	var items []string

	// Using Redis
	if masterPool != nil {
		list := simpleredis.NewList(masterPool, key)
		list.Add(item)
		items, err = list.GetAll()
		if err != nil {
			return nil, err
		}
	} else {
		items = lists[key]
		items = append(items, item)
		lists[key] = items
	}
	return items, nil
}

// ListRangeHandler handles lrange request
func ListRangeHandler(rw http.ResponseWriter, req *http.Request) {
	var data []byte

	items, err := GetList(mux.Vars(req)["key"])
	if err != nil {
		data = []byte("Error getting list: " + err.Error() + "\n")
	} else {
		if data, err = json.MarshalIndent(items, "", ""); err != nil {
			data = []byte("Error marhsalling list: " + err.Error() + "\n")
		}
	}

	rw.Write(data)
}

// ListPushHandler handles rpush request
func ListPushHandler(rw http.ResponseWriter, req *http.Request) {
	var data []byte

	key := mux.Vars(req)["key"]
	value := mux.Vars(req)["value"]

	items, err := AppendToList(value, key)

	if err != nil {
		data = []byte("Error adding to list: " + err.Error() + "\n")
	} else {
		if data, err = json.MarshalIndent(items, "", ""); err != nil {
			data = []byte("Error marshalling list: " + err.Error() + "\n")
		}

	}
	rw.Write(data)
}

// InfoHandler returns DB info
func InfoHandler(rw http.ResponseWriter, req *http.Request) {
	info := ""

	// Using Redis
	if masterPool != nil {
		i, err := masterPool.Get(0).Do("INFO")
		if err != nil {
			info = "Error getting DB info: " + err.Error()
		} else {
			info = string(i.([]byte))
		}
	} else {
		info = "In-memory datastore (not redis)"
	}
	rw.Write([]byte(info + "\n"))
}

// EnvHandler returns environment info
func EnvHandler(rw http.ResponseWriter, req *http.Request) {
	environment := make(map[string]string)
	for _, item := range os.Environ() {
		splits := strings.Split(item, "=")
		key := splits[0]
		val := strings.Join(splits[1:], "=")
		environment[key] = val
	}

	data, err := json.MarshalIndent(environment, "", "")
	if err != nil {
		data = []byte("Error marshalling env vars: " + err.Error())
	}

	rw.Write(data)
}

// HelloHandler returns "hello"
func HelloHandler(rw http.ResponseWriter, req *http.Request) {
	rw.Write([]byte("Hello from guestbook. " +
		"Your app is up! (Hostname: " +
		os.Getenv("HOSTNAME") +
		")\n"))
}

// Support multiple URL schemes for different use cases
func findRedisURL() string {
	host := os.Getenv("REDIS_MASTER_SERVICE_HOST")
	port := os.Getenv("REDIS_MASTER_SERVICE_PORT")
	password := os.Getenv("REDIS_MASTER_SERVICE_PASSWORD")
	masterPort := os.Getenv("REDIS_MASTER_PORT")

	if host != "" && port != "" && password != "" {
		return password + "@" + host + ":" + port
	} else if masterPort != "" {
		return "redis-master:6379"
	}
	return ""
}

func main() {
	// When using Redis, setup our DB connections
	url := findRedisURL()
	if url != "" {
		masterPool = simpleredis.NewConnectionPoolHost(url)
		defer masterPool.Close()
		slavePool = simpleredis.NewConnectionPoolHost("redis-slave:6379")
		defer slavePool.Close()
	}

	r := mux.NewRouter()
	r.Path("/lrange/{key}").Methods("GET").HandlerFunc(ListRangeHandler)
	r.Path("/rpush/{key}/{value}").Methods("GET").HandlerFunc(ListPushHandler)
	r.Path("/info").Methods("GET").HandlerFunc(InfoHandler)
	r.Path("/env").Methods("GET").HandlerFunc(EnvHandler)
	r.Path("/hello").Methods("GET").HandlerFunc(HelloHandler)

	n := negroni.Classic()
	n.UseHandler(r)
	n.Run(":4000")
}
