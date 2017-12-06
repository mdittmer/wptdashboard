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
	"html/template"
	"net/http"

	"google.golang.org/appengine"
)

var templates = template.Must(template.ParseGlob("templates/*.html"))

// TODO: Figure out how to access Datastore during app init phase to avoid
// need for lazy-load-on-first-request.
var devDataLoaded bool = false

func handleDev(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !devDataLoaded {
			EnsureDevData(appengine.NewContext(r))
			devDataLoaded = true
		}
		h(w, r)
	}
	return handler
}
func handleProd(h func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		h(w, r)
	}
	return handler
}

func init() {
	var decorate func(func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request)
	if appengine.IsDevAppServer() {
		decorate = handleDev
	} else {
		decorate = handleProd
	}

	http.HandleFunc("/test-runs", decorate(testRunsHandler))
	http.HandleFunc("/about", decorate(aboutHandler))
	http.HandleFunc("/api/diff", decorate(apiDiffHandler))
	http.HandleFunc("/api/runs", decorate(apiTestRunsHandler))
	http.HandleFunc("/api/run", decorate(apiTestRunHandler))
	http.HandleFunc("/results", decorate(resultsRedirectHandler))
	http.HandleFunc("/metrics", decorate(metricsHandler))
	http.HandleFunc("/", decorate(testHandler))
}
