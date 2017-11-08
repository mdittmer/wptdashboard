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

package wptdashboard

import (
	"encoding/json"
	"net/http"
	"net/url"

	"cloud.google.com/go/datastore"
	"google.golang.org/appengine"
)

const projectId = "wptdashboard"

// apiTestRunsHandler is responsible for emitting test-run JSON for all the runs at a given SHA.
//
// Params:
//     sha: SHA[0:10] of the repo when the tests were executed (or 'latest')
func apiTestRunsHandler(w http.ResponseWriter, r *http.Request) {
	runSHA, err := GetRunSHA(r)
	if err != nil {
		http.Error(w, "Invalid query params", http.StatusBadRequest)
		return
	}

	var browserNames []string
	browserNames, err = GetBrowserNames()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := appengine.NewContext(r)
	client, err := datastore.NewClient(ctx, projectId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	testRuns, err := TestRunsForShaAndBrowsers(ctx, client, runSHA, browserNames)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	testRunsBytes, err := json.Marshal(testRuns)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(testRunsBytes)
}

// apiTestRunHandler is responsible for emitting the test-run JSON a specific run,
// identified by a named browser (platform) at a given SHA.
//
// Params:
//     sha: SHA[0:10] of the repo when the test was executed (or 'latest')
//     browser: Browser for the run (e.g. 'chrome', 'safari-10')
func apiTestRunHandler(w http.ResponseWriter, r *http.Request) {
	runSHA, err := GetRunSHA(r)
	if err != nil {
		http.Error(w, "Invalid query params", http.StatusBadRequest)
		return
	}

	var browserName string
	browserName, err = getBrowserParam(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if browserName == "" {
		http.Error(w, "Invalid 'browser' param", http.StatusBadRequest)
		return
	}

	ctx := appengine.NewContext(r)
	client, err := datastore.NewClient(ctx, projectId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	testRun, err := TestRunsForShaAndBrowser(ctx, client, runSHA, browserName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if testRun == nil {
		http.NotFound(w, r)
		return
	}

	testRunsBytes, err := json.Marshal(*testRun)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(testRunsBytes)
}

// getBrowserParam parses and validates the 'browser' param for the request.
// It returns "" by default (and in error cases).
func getBrowserParam(r *http.Request) (browser string, err error) {
	browser = ""
	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return browser, err
	}

	browserNames, err := GetBrowserNames()
	if err != nil {
		return browser, err
	}

	browser = params.Get("browser")
	// Check that it's a browser name we recognize.
	for _, name := range browserNames {
		if name == browser {
			return name, nil
		}
	}
	return "", nil
}
