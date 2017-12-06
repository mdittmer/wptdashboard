// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gae

import (
	"fmt"
	"log"
	"time"

	base "github.com/w3c/wptdashboard"
	"github.com/w3c/wptdashboard/metrics"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

func EnsureDevData(ctx context.Context) {
	tokens := []interface{}{&base.Token{}}
	urlFmtString := "http://localhost:8080/static/wptd/%s/%s"
	timeZero := time.Unix(0, 0)
	mkURL := func(hash string, summaryJsonGz string) string {
		return fmt.Sprintf(urlFmtString, hash, summaryJsonGz)
	}
	properTestRuns := []base.TestRun{
		base.TestRun{
			"chrome",
			"63.0",
			"linux",
			"3.16",
			"b952881825",
			mkURL("b952881825", "chrome-63.0-linux-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"edge",
			"15",
			"windows",
			"10",
			"5d55258739",
			mkURL("5d55258739", "windows-10-sauce-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"firefox",
			"57.0",
			"linux",
			"*",
			"fc70df1f75",
			mkURL("fc70df1f75", "firefox-57.0-linux-summary.json.gz"),
			timeZero,
		},
		base.TestRun{
			"safari",
			"10",
			"macos",
			"10.12",
			"fc2e57a502",
			mkURL("fc2e57a502", "safari-11.0-macos-10.12-sauce-summary.json.gz"),
			timeZero,
		},
	}
	testRuns := make([]interface{}, len(properTestRuns))
	for i, testRun := range properTestRuns {
		testRuns[i] = &testRun
	}
	metricsRuns := []interface{}{
		&metrics.MetricsRun{
			timeZero,
			timeZero,
			properTestRuns,
		},
	}

	tokenKindName := "Token"
	testRunKindName := "TestRun"
	metricsRunKindName := metrics.GetDatastoreKindName(metrics.MetricsRun{})

	ensureDataByCount(ctx, tokenKindName, tokens)
	ensureDataByCount(ctx, testRunKindName, testRuns)
	ensureDataByCount(ctx, metricsRunKindName, metricsRuns)
}

func ensureDataByCount(ctx context.Context, kindName string, data []interface{}) {
	log.Printf("Ensuring at least %d %s entities\n", len(data), kindName)
	count, err := datastore.NewQuery(kindName).Count(ctx)
	if err != nil {
		log.Fatalf("Failed to execute count query: %v", err)
	}
	if count >= len(data) {
		return
	}
	keys := make([]*datastore.Key, len(data))
	for i, _ := range data {
		keys[i] = datastore.NewIncompleteKey(ctx, kindName, nil)
	}
	datastore.PutMulti(ctx, keys, data)
}

// func main() {
// 	mkDatastoreDir := false
// 	wd, err := os.Getwd()
// 	if err != nil {
// 		log.Fatal("Failed to get working directory")
// 	}
// 	datastoreDir := wd + "/datastore"
// 	_, err = os.Stat(datastoreDir)
// 	if err == nil {
// 		log.Fatalf("Datastore directory exists: %s", datastoreDir)
// 	}
// 	if !os.IsNotExist(err) {
// 		log.Fatalf("Error in stat of Datastore directory: %v", err)
// 	}
// 	log.Printf("Creating Datastore dir: %s\n", datastoreDir)
// 	if err = os.Mkdir(datastoreDir, 0777); err != nil {
// 		log.Fatalf("Error creating Datastore directory: %v", err)
// 	} else {
// 		mkDatastoreDir = true
// 	}

// 	host := "0.0.0.0"
// 	port := "9891"
// 	datastoreEmulatorHost := fmt.Sprintf("%s:%s", host, port)
// 	datastoreEmulatorCmd := exec.Command("gcloud", "beta", "emulators",
// 		"datastore", "start",
// 		fmt.Sprintf("--data-dir=%s", datastoreDir),
// 		fmt.Sprintf("--host-port=%s", datastoreEmulatorHost))
// 	log.Println("Starting Datastore emulator")
// 	datastoreEmulatorCmd.Stdout = os.Stdout
// 	datastoreEmulatorCmd.Stderr = os.Stderr
// 	if err = datastoreEmulatorCmd.Start(); err != nil {
// 		log.Fatalf("Error starting Datastore emulator: %v", err)
// 	}
// 	log.Println("Waiting for Datastore emulator to boot")
// 	time.Sleep(5 * time.Second)

// 	log.Println("Setting up Datastore client environment")
// 	if err = os.Setenv("DATASTORE_EMULATOR_HOST", datastoreEmulatorHost); err != nil {
// 		log.Fatalf("Error setting up Datastore client environment: %v", err)
// 	}

// 	log.Println("Creating Datastore client")
// 	ctx := context.Background()
// 	client, err := datastore.NewClient(ctx, "wptdashboard")
// 	if err != nil {
// 		log.Fatalf("Failed to create Datastore client: %v\n", err)
// 	}

// 	tokenKindName := "Token"
// 	testRunKindName := "TestRun"
// 	metricsRunKindName := storage.GetDatastoreKindName(metrics.MetricsRun{})

// 	if !mkDatastoreDir {
// 		log.Println("Ensuring Datastore state is clean")
// 		checkCount := func(kindName string) {
// 			count, err := client.Count(ctx, datastore.NewQuery(kindName))
// 			if err != nil {
// 				log.Fatalf("Failed to count %s objects in Datastore",
// 					kindName)
// 			}
// 			if count != 0 {
// 				log.Fatalf("Datastore already contains %s objects",
// 					kindName)
// 			}
// 		}
// 		checkCount(tokenKindName)
// 		checkCount(testRunKindName)
// 		checkCount(metricsRunKindName)
// 	}

// 	var token base.Token
// 	urlFmtString := "http://localhost:8080/static/%s/%s"
// 	timeZero := time.Unix(0, 0)
// 	mkURL := func(hash string, summaryJsonGz string) string {
// 		return fmt.Sprintf(urlFmtString, hash, summaryJsonGz)
// 	}
// 	testRuns := []base.TestRun{
// 		{
// 			"chrome",
// 			"63.0",
// 			"linux",
// 			"3.16",
// 			"b952881825",
// 			mkURL("b952881825", "chrome-63.0-linux-summary.json.gz"),
// 			timeZero,
// 		},
// 		{
// 			"edge",
// 			"15",
// 			"windows",
// 			"10",
// 			"5d55258739",
// 			mkURL("5d55258739", "windows-10-sauce-summary.json.gz"),
// 			timeZero,
// 		},
// 		{
// 			"firefox",
// 			"57.0",
// 			"linux",
// 			"*",
// 			"fc70df1f75",
// 			mkURL("fc70df1f75", "firefox-57.0-linux-summary.json.gz"),
// 			timeZero,
// 		},
// 		{
// 			"safari",
// 			"10",
// 			"macos",
// 			"10.12",
// 			"fc2e57a502",
// 			mkURL("fc2e57a502", "safari-11.0-macos-10.12-sauce-summary.json.gz"),
// 			timeZero,
// 		},
// 	}
// 	metricsRuns := [1]metrics.MetricsRun{
// 		metrics.MetricsRun{
// 			timeZero,
// 			timeZero,
// 			testRuns,
// 		},
// 	}

// 	log.Println("Adding data to Datastore")
// 	client.Put(ctx, datastore.IncompleteKey(tokenKindName, nil), &token)
// 	for _, testRun := range testRuns {
// 		client.Put(ctx, datastore.IncompleteKey(testRunKindName, nil),
// 			&testRun)
// 	}
// 	for _, metricsRun := range metricsRuns {
// 		client.Put(ctx,
// 			datastore.IncompleteKey(metricsRunKindName, nil),
// 			&metricsRun)
// 	}
// 	log.Println("Added data to Datastore")

// 	log.Println("Stopping Datastore")
// 	if err = datastoreEmulatorCmd.Process.Signal(os.Interrupt); err != nil {
// 		log.Fatalf("Failed to send interrupt signal to Datastore: %v\n",
// 			err)
// 	}
// 	datastoreEmulatorCmd.Wait()

// 	log.Println(" :: ")
// 	log.Printf(" :: Be sure to run dev_appserver.py with --datastore_path=%s\n",
// 		datastoreDir)
// 	log.Println(" :: ")
// }
